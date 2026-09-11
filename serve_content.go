// Package httpx is a collection of useful addons for net/http.
package httpx

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/gzip"
)

// ContentMaker is the callback type invoked by [ServeContent] to generate the
// response body. It writes the body to the provided [io.Writer] and returns an
// HTTP status code and an error.
//
// The writer is a buffering sink; no bytes reach the client until the callback
// returns successfully. The callback must not retain it.
type ContentMaker = func(io.Writer) (int, error)

// ServeContent invokes fn to generate a response body and delivers it to the
// client via w, handling buffering, optional gzip compression, and error
// reporting.
//
// Headers that affect delivery (Content-Type, ETag) must be set on w.Header()
// before calling. If Content-Encoding is already set, ServeContent assumes the
// caller is handling encoding and does not compress.
//
// The callback's output is fully buffered before any status or header is
// written, so an error or invalid status do not send partial body.
//
// An error returned from the content maker function discards output buffer.
// If the error is created by [Error] function, then the error's associated
// content is delivered to the client instead of a generic text for the status code.
//
// Return values: status is always the actual HTTP status sent to the client, while
// err is for logging only and reflects failures that may have happened during response
// creation and delivery. Content of err is never sent to the client.
func ServeContent(
	w http.ResponseWriter,
	r *http.Request,
	fn ContentMaker,
) (status int, err error) {
	// buffer
	b := allocBuffer()

	defer b.recycle()

	// HTTP header
	h := w.Header()

	// invoke content maker
	gz := contentEncodingNotSet(h.Get("Content-Encoding")) &&
		gzipAcceted(r.Header.Values("Accept-Encoding")) &&
		!skipCompression(h.Get("Content-Type"))

	if gz {
		status, err = compress(b, fn)
	} else {
		status, err = fn(b)
	}

	// fail on error
	if err != nil {
		// status must be 4xx or 5xx
		if status < 400 || status > 599 {
			status = http.StatusInternalServerError
		}

		// report error
		if e, ok := err.(*problem); ok {
			// set headers like http.Error does
			h.Del("Content-Length")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Content-Type", e.contType)

			// write response
			w.WriteHeader(status)
			writeString(w, e.cont)

			err = e.Unwrap()

		} else {
			http.Error(w, cmp.Or(http.StatusText(status), "Error"), status)
		}

		err = fmt.Errorf("content maker: %w", err)
		return
	}

	// check status
	if status < 200 || status > 599 { // disallow 1xx and non-standard codes
		return report(
			w,
			"content maker",
			errors.New("invalid HTTP status "+strconv.Itoa(status)),
		)
	}

	if status == http.StatusNoContent ||
		status == http.StatusNotModified ||
		status == http.StatusResetContent {
		// these responses must not have a body
		w.WriteHeader(status)
		return
	}

	// flush the buffer
	var contentLen int64

	if contentLen, err = b.flush(); err != nil {
		return report(w, "buffer flush", err)
	}

	// setup and send HTTP headers
	h.Set("Content-Length", strconv.FormatInt(contentLen, 10))
	setVaryHeader(h)

	if gz {
		h.Set("Content-Encoding", "gzip")

		if etag := h.Get("ETag"); len(etag) >= 2 {
			h.Set("ETag", etag[:len(etag)-1]+`-gzip"`) // overwrite last quote
		}
	}

	w.WriteHeader(status)

	// the actual write
	if r.Method != http.MethodHead {
		if err = b.writeTo(w); err != nil {
			err = fmt.Errorf("writing HTTP response: %w", err)
		}
	}

	return
}

// respond with 500 and format error
func report(w http.ResponseWriter, prefix string, err error) (int, error) {
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	return http.StatusInternalServerError, fmt.Errorf("%s: %w", prefix, err)
}

// content encoding check
func contentEncodingNotSet(s string) bool {
	return len(s) == 0 || strings.EqualFold(s, "identity")
}

// set Vary header if not there yet
func setVaryHeader(h http.Header) {
	for _, s := range h.Values("Vary") {
		if s = strings.TrimSpace(s); s == "*" || strings.EqualFold(s, "Accept-Encoding") {
			return
		}
	}

	h.Add("Vary", "Accept-Encoding")
}

// gzip acceptance tester
func gzipAcceted(h []string) bool {
	return slices.ContainsFunc(h, hasGzip)
}

const gzipRE = `(?i)(^|,)\s*(gzip(\s*;\s*q\s*=\s*(0?\.([1-9]\d{0,2})|1(\.0{0,3})?))?|\*)\s*(,|$)`

var hasGzip = regexp.MustCompile(gzipRE).MatchString

// apply compression
func compress(b *buffer, fn ContentMaker) (status int, err error) {
	gz := compressorPool.Get().(*gzip.Writer)

	defer func() {
		gz.Reset(io.Discard) // cut off buffer connection to help gc
		compressorPool.Put(gz)
	}()

	gz.Reset(b)

	if status, err = fn(compressor{gz}); err == nil {
		err = gz.Close()
	}

	return
}

// pool of compressors
var compressorPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(io.Discard)
	},
}

// gzip.Writer wrapper
type compressor struct {
	gz *gzip.Writer
}

func (c compressor) Write(data []byte) (n int, err error) {
	if err = write(c.gz, data); err == nil {
		n = len(data)
	}

	return
}

func (c compressor) WriteString(s string) (int, error) {
	return writeString(c, s)
}

// skip compression for some media types
func skipCompression(contType string) (skip bool) {
	if len(contType) == 0 {
		return
	}

	mediaType, _, err := mime.ParseMediaType(contType)

	if err != nil {
		return
	}

	// check patterns
	mainType, subType, _ := strings.Cut(mediaType, "/")

	switch mainType {
	case "video", "audio":
		skip = true

	case "image":
		skip = subType != "svg+xml" && subType != "x-icon" && subType != "vnd.microsoft.icon"

	case "application":
		if _, skip = skipCompressionApps[subType]; !skip {
			skip = strings.HasSuffix(subType, "+zip")
		}

	case "font":
		skip = subType == "woff" || subType == "woff2"
	}

	return
}

// TODO: check this map again at some point
var skipCompressionApps = map[string]struct{}{
	// already-compressed fonts
	"vnd.ms-fontobject": {},

	// archives
	"zstd":                        {},
	"x-zstd":                      {},
	"zip":                         {},
	"gzip":                        {},
	"x-gzip":                      {},
	"x-7z-compressed":             {},
	"x-bzip":                      {},
	"x-bzip2":                     {},
	"x-compress":                  {},
	"x-deb":                       {},
	"x-lzip":                      {},
	"x-lzma":                      {},
	"x-lzop":                      {},
	"x-rar-compressed":            {},
	"vnd.rar":                     {},
	"x-rpm":                       {},
	"x-xz":                        {},
	"java-archive":                {},
	"x-java-archive":              {},
	"vnd.android.package-archive": {},

	// documents (ZIP-based formats)
	"vnd.openxmlformats-officedocument.wordprocessingml.document":   {},
	"vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {},
	"vnd.openxmlformats-officedocument.presentationml.presentation": {},
	"vnd.oasis.opendocument.text":                                   {},
	"vnd.oasis.opendocument.spreadsheet":                            {},

	// other
	"pdf": {}, // usually compressed
}
