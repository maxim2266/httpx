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

func TestRespond(t *testing.T) {
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
			name:   "successful response with valid Content-Range",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Range": "bytes 0-100/*",
			},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("hello world"))
				return "text/plain", nil
			},
			expectedStatus: http.StatusPartialContent,
			expectedHeaders: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": "11",
				"Content-Range":  "bytes 0-10/11",
			},
			expectedBody: "hello world",
			expectError:  false,
		},
		{
			name:   "Content-Range with exact match",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Range": "bytes 0-10/*",
			},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("hello world"))
				return "text/plain", nil
			},
			expectedStatus: http.StatusPartialContent,
			expectedHeaders: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": "11",
				"Content-Range":  "bytes 0-10/11",
			},
			expectedBody: "hello world",
			expectError:  false,
		},
		{
			name:   "Content-Range too small - should reject",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Range": "bytes 0-5/*",
			},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("hello world"))
				return "text/plain", nil
			},
			expectedStatus: http.StatusRequestedRangeNotSatisfiable,
			expectedHeaders: map[string]string{
				"Content-Range": "bytes */11",
			},
			expectedBody: "Requested Range Not Satisfiable\n",
			expectError:  true,
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
			name:   "empty content with range - should return 204 and ignore range header",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Range": "bytes 0-100/*",
			},
			contentMaker: func(w io.Writer) (string, error) {
				return "text/plain", nil // empty content
			},
			expectedStatus: http.StatusNoContent,
			expectedHeaders: map[string]string{
				"Content-Range": "", // should not be set
			},
			expectedBody: "",
			expectError:  false,
		},
		{
			name:    "empty content without range - should return 204",
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
			name:   "multiple Content-Range headers",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Range": "bytes 0-100/*, bytes 0-200/*",
			},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("data"))
				return "text/plain", nil
			},
			expectedStatus:  http.StatusBadRequest,
			expectedHeaders: map[string]string{},
			expectedBody:    "Bad Request\n",
			expectError:     true,
		},
		{
			name:   "invalid Content-Range format",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Range": "bytes 5-100/*",
			},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("data"))
				return "text/plain", nil
			},
			expectedStatus:  http.StatusBadRequest,
			expectedHeaders: map[string]string{},
			expectedBody:    "Bad Request\n",
			expectError:     true,
		},
		{
			name:    "content maker error",
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
		{
			name:   "gzip compression with accepted encoding",
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
				"Content-Encoding": "", // should not be set
				"Content-Type":     "text/plain",
				"Content-Length":   "11",
			},
			expectedBody: "hello world",
			expectError:  false,
		},
		{
			name:    "default content type when empty",
			method:  http.MethodPost,
			headers: map[string]string{},
			contentMaker: func(w io.Writer) (string, error) {
				w.Write([]byte("binary data"))
				return "", nil // empty content type
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Type": "application/octet-stream",
			},
			expectedBody: "binary data",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create request
			req := httptest.NewRequest(tt.method, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// create response recorder
			w := httptest.NewRecorder()

			// call Respond
			err := Respond(w, req, tt.contentMaker)

			// check error
			if (err != nil) != tt.expectError {
				t.Errorf("Respond() error = %v, expectError %v", err, tt.expectError)
			}

			// check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("status code = %v, want %v", w.Code, tt.expectedStatus)
			}

			// check headers
			for k, v := range tt.expectedHeaders {
				if got := w.Header().Get(k); got != v {
					t.Errorf("header %s = %v, want %v", k, got, v)
				}
			}

			// check body (if not gzipped)
			if tt.expectedBody != "" {
				body := w.Body.String()
				if body != tt.expectedBody {
					t.Errorf("body = %v, want %v", body, tt.expectedBody)
				}
			}

			// special check for gzipped content
			if tt.headers["Accept-Encoding"] == "gzip" && tt.expectedHeaders["Content-Encoding"] == "gzip" {
				// verify we can decompress it
				reader, err := gzip.NewReader(w.Body)
				if err != nil {
					t.Fatalf("failed to create gzip reader: %v", err)
				}
				defer reader.Close()

				decompressed, err := io.ReadAll(reader)
				if err != nil {
					t.Fatalf("failed to decompress: %v", err)
				}

				// compare with expected (we need to know what was written)
				if len(decompressed) == 0 {
					t.Errorf("decompressed content is empty")
				}
			}
		})
	}
}

func TestRespondWithLargeContent(t *testing.T) {
	largeData := strings.Repeat("a", 10000)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Range", "bytes 0-20000/*")

	w := httptest.NewRecorder()

	err := Respond(w, req, func(wr io.Writer) (string, error) {
		wr.Write([]byte(largeData))
		return "text/plain", nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if w.Code != http.StatusPartialContent {
		t.Errorf("expected 206, got %d", w.Code)
	}

	if w.Header().Get("Content-Range") != "bytes 0-9999/10000" {
		t.Errorf("unexpected Content-Range: %v", w.Header().Get("Content-Range"))
	}

	if w.Header().Get("Content-Length") != "10000" {
		t.Errorf("unexpected Content-Length: %v", w.Header().Get("Content-Length"))
	}
}

func TestRespondWithBufferReuse(t *testing.T) {
	// test that buffer pooling works correctly
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w1 := httptest.NewRecorder()
	w2 := httptest.NewRecorder()

	counter := 0
	maker := func(wr io.Writer) (string, error) {
		counter++
		wr.Write([]byte("test"))
		return "text/plain", nil
	}

	// first request
	err := Respond(w1, req, maker)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	// second request (should reuse buffer)
	err = Respond(w2, req, maker)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	if counter != 2 {
		t.Errorf("content maker called %d times, want 2", counter)
	}
}

func TestParseContentRange(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		expected int64
		hasError bool
	}{
		{
			name:     "no headers",
			headers:  []string{},
			expected: -1,
			hasError: false,
		},
		{
			name:     "valid range with asterisk",
			headers:  []string{"bytes 0-1024/*"},
			expected: 1024,
			hasError: false,
		},
		{
			name:     "valid range with total",
			headers:  []string{"bytes 0-2048/4096"},
			expected: 2048,
			hasError: false,
		},
		{
			name:     "multiple headers",
			headers:  []string{"bytes 0-100/*", "bytes 0-200/*"},
			expected: 0,
			hasError: true,
		},
		{
			name:     "invalid format",
			headers:  []string{"bytes 10-100/*"},
			expected: 0,
			hasError: true,
		},
		{
			name:     "wrong unit",
			headers:  []string{"items 0-100/*"},
			expected: 0,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseContentRange(tt.headers)
			if (err != nil) != tt.hasError {
				t.Errorf("parseContentRange() error = %v, want error %v", err, tt.hasError)
			}
			if result != tt.expected {
				t.Errorf("parseContentRange() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// simple benchmark
func BenchmarkRespond(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	maker := func(w io.Writer) (string, error) {
		w.Write([]byte("benchmark data"))
		return "text/plain", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		Respond(w, req, maker)
	}
}

// benchmark different content sizes
func BenchmarkRespondWithFileBacking(b *testing.B) {
	sizes := []struct {
		name  string
		bytes int
	}{
		{"Small_1KB", 1024},
		{"Buffer_64KB", 64 * 1024},
		{"Buffer_65KB", 65 * 1024}, // Slightly over buffer
		{"Medium_128KB", 128 * 1024},
		{"Medium_256KB", 256 * 1024},
		{"Large_1MB", 1024 * 1024},
		{"Large_5MB", 5 * 1024 * 1024},
		{"Large_10MB", 10 * 1024 * 1024},
		{"VeryLarge_50MB", 50 * 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			// create test data
			testData := bytes.Repeat([]byte("x"), size.bytes)

			req := httptest.NewRequest(http.MethodPost, "/", nil)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()

				err := Respond(w, req, func(wr io.Writer) (string, error) {
					wr.Write(testData)
					return "text/plain", nil
				})

				if err != nil {
					b.Fatalf("Respond failed: %v", err)
				}

				// verify content length matches
				if w.Body.Len() != size.bytes {
					b.Fatalf("size mismatch: got %d, want %d", w.Body.Len(), size.bytes)
				}
			}
		})
	}
}

// benchmark with gzip and file backing
func BenchmarkRespondWithGzipAndFileBacking(b *testing.B) {
	sizes := []struct {
		name  string
		bytes int
	}{
		{"Small_1KB", 1024},
		{"Buffer_64KB", 64 * 1024},
		{"Buffer_65KB", 65 * 1024},
		{"Medium_128KB", 128 * 1024},
		{"Medium_256KB", 256 * 1024},
		{"Large_1MB", 1024 * 1024},
		{"Large_5MB", 5 * 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			testData := bytes.Repeat([]byte("x"), size.bytes)
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()

				err := Respond(w, req, func(wr io.Writer) (string, error) {
					wr.Write(testData)
					return "text/plain", nil
				})

				if err != nil {
					b.Fatalf("Respond failed: %v", err)
				}

				// verify we got a response (body may be compressed)
				if w.Body.Len() == 0 {
					b.Fatalf("empty response body")
				}
			}
		})
	}
}

// benchmark concurrent requests with file backing
func BenchmarkRespondConcurrentFileBacking(b *testing.B) {
	size := 256 * 1024 // 256KB - forces file backing
	testData := bytes.Repeat([]byte("x"), size)
	req := httptest.NewRequest(http.MethodPost, "/", nil)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			err := Respond(w, req, func(wr io.Writer) (string, error) {
				wr.Write(testData)
				return "text/plain", nil
			})
			if err != nil {
				b.Fatalf("Respond failed: %v", err)
			}
		}
	})
}
