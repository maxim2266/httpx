package httpx

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipAccepted(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"gzip", true},
		{"", false},
		{"gzip;q=0", false},
		{"gzip;q=0.5", true},
		{"gzip;q=1.000", true},
		{"*", true},
		{"deflate", false},
		{"gzip, deflate", true},
		{"deflate, gzip", true},
		{"br, gzip;q=0.5, *", true},
		{"gzip;q=0.0, deflate", false},
		{"  gzip  ; q=0.8  ", true},     // whitespace handled
		{"BR , GZIP ; Q=0.6 , *", true}, // case-insensitive, wildcard
		{"gzipX", false},                // false positive avoided
		{"xgzip", false},                // false positive avoided
		{"gzip;q=1.5", false},           // invalid q ignored
	}

	for _, tt := range tests {
		if got := gzipAccepted(tt.header); got != tt.want {
			t.Fatalf("CanCompress(%q) = %v; want %v", tt.header, got, tt.want)
		}
	}
}

func TestServeContent(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		headers         map[string]string
		contentMaker    ContentMaker
		expectedStatus  int
		expectedHeaders map[string]string
		expectedBody    string
		expectError     bool
	}{
		// no range header
		{
			name:    "successful response without range",
			method:  http.MethodPost,
			headers: map[string]string{},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("hello world"))
				return "text/plain", nil
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": "11",
			},
			expectedBody: "hello world",
			expectError:  false,
		},
		{
			name:    "empty content without range",
			method:  http.MethodPost,
			headers: map[string]string{},
			contentMaker: func(w io.Writer) (string, error) {
				return "text/plain", nil
			},
			expectedStatus:  http.StatusNoContent,
			expectedHeaders: map[string]string{},
			expectedBody:    "",
			expectError:     false,
		},
		{
			name:    "default content type when empty",
			method:  http.MethodPost,
			headers: map[string]string{},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("binary data"))
				return "", nil
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Type": "application/octet-stream",
			},
			expectedBody: "binary data",
			expectError:  false,
		},

		// content maker errors
		{
			name:    "content maker returns error",
			method:  http.MethodPost,
			headers: map[string]string{},
			contentMaker: func(w io.Writer) (string, error) {
				return "", errors.New("generation failed")
			},
			expectedStatus:  http.StatusInternalServerError,
			expectedHeaders: map[string]string{},
			expectedBody:    "Internal Server Error\n",
			expectError:     true,
		},

		// gzip compression
		{
			name:   "gzip compression when accepted",
			method: http.MethodPost,
			headers: map[string]string{
				"Accept-Encoding": "gzip",
			},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("hello world hello world"))
				return "text/plain", nil
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Encoding": "gzip",
				"Content-Type":     "text/plain",
			},
			expectError: false,
		},
		{
			name:   "no gzip when not accepted",
			method: http.MethodPost,
			headers: map[string]string{
				"Accept-Encoding": "deflate",
			},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("hello world"))
				return "text/plain", nil
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Encoding": "",
				"Content-Type":     "text/plain",
				"Content-Length":   "11",
			},
			expectedBody: "hello world",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			err := ServeContent(w, req, tt.contentMaker)

			if (err != nil) != tt.expectError {
				t.Errorf("ServeContent() error = %v, expectError %v", err, tt.expectError)
			}

			if w.Code != tt.expectedStatus {
				t.Errorf("status code = %v, want %v", w.Code, tt.expectedStatus)
			}

			for k, v := range tt.expectedHeaders {
				if got := w.Header().Get(k); got != v {
					t.Errorf("header %s = %v, want %v", k, got, v)
				}
			}

			// handle gzipped response body
			if tt.expectedHeaders["Content-Encoding"] == "gzip" {
				reader, err := gzip.NewReader(w.Body)

				if err != nil {
					t.Fatalf("failed to create gzip reader: %v", err)
				}

				defer reader.Close()

				decompressed, err := io.ReadAll(reader)

				if err != nil {
					t.Fatalf("failed to decompress: %v", err)
				}

				if len(decompressed) == 0 {
					t.Errorf("decompressed content is empty")
				}
			} else if tt.expectedBody != "" {
				if w.Body.String() != tt.expectedBody {
					t.Errorf("body = %q, want %q", w.Body.String(), tt.expectedBody)
				}
			}
		})
	}
}

// large content to verify file backing
func TestServeLargeContent(t *testing.T) {
	largeData := strings.Repeat("a", 100000) // 100KB, triggers file backing

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()

	err := ServeContent(w, req, func(wr io.Writer) (string, error) {
		wr.Write([]byte(largeData))
		return "text/plain", nil
	})

	if err != nil {
		t.Fatalf("ServeContent failed: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Length") != "100000" {
		t.Errorf("Content-Length = %s, want 100000", w.Header().Get("Content-Length"))
	}

	if w.Body.Len() != 100000 {
		t.Errorf("body length = %d, want 100000", w.Body.Len())
	}
}

func BenchmarkServeContentWithIncreasingSizes(b *testing.B) {
	// Test sizes: from small to large, crossing the 64KB buffer threshold
	sizes := []struct {
		name  string
		bytes int
	}{
		{"1KB", 1 * 1024},
		{"8KB", 8 * 1024},
		{"32KB", 32 * 1024},
		{"64KB", 64 * 1024}, // buffer limit
		{"65KB", 65 * 1024}, // just over - file backing
		{"128KB", 128 * 1024},
		{"256KB", 256 * 1024},
		{"512KB", 512 * 1024},
		{"1MB", 1 * 1024 * 1024},
		{"2MB", 2 * 1024 * 1024},
		{"4MB", 4 * 1024 * 1024},
		{"8MB", 8 * 1024 * 1024},
		{"16MB", 16 * 1024 * 1024},
		{"32MB", 32 * 1024 * 1024},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			testData := bytes.Repeat([]byte("x"), size.bytes)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var w discardWriter

				err := ServeContent(&w, req, func(wr io.Writer) (string, error) {
					wr.Write(testData)
					return "text/plain", nil
				})

				if err != nil {
					b.Fatalf("ServeContent failed: %v", err)
				}

				// verify content length
				if w.size != size.bytes {
					b.Fatalf("size mismatch: %d instead of %d", w.size, size.bytes)
				}
			}
		})
	}
}

// helpers
type discardWriter struct {
	http.ResponseWriter
	size int
}

func (w *discardWriter) Write(p []byte) (int, error) {
	w.size += len(p)
	return len(p), nil
}

func (w *discardWriter) Header() http.Header {
	if w.ResponseWriter == nil {
		return make(http.Header)
	}
	return w.ResponseWriter.Header()
}

func (w *discardWriter) WriteHeader(_ int) {
	// do nothing
}
