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
	"strings"
	"time"
)

type ContentMaker = func(io.Writer) (string, error)

// Respond generates dynamic content and writes it to the response.
//
// Range support is limited to "bytes=0-N" only (requests starting at 0).
// For full range support with static files, use http.ServeContent instead.
func Respond(w http.ResponseWriter, r *http.Request, fn ContentMaker) (err error) {
	var (
		contentType          string
		contentLen, maxBytes int64
	)

	h := w.Header()

	// 	range header
	switch maxBytes = parseRangeHeader(r.Header.Values("Range")); maxBytes {
	case -1:
		// ok, no header

	case -2:
		writeErr(w, http.StatusBadRequest)
		return errors.New("invalid Range header")

	case -3:
		h.Set("Content-Range", "bytes */0")
		writeErr(w, http.StatusRequestedRangeNotSatisfiable)
		return errors.New("content range cannot be satisfied")
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
	h.Set("Accept-Ranges", "bytes")

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

func parseRangeHeader(headers []string) int64 {
	// header length
	switch len(headers) {
	case 0:
		return -1
	case 1:
		// ok
	default:
		return -2
	}

	// prefix
	s, ok := strings.CutPrefix(headers[0], "bytes ")

	if !ok {
		return -2
	}

	// lower limit
	s, val, ok := rangeValue(s, '-')

	if !ok {
		return -2
	}

	if val != 0 {
		return -3
	}

	// upper limit
	if s, val, ok = rangeValue(s, '/'); !ok {
		return -2
	}

	// the rest after '/'
	switch len(s) {
	case 0:
		return -2
	case 1:
		if s[0] != '*' && (s[0] < '0' || s[0] > '9') {
			return -2
		}

	default:
		// all digits
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return -2
			}
		}
	}

	return val
}

func rangeValue(s string, delim byte) (rest string, val int64, ok bool) {
	if ok = len(s) > 1 && s[0] >= '0' && s[0] <= '9'; !ok {
		return
	}

	val = int64(s[0] - '0')

	if ok = val > 0 || s[1] == delim; !ok {
		return
	}

	for i := 1; i < len(s); i++ {
		if ok = s[i] == delim; ok {
			rest = s[i+1:]
			ok = len(rest) > 0
			return
		}

		if ok = s[i] >= '0' && s[i] <= '9'; !ok {
			return
		}

		if val = val*10 + int64(s[i]-'0'); val <= 0 {
			ok = false
			return
		}
	}

	ok = false
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

// ternary operator, the Go version
func choose[T any](cond bool, yes, no T) T {
	if cond {
		return yes
	}

	return no
}
