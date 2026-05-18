package domain

import (
	"testing"
)

func TestExtractRegistrable(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"example.com", "example.com", false},
		{"www.example.com", "example.com", false},
		{"cdn.assets.example.com", "example.com", false},
		{"example.co.uk", "example.co.uk", false},
		{"www.example.co.uk", "example.co.uk", false},
		{"sub.example.co.uk", "example.co.uk", false},
		{"example.com.au", "example.com.au", false},
		{"www.example.com.au", "example.com.au", false},
		{"example.org", "example.org", false},
		{"deep.sub.example.org", "example.org", false},
		{"example.co.jp", "example.co.jp", false},
		{"example.com.br", "example.com.br", false},

		// Trailing dot
		{"example.com.", "example.com", false},

		// Case insensitive
		{"WWW.EXAMPLE.COM", "example.com", false},

		// Errors
		{"192.168.1.1", "", true},           // IP address
		{"::1", "", true},                    // IPv6
		{"localhost", "", true},              // single label
		{"com", "", true},                    // TLD only
		{"", "", true},                       // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ExtractRegistrable(tt.input)
			if tt.err {
				if err == nil {
					t.Errorf("expected error for %q, got %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractRegistrable(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractFromURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"https://www.example.com/path", "example.com", false},
		{"http://cdn.example.co.uk/style.css", "example.co.uk", false},
		{"https://example.com", "example.com", false},
		{"//example.com/foo", "example.com", false},
		{"/relative/path", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ExtractFromURL(tt.input)
			if tt.err {
				if err == nil {
					t.Errorf("expected error for %q, got %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractFromURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
