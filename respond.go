package httpx

import (
	"cmp"
	"compress/gzip"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type ContentMaker = func(io.Writer) (string, error)

// Respond generates dynamic content and writes it to the response.
func Respond(w http.ResponseWriter, r *http.Request, fn ContentMaker) (err error) {
	var (
		contentType string
		contentLen  int64
	)

	// buffer
	b := allocBuffer()

	defer b.recycle()

	// invoke content maker
	comp := gzipAccepted(r.Header.Get("Accept-Encoding"))

	if comp {
		contentType, err = gzipped(b, fn)
	} else {
		contentType, err = fn(b)
	}

	if err != nil {
		writeErr(w, http.StatusInternalServerError)
		return
	}

	// flush the buffer
	if contentLen, err = b.flush(); err != nil {
		writeErr(w, http.StatusInternalServerError)
		return
	}

	if contentLen == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// HTTP headers
	h := w.Header()

	h.Set("Content-Length", strconv.FormatInt(contentLen, 10))
	h.Set("Content-Type", cmp.Or(contentType, "application/octet-stream"))

	if comp {
		h.Set("Content-Encoding", "gzip")
	}

	w.WriteHeader(http.StatusOK) // ???

	// the actual write
	return b.writeTo(w)
}

func gzipped(w io.Writer, fn ContentMaker) (cont string, err error) {
	gz := gzip.NewWriter(w)

	gz.Header.ModTime = time.Now()

	if cont, err = fn(gz); err == nil {
		err = gz.Close()
	}

	return
}

const gzipRE = `(?i)(^|,)\s*(gzip(\s*;\s*q\s*=\s*(0?\.([1-9]\d{0,2})|1(\.0{0,3})?))?|\*)\s*(,|$)`

var (
	gzipAccepted = regexp.MustCompile(gzipRE).MatchString

	TempDir string
)

// error writer
func writeErr(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}
