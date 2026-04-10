package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// ServeJson is built on top of ServeContent specifically for JSON-encoded responses.
func ServeJson(w http.ResponseWriter, r *http.Request, fn func(*json.Encoder) error) error {
	if isAcceptable(r.Header.Values("Accept"), "application/json") {
		return ServeContent(w, r, func(dest io.Writer) (string, error) {
			return "application/json", fn(json.NewEncoder(dest))
		})
	}

	sendErr(w, http.StatusNotAcceptable)
	return errors.New("client does not accept application/json")
}

// check if the given MIME type is acceptable as per Accept HTTP header
func isAcceptable(headers []string, targetType string) bool {
	if len(headers) == 0 {
		return true
	}

	// target media type
	main, sub := splitMediaType(targetType)

	if len(main) == 0 {
		return false
	}

	// acceptance check
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

			// media type
			m, s, _ := strings.Cut(mediaType, "/")

			// match
			if (m == main && (s == sub || s == "*")) || (m == "*" && s == "*") {
				return true
			}
		}
	}

	return false
}

func splitMediaType(s string) (main, sub string) {
	if m := matchMediaType(s); len(m) == 3 {
		main, sub = m[1], m[2]
	}

	return
}

const (
	restrictedNameRE = `[A-Za-z0-9][A-Za-z0-9!#$&\-^_.+]{0,126}`
	mediaTypeRE      = "^(" + restrictedNameRE + ")/(" + restrictedNameRE + ")$"
)

var matchMediaType = regexp.MustCompile(mediaTypeRE).FindStringSubmatch
