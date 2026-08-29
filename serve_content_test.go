package httpx

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
			t.Fatalf("gzipAccepted(%q) = %v; want %v", tt.header, got, tt.want)
		}
	}
}

func TestServeContent(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		headers         map[string]string
		contentMaker    func(io.Writer) error
		contentType     string
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
			contentMaker: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("hello world"))
				return
			},
			contentType:    "text/plain",
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
			contentMaker: func(_ io.Writer) error {
				return nil
			},
			contentType:     "text/plain",
			expectedStatus:  http.StatusNoContent,
			expectedHeaders: map[string]string{},
			expectedBody:    "",
			expectError:     false,
		},

		// content maker errors
		{
			name:    "content maker returns error",
			method:  http.MethodPost,
			headers: map[string]string{},
			contentMaker: func(w io.Writer) error {
				return errors.New("generation failed")
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
			contentMaker: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("hello world hello world"))
				return
			},
			contentType:    "text/plain",
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Encoding": "gzip",
				"Content-Type":     "text/plain",
			},
			expectError:  false,
			expectedBody: "hello world hello world",
		},
		{
			name:   "gzip compression with compressor reuse",
			method: http.MethodPost,
			headers: map[string]string{
				"Accept-Encoding": "gzip",
			},
			contentMaker: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"))
				return
			},
			contentType:    "text/plain",
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Encoding": "gzip",
				"Content-Type":     "text/plain",
			},
			expectError:  false,
			expectedBody: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		},
		{
			name:   "no gzip when not accepted",
			method: http.MethodPost,
			headers: map[string]string{
				"Accept-Encoding": "deflate",
			},
			contentMaker: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("hello world"))
				return
			},
			contentType:    "text/plain",
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

			w.HeaderMap.Add("Content-Type", tt.contentType)

			err := ServeContent(w, req, tt.contentMaker)

			if (err != nil) != tt.expectError {
				t.Fatalf("ServeContent() error = %v, expectError %v", err, tt.expectError)
			}

			if w.Code != tt.expectedStatus {
				t.Fatalf("status code = %v, want %v", w.Code, tt.expectedStatus)
			}

			for k, v := range tt.expectedHeaders {
				if got := w.Header().Get(k); got != v {
					t.Fatalf("header %s = %v, want %v", k, got, v)
				}
			}

			// response body
			var body string

			if w.Header().Get("Content-Encoding") == "gzip" {
				if body, err = readGzipBody(w.Body); err != nil {
					t.Fatal(err)
				}
			} else {
				body = w.Body.String()
			}

			// check the body
			if len(tt.expectedBody) > 0 && body != tt.expectedBody {
				t.Fatalf("body = %q, want %q", body, tt.expectedBody)
			}
		})
	}
}

func readGzipBody(src *bytes.Buffer) (string, error) {
	reader, err := gzip.NewReader(src)

	if err != nil {
		return "", fmt.Errorf("creating gzip reader: %w", err)
	}

	defer reader.Close()

	s, err := io.ReadAll(reader)

	if err != nil {
		return "", fmt.Errorf("reading gzip'ed content: %w", err)
	}

	return string(s), nil
}

// large content to verify file backing
func TestServeLargeContent(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 100000) // 100KB, triggers file backing

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	err := ServeContent(w, req, func(wr io.Writer) (err error) {
		_, err = wr.Write(data)
		return
	})

	if err != nil {
		t.Fatalf("ServeContent failed: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Length") != "100000" {
		t.Fatalf("Content-Length = %s, want 100000", w.Header().Get("Content-Length"))
	}

	if w.Body.Len() != 100000 {
		t.Fatalf("body length = %d, want 100000", w.Body.Len())
	}

	if !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatal("content mismatch")
	}
}

func BenchmarkServeContentWithIncreasingSizes(b *testing.B) {
	// Test sizes: from small to large, crossing the 64KB buffer threshold
	sizes := []int{
		0,
		1 * 1024,
		8 * 1024,
		32 * 1024,
		64 * 1024, // buffer limit
		65 * 1024, // just over - file backing
		128 * 1024,
		256 * 1024,
		512 * 1024,
		1 * 1024 * 1024,
		2 * 1024 * 1024,
		4 * 1024 * 1024,
		8 * 1024 * 1024,
		16 * 1024 * 1024,
		32 * 1024 * 1024,
	}

	data := bytes.Repeat([]byte("x"), sizes[len(sizes)-1])
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := discardWriter{header: make(http.Header)}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				w.reset()

				err := ServeContent(&w, req, func(wr io.Writer) (e error) {
					_, e = wr.Write(data[:size])
					return
				})

				if err != nil {
					b.Fatalf("%d - %v", size, err)
				}

				if w.size != size {
					b.Fatalf("size mismatch: %d instead of %d", w.size, size)
				}

				if (size > 0 && w.code != http.StatusOK) ||
					(size == 0 && w.code != http.StatusNoContent) {
					b.Fatalf("unexpected HTTP code %d", w.code)
				}
			}
		})
	}
}

type discardWriter struct {
	size, code int
	header     http.Header
}

func (w *discardWriter) reset() {
	w.size = 0
	w.code = 0
	clear(w.header)
}

func (w *discardWriter) Header() http.Header {
	return w.header
}

func (w *discardWriter) Write(s []byte) (int, error) {
	w.size += len(s)
	return len(s), nil
}

func (w *discardWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}
