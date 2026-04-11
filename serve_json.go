package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// ServeJson is built on top of ServeContent specifically for JSON-encoded responses.
func ServeJson(w http.ResponseWriter, r *http.Request, fn func(*json.Encoder) error) error {
	if MatchContentType(r.Header.Values("Accept"), "application/json") {
		return ServeContent(w, r, func(dest io.Writer) (string, error) {
			return "application/json", fn(json.NewEncoder(dest))
		})
	}

	sendErr(w, http.StatusNotAcceptable)
	return errors.New("client does not accept application/json")
}
