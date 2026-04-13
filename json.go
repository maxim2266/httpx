package httpx

import (
	"encoding/json"
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

	return sendErrStr(w, http.StatusNotAcceptable, "client does not accept "+jsonContentType)
}

const jsonContentType = "application/json"
