package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"CuckoosVision/internal/domain"
	"CuckoosVision/internal/model"
)

type Config struct {
	MaxDepth    int
	Concurrency int
	Delay       time.Duration
	UserAgent   string
	Timeout     time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxDepth:    3,
		Concurrency: 5,
		Delay:       time.Second,
		UserAgent:   "Mozilla/5.0 (compatible; CuckoosVision/1.0)",
		Timeout:     30 * time.Second,
	}
}

type Result struct {
	TargetURL    string
	References   []model.Reference
	Domains      map[string]struct{}
	PagesVisited int
	Errors       []error
}

type page struct {
	url   *url.URL
	depth int
}

func Crawl(ctx context.Context, targetURL string, cfg Config) (*Result, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	baseDomain, err := domain.ExtractRegistrable(target.Hostname())
	if err != nil {
		baseDomain = target.Hostname()
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	result := &Result{
		TargetURL: targetURL,
		Domains:   make(map[string]struct{}),
	}

	visited := make(map[string]struct{})
	currentLevel := []*url.URL{target}
	sem := make(chan struct{}, cfg.Concurrency)

	for depth := 0; depth <= cfg.MaxDepth && len(currentLevel) > 0; depth++ {
		var nextLevel []*url.URL
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, pageURL := range currentLevel {
			norm := normalizeURL(pageURL)
			if _, seen := visited[norm]; seen {
				continue
			}
			visited[norm] = struct{}{}

			wg.Add(1)
			go func(u *url.URL) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				if ctx.Err() != nil {
					return
				}

				refs, links, errs := fetchAndExtract(ctx, client, u, baseDomain, cfg.UserAgent)

				mu.Lock()
				result.PagesVisited++
				result.References = append(result.References, refs...)
				for _, ref := range refs {
					result.Domains[ref.Domain] = struct{}{}
				}
				result.Errors = append(result.Errors, errs...)
				nextLevel = append(nextLevel, links...)
				mu.Unlock()

				time.Sleep(cfg.Delay)
			}(pageURL)
		}

		wg.Wait()
		currentLevel = nextLevel
	}

	return result, nil
}

func fetchAndExtract(ctx context.Context, client *http.Client, pageURL *url.URL, baseDomain string, userAgent string) (externalRefs []model.Reference, internalLinks []*url.URL, errs []error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL.String(), nil)
	if err != nil {
		return nil, nil, []error{fmt.Errorf("request %s: %w", pageURL, err)}
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, []error{fmt.Errorf("fetch %s: %w", pageURL, err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
		return nil, nil, nil
	}

	refs := extractReferences(pageURL, resp.Body)

	for i := range refs {
		u, err := url.Parse(refs[i].TargetURL)
		if err != nil {
			continue
		}

		hostname := u.Hostname()
		if hostname == "" {
			continue
		}

		d, err := domain.ExtractRegistrable(hostname)
		if err != nil {
			continue
		}

		if d == baseDomain {
			internalLinks = append(internalLinks, u)
		} else {
			refs[i].Domain = d
			externalRefs = append(externalRefs, refs[i])
		}
	}

	return externalRefs, internalLinks, nil
}

func normalizeURL(u *url.URL) string {
	normalized := *u
	normalized.Fragment = ""
	if normalized.Path != "/" {
		normalized.Path = strings.TrimSuffix(normalized.Path, "/")
	}
	return normalized.String()
}
