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
	bigString := string(randDataSlice[:100000])

	tests := []struct {
		name            string
		headers         map[string]string
		contentMaker    func(io.Writer) error
		expectedStatus  int
		expectedHeaders map[string]string
		expectedBody    string
		expectError     bool
	}{
		// simple response w/o error
		{
			name:    "successful response",
			headers: map[string]string{},
			contentMaker: func(w io.Writer) (err error) {
				_, err = io.WriteString(w, "hello world")
				return
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Length": "11",
			},
			expectedBody: "hello world",
			expectError:  false,
		},
		{
			name:    "successful response with big string",
			headers: map[string]string{},
			contentMaker: func(w io.Writer) (err error) {
				_, err = io.WriteString(w, bigString)
				return
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Length": "100000",
			},
			expectedBody: bigString,
			expectError:  false,
		},
		{
			name:    "empty content",
			headers: map[string]string{},
			contentMaker: func(_ io.Writer) error {
				return nil
			},
			expectedStatus:  http.StatusNoContent,
			expectedHeaders: map[string]string{},
			expectedBody:    "",
			expectError:     false,
		},

		// content maker error
		{
			name:    "content maker returns error",
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
			name: "gzip compression when accepted",
			headers: map[string]string{
				"Accept-Encoding": "gzip",
			},
			contentMaker: func(w io.Writer) (err error) {
				_, err = io.WriteString(w, "hello world")
				return
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Encoding": "gzip",
			},
			expectError:  false,
			expectedBody: "hello world",
		},
		{
			name: "gzip compression with big string",
			headers: map[string]string{
				"Accept-Encoding": "gzip",
			},
			contentMaker: func(w io.Writer) (err error) {
				_, err = io.WriteString(w, bigString)
				return
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Encoding": "gzip",
			},
			expectError:  false,
			expectedBody: bigString,
		},
		{
			name: "gzip compression empty content",
			headers: map[string]string{
				"Accept-Encoding": "gzip",
			},
			contentMaker: func(_ io.Writer) error {
				return nil
			},
			expectedStatus:  http.StatusNoContent,
			expectedHeaders: map[string]string{},
			expectError:     false,
		},
		{
			name: "no gzip when not accepted",
			headers: map[string]string{
				"Accept-Encoding": "deflate",
			},
			contentMaker: func(w io.Writer) (err error) {
				_, err = io.WriteString(w, "hello world")
				return
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Encoding": "",
				"Content-Length":   "11",
			},
			expectedBody: "hello world",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
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

	s, err := io.ReadAll(reader)

	if err != nil {
		return "", fmt.Errorf("reading gzip'ed content: %w", err)
	}

	if err = reader.Close(); err != nil {
		return "", fmt.Errorf("closing gzip: %w", err)
	}

	return string(s), nil
}

func BenchmarkServeContent(b *testing.B) {
	benchServeContent(b, false)
}

func BenchmarkServeCompressedContent(b *testing.B) {
	benchServeContent(b, true)
}

func benchServeContent(b *testing.B, gz bool) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if gz {
		req.Header.Add("Accept-Encoding", "gzip")
	}

	w := discardWriter{header: make(http.Header)}

	for _, size := range testDataSizes {
		b.Run(formatSize(size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				w.reset()

				err := ServeContent(&w, req, func(wr io.Writer) (e error) {
					_, e = wr.Write(randDataSlice[len(randDataSlice)-size:])
					return
				})

				if err != nil {
					b.Fatalf("(%s) %v", baseNameOf(b), err)
				}

				switch size {
				case 0:
					if w.code != http.StatusNoContent {
						b.Fatalf(
							"(%s) unexpected HTTP code: %d instead of %d",
							baseNameOf(b),
							w.code,
							http.StatusNoContent,
						)
					}

				default:
					if w.code != http.StatusOK {
						b.Fatalf(
							"(%s) unexpected HTTP code: %d instead of %d",
							baseNameOf(b),
							w.code,
							http.StatusOK,
						)
					}
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
	w.size, w.code = 0, 0
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
