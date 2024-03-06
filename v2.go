package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gocolly/colly/v2"
	"github.com/haccer/available"
)

var (
	globalVerbose bool
	doLogging     bool
	target        string
	depth         int
)

func appendToFile(filePath string, content string) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content + "\n")
	if err != nil {
		return err
	}

	return nil
}
func extractDomain(url string) string {
	parts := strings.Split(url, ".")
	if len(parts) >= 2 {
		domain := strings.Join(parts[len(parts)-2:], ".")
		return domain
	}
	return ""
}

func checkLink(e *colly.HTMLElement, checkEl string, n *map[string]bool) {

	m := *n
	full := e.Request.AbsoluteURL(e.Attr(checkEl))
	//fmt.Println(full)
	u, err := url.Parse(full)
	if err != nil {
		fmt.Println("Error parsing URL:", err)
		return
	}
	domain := u.Hostname()
	domain = extractDomain(domain)

	exists := m[domain]
	if !exists {
		m[domain] = true
		if available.Domain(domain) {
			color.Green("****** HIT! ******")
			color.Green(e.Response.Request.URL.Path)
			color.Green(full)
			color.Green(domain)
			color.Green("******************")
			if doLogging {
				appendToFile("log.log", target+e.Response.Request.URL.Path)
				appendToFile("log.log", full)
				appendToFile("log.log", domain)
				appendToFile("log.log", "\n")
			}

		} else {
			if globalVerbose {
				color.Red(domain)
			}

		}
	}

	e.Request.Visit(full)

}

func main() {
	flag.BoolVar(&globalVerbose, "verbose", true, "Print not just hits")
	flag.BoolVar(&doLogging, "log", false, "Log to file")
	flag.StringVar(&target, "target", "google.com", "Target URL")
	flag.IntVar(&depth, "depth", 3, "How deep should it follow links?")
	flag.Parse()

	target := target
	u, err := url.Parse(target)
	if err != nil {
		fmt.Println(err)
		return
	}

	m := make(map[string]bool)
	_ = m
	c := colly.NewCollector(
		colly.AllowedDomains(u.Hostname(), "www."+u.Hostname()),
		colly.Async(false),
		colly.MaxDepth(depth),
		colly.UserAgent("Mozilla/5.0 (Linux; Android 13; SM-S901B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Mobile Safari/537.36"),
	)

	c.Limit(&colly.LimitRule{
		RandomDelay: 2 * time.Second,
		Parallelism: 2,
		DomainGlob:  "*" + u.Hostname() + "*",
	})

	c.OnHTML("[href]", func(e *colly.HTMLElement) {
		checkLink(e, "href", &m)

	})
	c.OnHTML("[src]", func(e *colly.HTMLElement) {
		checkLink(e, "src", &m)

	})

	c.Visit(target)
	c.Wait()

}
