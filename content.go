// Package httpx is a collection of useful addons for net/http.
package httpx

import (
	"cmp"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// ContentMaker is a function that writes the content to the given [io.Writer]
// and returns Content-Type value as a string, or an error.
type ContentMaker = func(io.Writer) (string, error)

// ServeContent calls the given [ContentMaker] function to generate (dynamic) content, and then
// writes the content to the given [http.ResponseWriter], while handling other aspects of the
// response delivery (like error processing, buffering, and setting HTTP headers) internally.
func ServeContent(w http.ResponseWriter, r *http.Request, fn ContentMaker) (err error) {
	var (
		contentType string
		contentLen  int64
	)

	// buffer
	b := allocBuffer()

	defer b.recycle()

	// invoke content maker
	gz := canGzip(r.Header.Values("Accept-Encoding"))

	if gz {
		contentType, err = gzipped(b, fn)
	} else {
		contentType, err = fn(b)
	}

	if err != nil {
		return sendErr(w, http.StatusInternalServerError, err)
	}

	// flush the buffer
	if contentLen, err = b.complete(); err != nil {
		return sendErr(w, http.StatusInternalServerError, err)
	}

	if contentLen == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// HTTP header
	h := w.Header()

	h.Set("Content-Length", strconv.FormatInt(contentLen, 10))
	h.Set("Content-Type", cmp.Or(contentType, "application/octet-stream"))

	if gz {
		h.Set("Content-Encoding", "gzip")
	}

	w.WriteHeader(http.StatusOK)

	// the actual write
	if r.Method != http.MethodHead {
		err = b.writeTo(w)
	}

	return
}

func gzipped(b *buffer, fn ContentMaker) (cont string, err error) {
	gz := gzip.NewWriter(b)

	gz.Header.ModTime = time.Now()

	if cont, err = fn(gz); err == nil {
		err = gz.Close()
	}

	return
}

func canGzip(headers []string) bool {
	for _, h := range headers {
		if gzipAccepted(h) {
			return true
		}
	}

	return false
}

const gzipRE = `(?i)(^|,)\s*(gzip(\s*;\s*q\s*=\s*(0?\.([1-9]\d{0,2})|1(\.0{0,3})?))?|\*)\s*(,|$)`

var gzipAccepted = regexp.MustCompile(gzipRE).MatchString

// error writers
func sendErr(w http.ResponseWriter, code int, err error) error {
	http.Error(w, http.StatusText(code), code)
	return fmt.Errorf("(%d) %w", code, err)
}

func sendErrStr(w http.ResponseWriter, code int, msg string) error {
	http.Error(w, http.StatusText(code), code)
	return errors.New("(" + strconv.Itoa(code) + ") " + msg)
}
