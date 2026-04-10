package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// ServeJson serialises the given object to JSON and sends it back to the client.
func ServeJson(w http.ResponseWriter, r *http.Request, obj any) error {
	if !isAcceptable(r.Header.Values("Accept"), "application/json") {
		sendErr(w, http.StatusNotAcceptable)
		return errors.New("request does not accept application/json")
	}

	return ServeContent(w, r, func(dest io.Writer) (string, error) {
		return "application/json", json.NewEncoder(dest).Encode(obj)
	})
}

// check if the given MIME type is acceptable as per Accept HTTP header
func isAcceptable(headers []string, targetType string) bool {
	if len(headers) == 0 {
		return true
	}

	if len(targetType) == 0 {
		return false
	}

	main, sub, _ := strings.Cut(targetType, "/")

	for _, h := range headers {
		for part := range strings.SplitSeq(h, ",") {
			if part = strings.TrimSpace(part); len(part) == 0 {
				continue
			}

			mediaType, params, err := mime.ParseMediaType(part)

			if err != nil {
				continue // malformed media type, skip
			}

			// check q value
			if s, ok := params["q"]; ok {
				if q, err := strconv.ParseFloat(s, 32); err != nil || q < 0.001 || q > 1.0 {
					continue // invalid q, skip this media type
				}
			}

			m, s, _ := strings.Cut(mediaType, "/")

			// match
			if (m == main && (s == sub || s == "*")) || (m == "*" && s == "*") {
				return true
			}
		}
	}

	return false
}
