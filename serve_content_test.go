package httpx

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestHasGzip(t *testing.T) {
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
		if got := hasGzip(tt.header); got != tt.want {
			t.Fatalf("gzipAccepted(%q) = %v; want %v", tt.header, got, tt.want)
		}
	}
}

func TestSetVaryHeader(t *testing.T) {
	tests := []struct {
		val, exp string
	}{
		{"", "Accept-Encoding"},
		{"Accept-Encoding", "Accept-Encoding"},
		{"accept-encoding", "accept-encoding"},
		{"*", "*"},
		{"Origin", "Origin,Accept-Encoding"},
	}

	h := make(http.Header)

	for i, tc := range tests {
		if len(tc.val) > 0 {
			h.Set("Vary", tc.val)
		}

		setVaryHeader(h)

		if r := strings.Join(h.Values("Vary"), ","); r != tc.exp {
			t.Fatalf(`(%d) header mismatch: "%s" instead of "%s"`, i, r, tc.exp)
		}
	}
}

func TestSkipCompression(t *testing.T) {
	tests := map[string]bool{
		"":                 false,
		"@@@$$$":           false,
		"application/json": false,
		"application/gzip": true,
		"IMAGE/JPEG":       true,
		"video/mp4":        true,
		"font/woff":        true,
	}

	for k, v := range tests {
		if r := skipCompression(k); r != v {
			t.Fatalf("%s: %v instead of %v", k, r, v)
		}
	}
}

// helpers: ContentMakers that write s and return status.
func body(s string, status int) ContentMaker {
	return func(w io.Writer) (int, error) {
		if _, err := io.WriteString(w, s); err != nil {
			return http.StatusInternalServerError, err
		}

		return status, nil
	}
}

func bodySlice(s []byte, status int) ContentMaker {
	return func(w io.Writer) (int, error) {
		if _, err := w.Write(s); err != nil {
			return http.StatusInternalServerError, err
		}

		return status, nil
	}
}

// helper: decompress a gzip-encoded response body.
func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()

	r, err := gzip.NewReader(bytes.NewReader(b))

	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}

	defer r.Close()

	out, err := io.ReadAll(r)

	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}

	return out
}

func TestServeContent_Body(t *testing.T) {
	const payload = "hello"

	cases := []struct {
		name         string
		acceptEnc    string
		preHeaders   map[string]string
		contentType  string
		wantEncoding string // "" means no Content-Encoding expected
		wantETag     string
		content      string
	}{
		{
			name:     "plain",
			content:  payload,
			wantETag: "",
		},
		{
			name:         "gzip",
			acceptEnc:    "gzip",
			content:      payload,
			wantEncoding: "gzip",
		},
		{
			name:         "gzip skipped when Content-Encoding already set",
			acceptEnc:    "gzip",
			preHeaders:   map[string]string{"Content-Encoding": "br"},
			content:      payload,
			wantEncoding: "br",
		},
		{
			name:         "gzip applied despite preset identity",
			acceptEnc:    "gzip",
			preHeaders:   map[string]string{"Content-Encoding": "identity"},
			content:      payload,
			wantEncoding: "gzip",
		},
		{
			name:         "gzip with strong ETag",
			acceptEnc:    "gzip",
			preHeaders:   map[string]string{"ETag": `"abc123"`},
			content:      payload,
			wantEncoding: "gzip",
			wantETag:     `"abc123-gzip"`,
		},
		{
			name:         "gzip with weak ETag",
			acceptEnc:    "gzip",
			preHeaders:   map[string]string{"ETag": `W/"abc123"`},
			content:      payload,
			wantEncoding: "gzip",
			wantETag:     `W/"abc123-gzip"`,
		},
		{
			name:       "uncompressed keeps ETag unchanged",
			preHeaders: map[string]string{"ETag": `"abc123"`},
			content:    payload,
			wantETag:   `"abc123"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			if tc.acceptEnc != "" {
				req.Header.Set("Accept-Encoding", tc.acceptEnc)
			}

			for k, v := range tc.preHeaders {
				rec.Header().Set(k, v)
			}

			status, err := ServeContent(rec, req, body(tc.content, http.StatusOK))

			if err != nil {
				t.Fatalf("ServeContent: %v", err)
			}

			if status != http.StatusOK {
				t.Fatalf("status: got %d, want 200", status)
			}

			res := rec.Result()

			if got := res.Header.Get("Content-Encoding"); got != tc.wantEncoding {
				t.Fatalf("Content-Encoding: got %q, want %q", got, tc.wantEncoding)
			}

			if got := res.Header.Get("ETag"); got != tc.wantETag {
				t.Fatalf("ETag: got %q, want %q", got, tc.wantETag)
			}

			if got := res.Header.Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
				t.Fatalf("Vary: got %q, want it to contain Accept-Encoding", got)
			}

			got := rec.Body.Bytes()

			if tc.wantEncoding == "gzip" {
				got = gunzip(t, got)
			}

			if string(got) != tc.content {
				t.Fatalf("body: got %q, want %q", got, tc.content)
			}

			// Content-Length must match the bytes actually sent.
			wantCL := strconv.Itoa(len(rec.Body.Bytes()))

			if got := res.Header.Get("Content-Length"); got != wantCL {
				t.Fatalf("Content-Length: got %q, want %q", got, wantCL)
			}
		})
	}
}

func TestServeContent_NoBodyStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusNoContent,
		http.StatusNotModified,
		http.StatusResetContent,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")

			gotStatus, err := ServeContent(rec, req, body("ignored", status))

			if err != nil {
				t.Fatalf("ServeContent: %v", err)
			}

			if gotStatus != status {
				t.Fatalf("status: got %d, want %d", gotStatus, status)
			}

			if rec.Body.Len() != 0 {
				t.Fatalf("body: got %q, want empty", rec.Body.String())
			}

			if h := rec.Result().Header; h.Get("Content-Encoding") != "" {
				t.Fatalf("Content-Encoding: got %q, want empty", h.Get("Content-Encoding"))
			}
		})
	}
}

func TestServeContent_InvalidStatus(t *testing.T) {
	for _, status := range []int{0, 100, 150, 199, 600, 999} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			gotStatus, err := ServeContent(rec, req, body("x", status))

			if err == nil {
				t.Fatalf("expected error for status %d", status)
			}

			if gotStatus != http.StatusInternalServerError {
				t.Fatalf("returned status: got %d, want 500", gotStatus)
			}

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("recorder code: got %d, want 500", rec.Code)
			}
		})
	}
}

func TestServeContent_MakerError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	makerErr := errors.New("boom")

	_, err := ServeContent(rec, req, func(w io.Writer) (int, error) {
		io.WriteString(w, "partial")
		return http.StatusOK, makerErr
	})

	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, makerErr) {
		t.Fatalf("error chain: got %v, want it to wrap %v", err, makerErr)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code: got %d, want 500", rec.Code)
	}

	if got := rec.Body.String(); got != "Internal Server Error\n" {
		t.Fatalf("body: %q", got)
	}
}

func TestServeContent_MakerCustomError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	makerErr := errors.New("boom")

	status, err := ServeContent(rec, req, func(w io.Writer) (int, error) {
		io.WriteString(w, "partial")
		return http.StatusOK, Error(makerErr, "text/plain", "zzz")
	})

	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, makerErr) {
		t.Fatalf("error chain: got %v, want it to wrap %v", err, makerErr)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code: got %d, want 500", rec.Code)
	}

	if status != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", status)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("content type: %q", ct)
	}

	if got := rec.Body.String(); got != "zzz" {
		t.Fatalf("body: %q", got)
	}
}

func TestServeContent_HEAD(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/", nil)

	req.Header.Set("Accept-Encoding", "gzip")

	payload := strings.Repeat("hello world ", 100)
	status, err := ServeContent(rec, req, body(payload, http.StatusOK))

	if err != nil {
		t.Fatalf("ServeContent: %v", err)
	}

	if status != http.StatusOK {
		t.Fatalf("status: got %d, want 200", status)
	}

	res := rec.Result()

	if res.Header.Get("Content-Length") == "" {
		t.Fatalf("Content-Length: missing for HEAD")
	}

	if res.Header.Get("Content-Length") == strconv.Itoa(len(payload)) {
		t.Fatalf("Content-Length: got uncompressed length, want compressed length")
	}

	if rec.Body.Len() != 0 {
		t.Fatalf("body: got %d bytes, want 0", rec.Body.Len())
	}
}

func TestServeContent_LargeGzipSpill(t *testing.T) {
	// payload larger than the 64 KiB in-memory buffer to exercise the temp-file path.
	payload := strings.Repeat("abcdefghij", 20_000) // 200 KB

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	req.Header.Set("Accept-Encoding", "gzip")

	status, err := ServeContent(rec, req, body(payload, http.StatusOK))

	if err != nil {
		t.Fatalf("ServeContent: %v", err)
	}

	if status != http.StatusOK {
		t.Fatalf("status: got %d, want 200", status)
	}

	if got := rec.Result().Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding: got %q, want gzip", got)
	}

	if got := string(gunzip(t, rec.Body.Bytes())); got != payload {
		t.Fatalf("decompressed body mismatch (got %d bytes, want %d)", len(got), len(payload))
	}
}

func TestServeContent_HEAD_ErrorPath(t *testing.T) {
	// uses httptest.NewServer so Go's HEAD body suppression is exercised end-to-end.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeContent(w, r, func(io.Writer) (int, error) {
			return http.StatusInternalServerError, errors.New("boom")
		})
	}))

	defer srv.Close()

	res, err := http.Head(srv.URL)

	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", res.StatusCode)
	}

	b, err := io.ReadAll(res.Body)

	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if len(b) != 0 {
		t.Fatalf("body: got %q, want empty", b)
	}
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

				_, err := ServeContent(
					&w,
					req,
					bodySlice(randDataSlice[len(randDataSlice)-size:], http.StatusOK),
				)

				if err != nil {
					b.Fatalf("(%s) %v", baseNameOf(b), err)
				}

				if w.code != http.StatusOK {
					b.Fatalf(
						"(%s) unexpected HTTP code: %d instead of %d",
						baseNameOf(b),
						w.code,
						http.StatusOK,
					)
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
