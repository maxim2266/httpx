package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAcceptable(t *testing.T) {
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
			got := isAcceptable(tt.headers, tt.targetType)

			if got != tt.want {
				t.Fatalf("isAcceptable(%v, %q) = %v, want %v",
					tt.headers, tt.targetType, got, tt.want)
			}
		})
	}
}

func TestServeJson(t *testing.T) {
	tests := []struct {
		name           string
		acceptHeader   string
		obj            any
		expectedStatus int
		expectedBody   string
		expectError    bool
	}{
		{
			name:           "valid JSON request with correct Accept header",
			acceptHeader:   "application/json",
			obj:            map[string]string{"message": "hello"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"hello"}` + "\n",
			expectError:    false,
		},
		{
			name:           "valid JSON request with Accept header containing multiple types",
			acceptHeader:   "text/html, application/json, */*",
			obj:            []int{1, 2, 3},
			expectedStatus: http.StatusOK,
			expectedBody:   `[1,2,3]` + "\n",
			expectError:    false,
		},
		{
			name:           "valid JSON request with */ Accept header",
			acceptHeader:   "*/*",
			obj:            struct{ Name string }{"test"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"Name":"test"}` + "\n",
			expectError:    false,
		},
		{
			name:           "invalid Accept header - wrong type",
			acceptHeader:   "application/xml",
			obj:            map[string]string{"message": "hello"},
			expectedStatus: http.StatusNotAcceptable,
			expectedBody:   "Not Acceptable\n",
			expectError:    true,
		},
		{
			name:           "invalid Accept header - text/plain only",
			acceptHeader:   "text/plain",
			obj:            map[string]string{"message": "hello"},
			expectedStatus: http.StatusNotAcceptable,
			expectedBody:   "Not Acceptable\n",
			expectError:    true,
		},
		{
			name:           "empty Accept header - should accept JSON",
			acceptHeader:   "",
			obj:            map[string]string{"message": "hello"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"hello"}` + "\n",
			expectError:    false,
		},
		{
			name:           "nil object - valid JSON",
			acceptHeader:   "application/json",
			obj:            nil,
			expectedStatus: http.StatusOK,
			expectedBody:   "null\n",
			expectError:    false,
		},
		{
			name:           "primitive type - int",
			acceptHeader:   "application/json",
			obj:            42,
			expectedStatus: http.StatusOK,
			expectedBody:   "42\n",
			expectError:    false,
		},
		{
			name:           "primitive type - string",
			acceptHeader:   "application/json",
			obj:            "hello",
			expectedStatus: http.StatusOK,
			expectedBody:   `"hello"` + "\n",
			expectError:    false,
		},
		{
			name:           "Accept header with quality values",
			acceptHeader:   "text/html;q=0.8, application/json;q=0.9",
			obj:            map[string]string{"message": "hello"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"hello"}` + "\n",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}

			w := httptest.NewRecorder()
			err := ServeJson(w, req, tt.obj)

			if (err != nil) != tt.expectError {
				t.Fatalf("ServeJson() error = %v, expectError %v", err, tt.expectError)
			}

			if w.Code != tt.expectedStatus {
				t.Fatalf("status code = %v, want %v", w.Code, tt.expectedStatus)
			}

			// for successful responses, verify JSON content
			if tt.expectedStatus == http.StatusOK {
				// verify Content-Type header
				if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
					t.Fatalf("Content-Type = %v, want application/json", contentType)
				}

				// verify body is valid JSON
				var result any
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatalf("response body is not valid JSON: %v", err)
				}

				// verify body content (as string)
				if w.Body.String() != tt.expectedBody {
					t.Fatalf("body = %q, want %q", w.Body.String(), tt.expectedBody)
				}
			} else {
				// for error responses, verify error body
				if w.Body.String() != tt.expectedBody {
					t.Fatalf("body = %q, want %q", w.Body.String(), tt.expectedBody)
				}
			}
		})
	}
}

// Test that ServeJson properly handles encoding errors (e.g., channel)
func TestServeJsonEncodingError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")

	w := httptest.NewRecorder()

	// channel cannot be encoded to JSON
	obj := make(chan int)

	err := ServeJson(w, req, obj)

	if err == nil {
		t.Fatal("ServeJson() expected error from JSON encoding, got nil")
	}

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %v, want 500", w.Code)
	}
}

// Test that ServeJson passes through the underlying ServeContent behavior
// for range requests (if supported)
func TestServeJsonWithAcceptHeaderVariants(t *testing.T) {
	variants := []struct {
		name         string
		acceptHeader string
		shouldAccept bool
	}{
		{"exact match", "application/json", true},
		{"with charset", "application/json; charset=utf-8", true},
		{"wildcard", "*/*", true},
		{"partial match", "application/*", true},
		{"wrong type", "text/plain", false},
		{"multiple types with JSON", "text/html, application/json, application/xml", true},
		{"quality values with JSON higher", "text/html;q=0.5, application/json;q=0.9", true},
		{"quality values with JSON lower", "application/json;q=0.2, text/html;q=0.9", true}, // Still acceptable
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept", v.acceptHeader)

			w := httptest.NewRecorder()
			err := ServeJson(w, req, map[string]string{"test": "value"})

			if v.shouldAccept {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if w.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d", w.Code)
				}
				if w.Header().Get("Content-Type") != "application/json" {
					t.Fatalf("Content-Type = %v, want application/json", w.Header().Get("Content-Type"))
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if w.Code != http.StatusNotAcceptable {
					t.Fatalf("expected 406, got %d", w.Code)
				}
			}
		})
	}
}
