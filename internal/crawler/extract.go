package crawler

import (
	"io"
	"net/url"
	"strings"

	"CuckoosVision/internal/model"

	"golang.org/x/net/html"
)

func extractReferences(pageURL *url.URL, body io.Reader) []model.Reference {
	tokenizer := html.NewTokenizer(body)
	base := pageURL
	var refs []model.Reference

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}

		token := tokenizer.Token()
		element := token.Data

		if element == "base" {
			if href := getAttr(token.Attr, "href"); href != "" {
				if u, err := url.Parse(href); err == nil {
					base = pageURL.ResolveReference(u)
				}
			}
			continue
		}

		for _, attr := range token.Attr {
			if !isResourceAttr(element, attr.Key) {
				continue
			}

			if attr.Key == "srcset" {
				for _, src := range parseSrcset(attr.Val) {
					if ref, ok := makeReference(base, element, attr.Key, src, token.Attr); ok {
						refs = append(refs, ref)
					}
				}
				continue
			}

			if ref, ok := makeReference(base, element, attr.Key, attr.Val, token.Attr); ok {
				refs = append(refs, ref)
			}
		}
	}

	return refs
}

func isResourceAttr(element, attr string) bool {
	switch attr {
	case "src":
		return true
	case "href":
		return element == "a" || element == "link"
	case "srcset":
		return element == "img" || element == "source"
	case "data":
		return element == "object"
	case "action":
		return element == "form"
	default:
		return false
	}
}

func classifyResource(element string, attrs []html.Attribute) model.ResourceType {
	switch element {
	case "script":
		return model.ResourceScript
	case "iframe":
		return model.ResourceIframe
	case "img", "picture":
		return model.ResourceImage
	case "video", "audio", "source":
		return model.ResourceMedia
	case "object", "embed":
		return model.ResourceObject
	case "a":
		return model.ResourceLink
	case "form":
		return model.ResourceOther
	case "link":
		rel := strings.ToLower(getAttr(attrs, "rel"))
		switch rel {
		case "stylesheet":
			return model.ResourceStylesheet
		case "preload", "modulepreload":
			switch getAttr(attrs, "as") {
			case "script":
				return model.ResourceScript
			case "style":
				return model.ResourceStylesheet
			case "image":
				return model.ResourceImage
			default:
				return model.ResourceOther
			}
		case "icon", "apple-touch-icon", "shortcut icon":
			return model.ResourceImage
		default:
			return model.ResourceOther
		}
	default:
		return model.ResourceOther
	}
}

func makeReference(base *url.URL, element, attr, rawURL string, attrs []html.Attribute) (model.Reference, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return model.Reference{}, false
	}
	for _, prefix := range []string{"data:", "javascript:", "mailto:", "tel:", "#"} {
		if strings.HasPrefix(rawURL, prefix) {
			return model.Reference{}, false
		}
	}

	resolved := resolveURL(base, rawURL)
	if resolved == nil {
		return model.Reference{}, false
	}

	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return model.Reference{}, false
	}

	return model.Reference{
		SourceURL:    base.String(),
		TargetURL:    resolved.String(),
		ResourceType: classifyResource(element, attrs),
		Element:      element,
		Attribute:    attr,
	}, true
}

func resolveURL(base *url.URL, raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return base.ResolveReference(u)
}

func getAttr(attrs []html.Attribute, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func parseSrcset(srcset string) []string {
	var urls []string
	for _, part := range strings.Split(srcset, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) > 0 {
			urls = append(urls, fields[0])
		}
	}
	return urls
}
