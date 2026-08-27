package httpx

import (
	"errors"
	"io"
	"net/http"
	"strconv"
)

// ServeJson wraps around [ServeContent] to deliver JSON-encoded responses.
func ServeJson(w http.ResponseWriter, r *http.Request, fn func(io.Writer) error) error {
	if MatchContentType(r.Header.Values("Accept"), jsonContentType) {
		return ServeContent(w, r, func(dest io.Writer) (string, error) {
			return jsonContentType, fn(dest)
		})
	}

	return sendErrStr(w, http.StatusNotAcceptable, "client does not accept "+jsonContentType)
}

const jsonContentType = "application/json"

// error writer
func sendErrStr(w http.ResponseWriter, code int, msg string) error {
	http.Error(w, http.StatusText(code), code)
	return errors.New("(" + strconv.Itoa(code) + ") " + msg)
}
