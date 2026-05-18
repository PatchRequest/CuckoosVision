package domain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"CuckoosVision/internal/model"

	"golang.org/x/net/publicsuffix"
)

func ExtractRegistrable(hostname string) (string, error) {
	hostname = strings.TrimSuffix(hostname, ".")
	hostname = strings.ToLower(hostname)

	if net.ParseIP(hostname) != nil {
		return "", fmt.Errorf("hostname is an IP address: %s", hostname)
	}

	if !strings.Contains(hostname, ".") {
		return "", fmt.Errorf("single-label hostname: %s", hostname)
	}

	if !isValidHostname(hostname) {
		return "", fmt.Errorf("invalid hostname: %s", hostname)
	}

	d, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		return "", fmt.Errorf("extract registrable domain from %s: %w", hostname, err)
	}

	tld, icann := publicsuffix.PublicSuffix(hostname)
	if !icann && !strings.Contains(tld, ".") {
		return "", fmt.Errorf("unknown TLD in %s", hostname)
	}

	return d, nil
}

func isValidHostname(h string) bool {
	for _, r := range h {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			continue
		}
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
	}
	return true
}

func ExtractFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	hostname := u.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("no hostname in URL: %s", rawURL)
	}
	return ExtractRegistrable(hostname)
}

func CheckAvailability(ctx context.Context, d string) (model.DomainStatus, error) {
	status := model.DomainStatus{
		Domain:    d,
		CheckedAt: time.Now(),
	}

	ns, nsErr := net.DefaultResolver.LookupNS(ctx, d)
	if nsErr == nil && len(ns) > 0 {
		return status, nil
	}

	addrs, hostErr := net.DefaultResolver.LookupHost(ctx, d)
	if hostErr == nil && len(addrs) > 0 {
		return status, nil
	}

	var nxdomain bool
	for _, err := range []error{nsErr, hostErr} {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			nxdomain = true
			break
		}
	}

	status.Available = nxdomain

	if !nxdomain && nsErr != nil && hostErr != nil {
		return status, fmt.Errorf("DNS lookups for %s failed: %v / %v", d, nsErr, hostErr)
	}

	return status, nil
}

type CheckResult struct {
	Status model.DomainStatus
	Err    error
}

func CheckMany(ctx context.Context, domains []string, concurrency int) []CheckResult {
	results := make([]CheckResult, len(domains))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, d := range domains {
		wg.Add(1)
		go func(idx int, dom string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			status, err := CheckAvailability(ctx, dom)
			results[idx] = CheckResult{Status: status, Err: err}
		}(i, d)
	}

	wg.Wait()
	return results
}
