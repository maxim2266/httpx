package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// ServeJson wraps around [ServeContent] to deliver JSON-encoded responses.
func ServeJson(w http.ResponseWriter, r *http.Request, fn func(*json.Encoder) error) error {
	if MatchContentType(r.Header.Values("Accept"), jsonContentType) {
		return ServeContent(w, r, func(dest io.Writer) (string, error) {
			return jsonContentType, fn(json.NewEncoder(dest))
		})
	}

	sendErr(w, http.StatusNotAcceptable)
	return errors.New("client does not accept " + jsonContentType)
}

const jsonContentType = "application/json"
