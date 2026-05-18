# CuckoosVision

Dangling domain detector. Crawls websites, extracts all external resource references (scripts, stylesheets, iframes, images, links), and checks if the referenced domains are still registered. Unregistered domains are supply chain takeover candidates.

## Install

```bash
CGO_ENABLED=0 go build -o cuckoovision .
```

## Usage

### Scan a single site

```bash
./cuckoovision scan example.com
./cuckoovision scan --depth 2 --json example.com
./cuckoovision scan --depth 1 --min-risk critical example.com
```

### Watch Certificate Transparency logs

Monitors CT logs for newly certified domains, crawls each one, and reports dangling references in real time.

```bash
# Start self-hosted CertStream (requires Docker)
docker compose up -d

# Watch (connects to CertStream on localhost:8080)
./cuckoovision watch

# Or use Google CT log API directly (no Docker needed)
./cuckoovision watch --source ctlog
```

### View stored results

```bash
./cuckoovision results
./cuckoovision results --json
./cuckoovision results --min-risk high --sort domain
```

## Risk levels

| Level | Resource types | Threat |
|-------|---------------|--------|
| CRITICAL | `<script src>`, `<iframe src>` | Full code execution |
| HIGH | `<link rel=stylesheet>` | CSS injection, data exfil |
| MEDIUM | `<img src>`, `<video>`, `<audio>` | Phishing, defacement |
| LOW | `<a href>` | Redirect, phishing |

## Architecture

- **Domain checking**: DNS NS + A/AAAA lookups (fast, reliable, no rate limits)
- **Domain extraction**: Public Suffix List via `golang.org/x/net/publicsuffix`
- **HTML parsing**: `golang.org/x/net/html` tokenizer
- **Cache**: bbolt key-value store (`~/.cuckoovision/cache.db`, 24h TTL)
- **CT stream**: Self-hosted CertStream (WebSocket) or Google CT log API

## Flags

### scan

| Flag | Default | Description |
|------|---------|-------------|
| `--depth` | 3 | Max crawl depth |
| `--concurrency` | 5 | Concurrent page fetches |
| `--delay` | 1s | Delay between requests |
| `--cache` | `~/.cuckoovision/cache.db` | Cache path |
| `--json` | false | JSON output |
| `--min-risk` | low | Minimum risk level |

### watch

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | certstream | `certstream` or `ctlog` |
| `--certstream-url` | `ws://localhost:8080/` | CertStream WebSocket URL |
| `--ctlog-url` | Google Argon 2026h1 | CT log URL |
| `--concurrency` | 10 | Concurrent site crawls |
| `--scan-depth` | 1 | Crawl depth per site |
| `--min-risk` | low | Minimum risk level |

### results

| Flag | Default | Description |
|------|---------|-------------|
| `--cache` | `~/.cuckoovision/cache.db` | Cache path |
| `--json` | false | JSON output |
| `--min-risk` | low | Minimum risk level |
| `--sort` | risk | Sort by: risk, domain, first-seen |
