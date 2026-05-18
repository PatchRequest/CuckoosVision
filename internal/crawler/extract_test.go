package crawler

import (
	"net/url"
	"strings"
	"testing"

	"CuckoosVision/internal/model"
)

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func TestExtractReferences(t *testing.T) {
	base := mustParseURL("https://example.com/page")

	tests := []struct {
		name     string
		html     string
		wantLen  int
		wantType model.ResourceType
		wantElem string
		wantURL  string
	}{
		{
			name:     "script src",
			html:     `<script src="https://cdn.other.com/app.js"></script>`,
			wantLen:  1,
			wantType: model.ResourceScript,
			wantElem: "script",
			wantURL:  "https://cdn.other.com/app.js",
		},
		{
			name:     "stylesheet link",
			html:     `<link rel="stylesheet" href="https://fonts.googleapis.com/css">`,
			wantLen:  1,
			wantType: model.ResourceStylesheet,
			wantElem: "link",
			wantURL:  "https://fonts.googleapis.com/css",
		},
		{
			name:     "iframe src",
			html:     `<iframe src="https://embed.other.com/widget"></iframe>`,
			wantLen:  1,
			wantType: model.ResourceIframe,
			wantElem: "iframe",
			wantURL:  "https://embed.other.com/widget",
		},
		{
			name:     "image src",
			html:     `<img src="https://images.other.com/photo.jpg">`,
			wantLen:  1,
			wantType: model.ResourceImage,
			wantElem: "img",
			wantURL:  "https://images.other.com/photo.jpg",
		},
		{
			name:     "anchor href",
			html:     `<a href="https://other.com/page">Link</a>`,
			wantLen:  1,
			wantType: model.ResourceLink,
			wantElem: "a",
			wantURL:  "https://other.com/page",
		},
		{
			name:    "multiple references",
			html:    `<script src="https://a.com/1.js"></script><link rel="stylesheet" href="https://b.com/2.css"><a href="https://c.com">X</a>`,
			wantLen: 3,
		},
		{
			name:    "data URI ignored",
			html:    `<img src="data:image/png;base64,abc123">`,
			wantLen: 0,
		},
		{
			name:    "javascript URI ignored",
			html:    `<a href="javascript:void(0)">Click</a>`,
			wantLen: 0,
		},
		{
			name:    "mailto ignored",
			html:    `<a href="mailto:test@example.com">Mail</a>`,
			wantLen: 0,
		},
		{
			name:    "empty href ignored",
			html:    `<a href="">Empty</a>`,
			wantLen: 0,
		},
		{
			name:    "fragment-only ignored",
			html:    `<a href="#section">Anchor</a>`,
			wantLen: 0,
		},
		{
			name:     "relative URL resolved",
			html:     `<script src="/scripts/app.js"></script>`,
			wantLen:  1,
			wantURL:  "https://example.com/scripts/app.js",
			wantType: model.ResourceScript,
		},
		{
			name:     "protocol-relative URL",
			html:     `<script src="//cdn.other.com/lib.js"></script>`,
			wantLen:  1,
			wantURL:  "https://cdn.other.com/lib.js",
			wantType: model.ResourceScript,
		},
		{
			name:     "srcset parsed",
			html:     `<img srcset="https://img.other.com/small.jpg 300w, https://img.other.com/large.jpg 800w">`,
			wantLen:  2,
			wantType: model.ResourceImage,
		},
		{
			name:     "link preload script",
			html:     `<link rel="preload" as="script" href="https://cdn.other.com/chunk.js">`,
			wantLen:  1,
			wantType: model.ResourceScript,
		},
		{
			name:     "link icon",
			html:     `<link rel="icon" href="https://cdn.other.com/favicon.ico">`,
			wantLen:  1,
			wantType: model.ResourceImage,
		},
		{
			name:     "video source",
			html:     `<video><source src="https://media.other.com/video.mp4"></video>`,
			wantLen:  1,
			wantType: model.ResourceMedia,
		},
		{
			name:     "object data",
			html:     `<object data="https://cdn.other.com/widget.swf"></object>`,
			wantLen:  1,
			wantType: model.ResourceObject,
			wantElem: "object",
		},
		{
			name:     "form action",
			html:     `<form action="https://submit.other.com/handler"></form>`,
			wantLen:  1,
			wantType: model.ResourceOther,
			wantElem: "form",
		},
		{
			name:     "base tag changes resolution",
			html:     `<base href="https://other-base.com/"><script src="/app.js"></script>`,
			wantLen:  1,
			wantURL:  "https://other-base.com/app.js",
			wantType: model.ResourceScript,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := extractReferences(base, strings.NewReader(tt.html))

			if len(refs) != tt.wantLen {
				t.Fatalf("got %d references, want %d: %+v", len(refs), tt.wantLen, refs)
			}

			if tt.wantLen == 0 {
				return
			}

			ref := refs[0]
			if tt.wantType != 0 || tt.wantElem != "" || tt.wantURL != "" {
				if tt.wantURL != "" && ref.TargetURL != tt.wantURL {
					t.Errorf("TargetURL = %q, want %q", ref.TargetURL, tt.wantURL)
				}
				if tt.wantType != ref.ResourceType && tt.wantLen == 1 {
					t.Errorf("ResourceType = %v, want %v", ref.ResourceType, tt.wantType)
				}
				if tt.wantElem != "" && ref.Element != tt.wantElem {
					t.Errorf("Element = %q, want %q", ref.Element, tt.wantElem)
				}
			}
		})
	}
}

func TestClassifyResource(t *testing.T) {
	tests := []struct {
		element string
		attrs   string
		want    model.ResourceType
	}{
		{"script", "", model.ResourceScript},
		{"iframe", "", model.ResourceIframe},
		{"img", "", model.ResourceImage},
		{"video", "", model.ResourceMedia},
		{"audio", "", model.ResourceMedia},
		{"a", "", model.ResourceLink},
		{"object", "", model.ResourceObject},
		{"embed", "", model.ResourceObject},
	}

	for _, tt := range tests {
		t.Run(tt.element, func(t *testing.T) {
			got := classifyResource(tt.element, nil)
			if got != tt.want {
				t.Errorf("classifyResource(%q) = %v, want %v", tt.element, got, tt.want)
			}
		})
	}
}
