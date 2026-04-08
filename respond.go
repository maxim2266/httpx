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

type ContentMaker = func(io.Writer) (string, error)

func Respond(w http.ResponseWriter, r *http.Request, fn ContentMaker) (err error) {
	var (
		contentType          string
		contentLen, maxBytes int64
	)

	// 	range header
	if maxBytes, err = parseContentRange(r.Header.Values("Content-Range")); err != nil {
		writeErr(w, http.StatusBadRequest)
		return
	}

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

	// range header satisfiability check
	h := w.Header()

	if maxBytes >= 0 {
		if maxBytes < contentLen-1 {
			h.Set("Content-Range", "bytes */"+strconv.FormatInt(contentLen, 10))
			writeErr(w, http.StatusRequestedRangeNotSatisfiable)

			return fmt.Errorf(
				"content length of %d bytes exceeds content range on max. %d bytes",
				contentLen,
				maxBytes,
			)
		}

		h.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", contentLen-1, contentLen))
	}

	// other HTTP headers
	h.Set("Content-Length", strconv.FormatInt(contentLen, 10))
	h.Set("Content-Type", cmp.Or(contentType, "application/octet-stream"))

	if comp {
		h.Set("Content-Encoding", "gzip")
	}

	w.WriteHeader(choose(maxBytes >= 0, http.StatusPartialContent, http.StatusOK))

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

func parseContentRange(headers []string) (int64, error) {
	switch len(headers) {
	case 0:
		return -1, nil
	case 1:
		// OK
	default:
		return 0, errors.New("multiple Content-Range headers not allowed")
	}

	m := matchUpperLimit(headers[0])

	if len(m) == 0 {
		return 0, fmt.Errorf(
			"invalid Content-Range '%s': must be 'bytes 0-N'",
			headers[0],
		)
	}

	return strconv.ParseInt(m[1], 10, 64)
}

const gzipRE = `(?i)(^|,)\s*(gzip(\s*;\s*q\s*=\s*(0?\.([1-9]\d{0,2})|1(\.0{0,3})?))?|\*)\s*(,|$)`

var (
	gzipAccepted    = regexp.MustCompile(gzipRE).MatchString
	matchUpperLimit = regexp.MustCompile(`^bytes\s+0-(\d+)(?:/\*|/\d+)$`).FindStringSubmatch

	TempDir string
)

// error writer
func writeErr(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

// ternary operator, the Go version
func choose[T any](cond bool, yes, no T) T {
	if cond {
		return yes
	}

	return no
}
