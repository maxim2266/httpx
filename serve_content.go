// Package httpx is a collection of useful addons for net/http.
package httpx

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"
)

// ServeContent calls the given function to generate (dynamic) content, and then
// writes the content to the given [http.ResponseWriter], while handling other aspects of the
// response delivery (like error processing, buffering, and setting HTTP headers) internally.
func ServeContent(w http.ResponseWriter, r *http.Request, fn func(io.Writer) error) (err error) {
	// buffer
	b := allocBuffer()

	defer b.recycle()

	// invoke content maker
	gz := slices.ContainsFunc(r.Header.Values("Accept-Encoding"), gzipAccepted)

	if gz {
		err = gzipped(b, fn)
	} else {
		err = fn(b)
	}

	if err != nil {
		return sendErr(w, http.StatusInternalServerError, err)
	}

	// flush the buffer
	var contentLen int64

	if contentLen, err = b.flush(); err != nil {
		return sendErr(w, http.StatusInternalServerError, err)
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
		err = b.writeTo(w)
	}

	return
}

func gzipped(b *buffer, fn func(io.Writer) error) (err error) {
	gz := compressorPool.Get().(*gzip.Writer)

	defer compressorPool.Put(gz)

	gz.Reset(b)
	gz.Header.ModTime = time.Now()

	if err = fn(gz); err == nil {
		err = gz.Close()
	}

	return
}

const gzipRE = `(?i)(^|,)\s*(gzip(\s*;\s*q\s*=\s*(0?\.([1-9]\d{0,2})|1(\.0{0,3})?))?|\*)\s*(,|$)`

var gzipAccepted = regexp.MustCompile(gzipRE).MatchString

// pool of compressors
var compressorPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(nil)
	},
}

// error writer
func sendErr(w http.ResponseWriter, code int, err error) error {
	http.Error(w, http.StatusText(code), code)
	return fmt.Errorf("(%d) %w", code, err)
}
