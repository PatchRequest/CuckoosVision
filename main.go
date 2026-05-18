package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"CuckoosVision/internal/cache"
	"CuckoosVision/internal/crawler"
	"CuckoosVision/internal/ctstream"
	"CuckoosVision/internal/domain"
	"CuckoosVision/internal/model"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "scan":
		cmdScan(os.Args[2:])
	case "watch":
		cmdWatch(os.Args[2:])
	case "results":
		cmdResults(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: cuckoovision <command> [flags]

Commands:
  scan <url>     Crawl a website and find dangling domain references
  watch          Watch Certificate Transparency logs for new sites to scan
  results        Show stored findings from the cache
`)
}

func defaultCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cuckoovision", "cache.db")
}

func cmdScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	depth := fs.Int("depth", 3, "max crawl depth")
	concurrency := fs.Int("concurrency", 5, "concurrent page fetches")
	delay := fs.Duration("delay", time.Second, "delay between requests to same host")
	cachePath := fs.String("cache", defaultCachePath(), "cache database path")
	jsonOutput := fs.Bool("json", false, "output as JSON")
	minRisk := fs.String("min-risk", "low", "minimum risk level to report (critical/high/medium/low)")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: cuckoovision scan <url> [flags]")
		os.Exit(1)
	}

	targetURL := fs.Arg(0)
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	minRiskLevel, ok := model.ParseRiskLevel(*minRisk)
	if !ok {
		log.Fatalf("invalid risk level: %s (use critical/high/medium/low)", *minRisk)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	store := openCache(*cachePath)
	defer store.Close()

	cfg := crawler.Config{
		MaxDepth:    *depth,
		Concurrency: *concurrency,
		Delay:       *delay,
		UserAgent:   "Mozilla/5.0 (compatible; CuckoosVision/1.0)",
		Timeout:     30 * time.Second,
	}

	fmt.Fprintf(os.Stderr, "Scanning %s (depth=%d, concurrency=%d)\n", targetURL, *depth, *concurrency)

	result, err := crawler.Crawl(ctx, targetURL, cfg)
	if err != nil {
		log.Fatalf("crawl failed: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Crawled %d pages, found %d external domains\n", result.PagesVisited, len(result.Domains))

	findings := checkAndReport(ctx, store, result.References, minRiskLevel)
	printFindings(findings, *jsonOutput)
}

func cmdWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	concurrency := fs.Int("concurrency", 10, "concurrent site crawls")
	cachePath := fs.String("cache", defaultCachePath(), "cache database path")
	source := fs.String("source", "certstream", "CT source: certstream (WebSocket) or ctlog (Google CT log API)")
	certstreamURL := fs.String("certstream-url", "ws://localhost:8080/", "CertStream websocket URL")
	ctlogURL := fs.String("ctlog-url", ctstream.DefaultLogConfig().URL, "CT log URL (only with --source ctlog)")
	scanDepth := fs.Int("scan-depth", 1, "crawl depth for each discovered site")
	minRisk := fs.String("min-risk", "low", "minimum risk level to report")
	scanDelay := fs.Duration("scan-delay", 500*time.Millisecond, "delay between requests when crawling")
	fs.Parse(args)

	minRiskLevel, ok := model.ParseRiskLevel(*minRisk)
	if !ok {
		log.Fatalf("invalid risk level: %s", *minRisk)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	store := openCache(*cachePath)
	defer store.Close()

	scanCfg := crawler.Config{
		MaxDepth:    *scanDepth,
		Concurrency: 3,
		Delay:       *scanDelay,
		UserAgent:   "Mozilla/5.0 (compatible; CuckoosVision/1.0)",
		Timeout:     15 * time.Second,
	}

	work := make(chan string, 200)

	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range work {
				scanSingleDomain(ctx, store, d, scanCfg, minRiskLevel)
			}
		}()
	}

	handler := func(ctx context.Context, d string) {
		select {
		case work <- d:
		default:
		}
	}

	var err error
	switch *source {
	case "ctlog":
		logCfg := ctstream.DefaultLogConfig()
		logCfg.URL = *ctlogURL
		fmt.Fprintf(os.Stderr, "Watching CT log at %s (concurrency=%d, scan-depth=%d)\n", logCfg.URL, *concurrency, *scanDepth)
		err = ctstream.PollLog(ctx, logCfg, handler)
	case "certstream":
		streamCfg := ctstream.Config{URL: *certstreamURL}
		fmt.Fprintf(os.Stderr, "Watching CertStream at %s (concurrency=%d, scan-depth=%d)\n", *certstreamURL, *concurrency, *scanDepth)
		err = ctstream.Stream(ctx, streamCfg, handler)
	default:
		log.Fatalf("unknown source: %s (use ctlog or certstream)", *source)
	}

	close(work)
	wg.Wait()

	if err != nil && ctx.Err() == nil {
		log.Fatalf("stream error: %v", err)
	}
}

func cmdResults(args []string) {
	fs := flag.NewFlagSet("results", flag.ExitOnError)
	cachePath := fs.String("cache", defaultCachePath(), "cache database path")
	jsonOutput := fs.Bool("json", false, "output as JSON")
	minRisk := fs.String("min-risk", "low", "minimum risk level to report")
	sortBy := fs.String("sort", "risk", "sort by: risk, domain, first-seen")
	fs.Parse(args)

	minRiskLevel, ok := model.ParseRiskLevel(*minRisk)
	if !ok {
		log.Fatalf("invalid risk level: %s", *minRisk)
	}

	store := openCache(*cachePath)
	defer store.Close()

	findings, err := store.GetFindings()
	if err != nil {
		log.Fatalf("failed to read findings: %v", err)
	}

	var filtered []model.Finding
	for _, f := range findings {
		if f.RiskLevel <= minRiskLevel {
			filtered = append(filtered, f)
		}
	}

	sortFindings(filtered, *sortBy)
	printFindings(filtered, *jsonOutput)
}

func openCache(path string) *cache.Store {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Fatalf("create cache directory: %v", err)
	}
	store, err := cache.Open(path, 24*time.Hour)
	if err != nil {
		log.Fatalf("open cache: %v", err)
	}
	return store
}

func checkAndReport(ctx context.Context, store *cache.Store, refs []model.Reference, minRiskLevel model.RiskLevel) []model.Finding {
	grouped := groupByDomain(refs)
	var findings []model.Finding

	for d, domainRefs := range grouped {
		status, cached := store.GetDomain(d)
		if !cached {
			var err error
			status, err = domain.CheckAvailability(ctx, d)
			if err != nil {
				log.Printf("check %s: %v", d, err)
				continue
			}
			store.PutDomain(status)
		}

		if !status.Available {
			continue
		}

		risk := highestRisk(domainRefs)
		if risk > minRiskLevel {
			continue
		}

		finding := model.Finding{
			Domain:     d,
			References: domainRefs,
			FirstSeen:  time.Now(),
			RiskLevel:  risk,
		}
		store.AddFinding(finding)
		findings = append(findings, finding)
	}

	sortFindings(findings, "risk")
	return findings
}

func scanSingleDomain(ctx context.Context, store *cache.Store, d string, cfg crawler.Config, minRiskLevel model.RiskLevel) {
	if ctx.Err() != nil {
		return
	}

	targetURL := "https://" + d
	result, err := crawler.Crawl(ctx, targetURL, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[2m✗ %s (error)\033[0m\n", d)
		return
	}

	if len(result.References) == 0 {
		fmt.Fprintf(os.Stderr, "\033[2m· %s  %d pages, 0 external refs\033[0m\n", d, result.PagesVisited)
		return
	}

	findings := checkAndReport(ctx, store, result.References, minRiskLevel)

	if len(findings) == 0 {
		fmt.Fprintf(os.Stderr, "\033[2m· %s  %d pages, %d ext domains, all registered\033[0m\n", d, result.PagesVisited, len(result.Domains))
	} else {
		fmt.Fprintf(os.Stderr, "\033[1;32m★ %s  %d pages, %d ext domains, %d DANGLING:\033[0m\n", d, result.PagesVisited, len(result.Domains), len(findings))
		for _, f := range findings {
			printOneFinding(f)
		}
	}
}

func groupByDomain(refs []model.Reference) map[string][]model.Reference {
	grouped := make(map[string][]model.Reference)
	for _, ref := range refs {
		if ref.Domain != "" {
			grouped[ref.Domain] = append(grouped[ref.Domain], ref)
		}
	}
	return grouped
}

func highestRisk(refs []model.Reference) model.RiskLevel {
	highest := model.RiskLow
	for _, ref := range refs {
		r := ref.ResourceType.RiskLevel()
		if r < highest {
			highest = r
		}
	}
	return highest
}

func sortFindings(findings []model.Finding, by string) {
	switch by {
	case "domain":
		sort.Slice(findings, func(i, j int) bool {
			return findings[i].Domain < findings[j].Domain
		})
	case "first-seen":
		sort.Slice(findings, func(i, j int) bool {
			return findings[i].FirstSeen.Before(findings[j].FirstSeen)
		})
	default:
		sort.Slice(findings, func(i, j int) bool {
			if findings[i].RiskLevel != findings[j].RiskLevel {
				return findings[i].RiskLevel < findings[j].RiskLevel
			}
			return findings[i].Domain < findings[j].Domain
		})
	}
}

func printFindings(findings []model.Finding, asJSON bool) {
	if asJSON {
		if findings == nil {
			findings = []model.Finding{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(findings)
		return
	}

	if len(findings) == 0 {
		fmt.Println("\nNo dangling domains found.")
		return
	}

	fmt.Println()
	for _, f := range findings {
		printOneFinding(f)
	}

	fmt.Printf("\nFound %d dangling domain(s)", len(findings))
	counts := map[model.RiskLevel]int{}
	for _, f := range findings {
		counts[f.RiskLevel]++
	}
	var parts []string
	for _, level := range []model.RiskLevel{model.RiskCritical, model.RiskHigh, model.RiskMedium, model.RiskLow} {
		if c := counts[level]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, strings.ToLower(level.String())))
		}
	}
	if len(parts) > 0 {
		fmt.Printf(" (%s)", strings.Join(parts, ", "))
	}
	fmt.Println()
}

func printOneFinding(f model.Finding) {
	color := riskColor(f.RiskLevel)
	fmt.Printf("%s%-10s\033[0m %s\n", color, f.RiskLevel, f.Domain)
	for _, ref := range f.References {
		fmt.Printf("           <%s %s=\"...\"> on %s\n", ref.Element, ref.Attribute, ref.SourceURL)
	}
}

func riskColor(r model.RiskLevel) string {
	switch r {
	case model.RiskCritical:
		return "\033[1;31m"
	case model.RiskHigh:
		return "\033[31m"
	case model.RiskMedium:
		return "\033[33m"
	case model.RiskLow:
		return "\033[36m"
	default:
		return ""
	}
}
