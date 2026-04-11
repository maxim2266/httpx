package httpx

import (
	"strings"
	"testing"
)

func TestMatchContentType(t *testing.T) {
	tests := []struct {
		name       string
		headers    []string
		targetType string
		want       bool
	}{
		// basic cases
		{
			name:       "empty headers",
			headers:    []string{},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "empty target",
			headers:    []string{"text/html"},
			targetType: "",
			want:       false,
		},
		{
			name:       "exact match",
			headers:    []string{"text/html"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "no match",
			headers:    []string{"application/json"},
			targetType: "text/html",
			want:       false,
		},

		// wildcard cases
		{
			name:       "subtype wildcard match",
			headers:    []string{"text/*"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "subtype wildcard no match",
			headers:    []string{"text/*"},
			targetType: "application/json",
			want:       false,
		},
		{
			name:       "full wildcard match",
			headers:    []string{"*/*"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "full wildcard with other types",
			headers:    []string{"application/json", "*/*"},
			targetType: "text/html",
			want:       true,
		},

		// quality value cases
		{
			name:       "q=1.0 accepted",
			headers:    []string{"text/html;q=1.0"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "q=0.5 accepted",
			headers:    []string{"text/html;q=0.5"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "q=0.001 accepted (minimum valid)",
			headers:    []string{"text/html;q=0.001"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "q=0 rejected (not acceptable)",
			headers:    []string{"text/html;q=0"},
			targetType: "text/html",
			want:       false,
		},
		{
			name:       "q=0.000 rejected",
			headers:    []string{"text/html;q=0.000"},
			targetType: "text/html",
			want:       false,
		},
		{
			name:       "invalid q skipped, falls back to next entry",
			headers:    []string{"text/html;q=invalid", "text/plain"},
			targetType: "text/plain",
			want:       true,
		},
		{
			name:       "q > 1.0 skipped",
			headers:    []string{"text/html;q=1.5"},
			targetType: "text/html",
			want:       false,
		},
		{
			name:       "q negative skipped",
			headers:    []string{"text/html;q=-0.5"},
			targetType: "text/html",
			want:       false,
		},

		// multiple headers and values
		{
			name:       "match in second header",
			headers:    []string{"application/json", "text/html"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "match in second value of same header",
			headers:    []string{"application/json, text/html"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "first value q=0, second value accepted",
			headers:    []string{"text/html;q=0, text/html;q=1.0"},
			targetType: "text/html",
			want:       true,
		},

		// edge cases
		{
			name:       "malformed media type skipped",
			headers:    []string{"invalid;q=1.0", "text/html"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "empty part skipped",
			headers:    []string{"text/html, , application/json"},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "whitespace handled correctly",
			headers:    []string{"  text/html  ;  q=0.8  "},
			targetType: "text/html",
			want:       true,
		},
		{
			name:       "case insensitivity per RFC 2045",
			headers:    []string{"Text/HTML"},
			targetType: "text/html",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchContentType(tt.headers, tt.targetType)

			if got != tt.want {
				t.Fatalf("MatchContentType(%v, %q) = %v, want %v",
					tt.headers, tt.targetType, got, tt.want)
			}
		})
	}
}

func TestSplitMediaType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMain string
		wantSub  string
		wantOk   bool
	}{
		// valid cases - minimum length
		{
			name:     "minimum valid - single char each",
			input:    "a/b",
			wantMain: "a",
			wantSub:  "b",
			wantOk:   true,
		},
		{
			name:     "digit first char allowed",
			input:    "0/1",
			wantMain: "0",
			wantSub:  "1",
			wantOk:   true,
		},
		{
			name:     "uppercase allowed",
			input:    "A/B",
			wantMain: "A",
			wantSub:  "B",
			wantOk:   true,
		},

		// valid cases - maximum length
		{
			name:     "maximum length - 127 chars each",
			input:    strings.Repeat("a", 127) + "/" + strings.Repeat("b", 127),
			wantMain: strings.Repeat("a", 127),
			wantSub:  strings.Repeat("b", 127),
			wantOk:   true,
		},
		{
			name:     "allowed special characters",
			input:    "a!#$&-^_.+0/b!#$&-^_.+0",
			wantMain: "a!#$&-^_.+0",
			wantSub:  "b!#$&-^_.+0",
			wantOk:   true,
		},

		// invalid cases - length boundaries
		{
			name:   "main too long - 128 chars",
			input:  strings.Repeat("a", 128) + "/b",
			wantOk: false,
		},
		{
			name:   "sub too long - 128 chars",
			input:  "a/" + strings.Repeat("b", 128),
			wantOk: false,
		},
		{
			name:   "both too long",
			input:  strings.Repeat("a", 128) + "/" + strings.Repeat("b", 128),
			wantOk: false,
		},
		{
			name:   "main exactly 128 with slash at 128",
			input:  strings.Repeat("a", 128) + "/",
			wantOk: false,
		},

		// invalid cases - first character rules
		{
			name:   "main starts with invalid char - hyphen",
			input:  "-a/b",
			wantOk: false,
		},
		{
			name:   "main starts with invalid char - dot",
			input:  ".a/b",
			wantOk: false,
		},
		{
			name:   "main starts with invalid char - plus",
			input:  "+a/b",
			wantOk: false,
		},
		{
			name:   "sub starts with invalid char",
			input:  "a/-b",
			wantOk: false,
		},

		// invalid cases - invalid characters
		{
			name:   "main contains space",
			input:  "a b/c",
			wantOk: false,
		},
		{
			name:   "main contains asterisk",
			input:  "a*/b",
			wantOk: false,
		},
		{
			name:   "main contains slash in wrong place",
			input:  "a/b/c",
			wantOk: false,
		},
		{
			name:   "main contains backslash",
			input:  "a\\/b",
			wantOk: false,
		},
		{
			name:   "sub contains invalid char",
			input:  "a/b@c",
			wantOk: false,
		},
		{
			name:   "sub contains semicolon",
			input:  "a/b;c",
			wantOk: false,
		},

		// invalid cases - malformed structure
		{
			name:   "empty string",
			input:  "",
			wantOk: false,
		},
		{
			name:   "too short - single char",
			input:  "a",
			wantOk: false,
		},
		{
			name:   "too short - two chars no slash",
			input:  "ab",
			wantOk: false,
		},
		{
			name:   "missing main",
			input:  "/b",
			wantOk: false,
		},
		{
			name:   "missing sub",
			input:  "a/",
			wantOk: false,
		},
		{
			name:   "only slash",
			input:  "/",
			wantOk: false,
		},
		{
			name:   "empty main and sub",
			input:  "/",
			wantOk: false,
		},

		// edge cases - exact length boundaries
		{
			name:     "main 126 chars - valid",
			input:    strings.Repeat("a", 126) + "/b",
			wantMain: strings.Repeat("a", 126),
			wantSub:  "b",
			wantOk:   true,
		},
		{
			name:     "main 127 chars - valid max",
			input:    strings.Repeat("a", 127) + "/b",
			wantMain: strings.Repeat("a", 127),
			wantSub:  "b",
			wantOk:   true,
		},
		{
			name:   "main 128 chars - invalid",
			input:  strings.Repeat("a", 128) + "/b",
			wantOk: false,
		},
		{
			name:     "sub 126 chars - valid",
			input:    "a/" + strings.Repeat("b", 126),
			wantMain: "a",
			wantSub:  strings.Repeat("b", 126),
			wantOk:   true,
		},
		{
			name:     "sub 127 chars - valid max",
			input:    "a/" + strings.Repeat("b", 127),
			wantMain: "a",
			wantSub:  strings.Repeat("b", 127),
			wantOk:   true,
		},
		{
			name:   "sub 128 chars - invalid",
			input:  "a/" + strings.Repeat("b", 128),
			wantOk: false,
		},

		// no slash scenarios with different lengths
		{
			name:   "no slash - 127 chars",
			input:  strings.Repeat("a", 127),
			wantOk: false,
		},
		{
			name:   "no slash - 128 chars",
			input:  strings.Repeat("a", 128),
			wantOk: false,
		},
		{
			name:   "no slash - 129 chars",
			input:  strings.Repeat("a", 129),
			wantOk: false,
		},
		{
			name:   "no slash - 130 chars",
			input:  strings.Repeat("a", 130),
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMain, gotSub, gotOk := splitMediaType(tt.input)

			if gotOk != tt.wantOk {
				t.Fatalf("splitMediaType(%q) ok = %v, want %v", tt.input, gotOk, tt.wantOk)
			}

			if gotOk && tt.wantOk {
				if gotMain != tt.wantMain {
					t.Fatalf("splitMediaType(%q) main = %q, want %q", tt.input, gotMain, tt.wantMain)
				}
				if gotSub != tt.wantSub {
					t.Fatalf("splitMediaType(%q) sub = %q, want %q", tt.input, gotSub, tt.wantSub)
				}
			}
		})
	}
}
