package ctstream

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"CuckoosVision/internal/domain"
)

type LogConfig struct {
	URL       string
	BatchSize int64
	Interval  time.Duration
}

func DefaultLogConfig() LogConfig {
	return LogConfig{
		URL:       "https://ct.googleapis.com/logs/us1/argon2026h1",
		BatchSize: 1000,
		Interval:  2 * time.Second,
	}
}

type sthResponse struct {
	TreeSize int64 `json:"tree_size"`
}

type getEntriesResponse struct {
	Entries []ctEntry `json:"entries"`
}

type ctEntry struct {
	LeafInput string `json:"leaf_input"`
	ExtraData string `json:"extra_data"`
}

func PollLog(ctx context.Context, cfg LogConfig, handler Handler) error {
	client := &http.Client{Timeout: 30 * time.Second}

	sth, err := getSTH(ctx, client, cfg.URL)
	if err != nil {
		return fmt.Errorf("get STH: %w", err)
	}

	start := sth.TreeSize - cfg.BatchSize
	if start < 0 {
		start = 0
	}

	seen := make(map[string]struct{})
	log.Printf("CT log at %s, tree size: %d, starting from entry %d", cfg.URL, sth.TreeSize, start)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		sth, err = getSTH(ctx, client, cfg.URL)
		if err != nil {
			log.Printf("STH refresh failed: %v", err)
			sleep(ctx, cfg.Interval)
			continue
		}

		if start >= sth.TreeSize {
			sleep(ctx, cfg.Interval)
			continue
		}

		end := start + cfg.BatchSize - 1
		if end >= sth.TreeSize {
			end = sth.TreeSize - 1
		}

		entries, err := getEntries(ctx, client, cfg.URL, start, end)
		if err != nil {
			log.Printf("fetch entries %d-%d failed: %v", start, end, err)
			sleep(ctx, cfg.Interval)
			continue
		}

		for _, entry := range entries {
			domains := domainsFromEntry(entry)
			for _, d := range domains {
				registrable, err := domain.ExtractRegistrable(d)
				if err != nil {
					continue
				}
				if _, ok := seen[registrable]; ok {
					continue
				}
				seen[registrable] = struct{}{}
				if len(seen) > 100_000 {
					seen = make(map[string]struct{})
				}
				handler(ctx, registrable)
			}
		}

		start = end + 1
		sleep(ctx, cfg.Interval)
	}
}

func getSTH(ctx context.Context, client *http.Client, logURL string) (*sthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", logURL+"/ct/v1/get-sth", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sth sthResponse
	if err := json.NewDecoder(resp.Body).Decode(&sth); err != nil {
		return nil, err
	}
	return &sth, nil
}

func getEntries(ctx context.Context, client *http.Client, logURL string, start, end int64) ([]ctEntry, error) {
	url := fmt.Sprintf("%s/ct/v1/get-entries?start=%d&end=%d", logURL, start, end)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result getEntriesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse entries: %w", err)
	}
	return result.Entries, nil
}

func domainsFromEntry(entry ctEntry) []string {
	raw, err := base64.StdEncoding.DecodeString(entry.LeafInput)
	if err != nil {
		return nil
	}

	// MerkleTreeLeaf: version(1) + leaf_type(1) + timestamp(8) + entry_type(2) + entry_data
	if len(raw) < 12 {
		return nil
	}

	entryType := binary.BigEndian.Uint16(raw[10:12])
	certDER := extractCertDER(raw[12:], entryType)
	if certDER == nil {
		return nil
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil
	}

	var domains []string
	if cert.Subject.CommonName != "" {
		domains = append(domains, cert.Subject.CommonName)
	}
	domains = append(domains, cert.DNSNames...)
	return domains
}

func extractCertDER(data []byte, entryType uint16) []byte {
	switch entryType {
	case 0: // x509_entry: length(3) + certificate
		if len(data) < 3 {
			return nil
		}
		certLen := int(data[0])<<16 | int(data[1])<<8 | int(data[2])
		data = data[3:]
		if len(data) < certLen {
			return nil
		}
		return data[:certLen]

	case 1: // precert_entry: issuer_key_hash(32) + tbs_length(3) + tbs_certificate
		if len(data) < 35 {
			return nil
		}
		tbsLen := int(data[32])<<16 | int(data[33])<<8 | int(data[34])
		data = data[35:]
		if len(data) < tbsLen {
			return nil
		}
		// Pre-certs are TBSCertificate, not full certs – try parsing anyway
		return data[:tbsLen]

	default:
		return nil
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
