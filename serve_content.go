// Package httpx is a collection of useful addons for net/http.
package httpx

import (
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
	"unsafe"

	"github.com/klauspost/compress/gzip"
)

// Error replies to the request with the specified HTTP code and its associated standard message.
func Error(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

// error type
type problem struct {
	code int
	err  error
}

// Failure creates a new error object that includes the given HTTP status code and the error.
// Status code must be either 4xx or 5xx. The function should be used in content makers
// when upon an error a specific HTTP status has to be sent back to the client.
func Failure(code int, err error) error {
	if code < 400 || len(http.StatusText(code)) == 0 {
		err = errors.Join(
			err,
			errors.New("invalid HTTP error code "+strconv.Itoa(code)+" in httpx.Failure"),
		)

		code = http.StatusInternalServerError
	}

	return &problem{code, err}
}

// FailureMsg creates a new error object that includes the given HTTP status code and
// the message. It is a thin wrapper around [Failure] function.
func FailureMsg(code int, msg string) error {
	return Failure(code, errors.New(msg))
}

// Error returns error message string.
func (p *problem) Error() string {
	return "(HTTP status " + strconv.Itoa(p.code) + ") " + p.err.Error()
}

// Unwrap returns the underlying error object.
func (p *problem) Unwrap() error {
	return p.err
}

// ServeContent calls the given content maker function to generate (dynamic) content, and then
// writes the content to the given [http.ResponseWriter], while handling other aspects of the
// response delivery (like error processing, buffering, and setting HTTP headers) internally.
func ServeContent(w http.ResponseWriter, r *http.Request, fn func(io.Writer) error) (err error) {
	// buffer
	b := allocBuffer()

	defer b.recycle()

	// invoke content maker
	gz := (r.Method != http.MethodHead) &&
		slices.ContainsFunc(r.Header.Values("Accept-Encoding"), gzipAccepted) &&
		!skipCompression(w.Header().Get("Content-Type"))

	if gz {
		err = compress(b, fn)
	} else {
		err = fn(b)
	}

	if err != nil {
		// extract HTTP status, if any, and fail
		if e, ok := err.(*problem); ok {
			Error(w, e.code)
			return err
		}

		Error(w, http.StatusInternalServerError)
		return fmt.Errorf("HTTP content maker: %w", err)
	}

	// flush the buffer
	var contentLen int64

	if contentLen, err = b.flush(); err != nil {
		Error(w, http.StatusInternalServerError)
		return fmt.Errorf("flushing HTTP buffer: %w", err)
	}

	if contentLen == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// HTTP header
	h := w.Header()

	h.Set("Content-Length", strconv.FormatInt(contentLen, 10))

	if gz {
		h.Set("Content-Encoding", "gzip")
	}

	w.WriteHeader(http.StatusOK)

	// the actual write
	if r.Method != http.MethodHead {
		if err = b.writeTo(w); err != nil {
			err = fmt.Errorf("writing HTTP response: %w", err)
		}
	}

	return
}

func compress(b *buffer, fn func(io.Writer) error) (err error) {
	c := compressorPool.Get().(*compressor)

	defer c.recycle()

	c.gz.Reset(b)

	if err = c.apply(fn); err == nil && c.count == 0 {
		// nothing has been written to the compressor - reset target buffer
		b.wi = 0
	}

	return
}

const gzipRE = `(?i)(^|,)\s*(gzip(\s*;\s*q\s*=\s*(0?\.([1-9]\d{0,2})|1(\.0{0,3})?))?|\*)\s*(,|$)`

var gzipAccepted = regexp.MustCompile(gzipRE).MatchString

// pool of compressors
var compressorPool = sync.Pool{
	New: func() any {
		return &compressor{
			gz: gzip.NewWriter(io.Discard),
		}
	},
}

// gzip.Writer wrapper
type compressor struct {
	gz    *gzip.Writer
	count int64
}

func (c *compressor) recycle() {
	c.gz.Reset(io.Discard) // cut off buffer connection to help gc
	c.count = 0
	compressorPool.Put(c)
}

func (c *compressor) apply(fn func(io.Writer) error) (err error) {
	if err = fn(c); err == nil {
		err = c.gz.Close()
	}

	return
}

func (c *compressor) Write(data []byte) (n int, err error) {
	if err = write(c.gz, data); err == nil {
		n = len(data)
		c.count += int64(n)
	}

	return
}

func (c *compressor) WriteString(s string) (int, error) {
	return c.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
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
