package httpx

import (
	"mime"
	"strconv"
	"strings"
)

// MatchContentType reports whether the given targetType is acceptable according
// to the Accept header values provided in headers.
//
// It parses the Accept header as defined by RFC 9110, handling quality values,
// media ranges, and wildcards. An empty headers slice is treated as accepting
// all media types.
//
// TargetType must be a valid media type of the form "type/subtype" with no
// parameters. If targetType is invalid, MatchContentType returns false.
func MatchContentType(headers []string, targetType string) bool {
	if len(headers) == 0 {
		return true
	}

	// target media type
	main, sub, ok := splitMediaType(targetType)

	if !ok {
		return false
	}

	// acceptance check
	for _, h := range headers {
		for part := range strings.SplitSeq(h, ",") {
			if part = trimSpace(part); len(part) == 0 {
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

// ASCII space trimming
func trimSpace(s string) string {
	for len(s) > 0 && isSpace(s[0]) {
		s = s[1:]
	}

	for len(s) > 0 && isSpace(s[len(s)-1]) {
		s = s[:len(s)-1]
	}

	return s
}

// ASCII space
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// split media type string on '/', with validation
func splitMediaType(s string) (main, sub string, ok bool) {
	if len(s) < 3 || !firstMTByte(s[0]) {
		return
	}

	i, n := 1, min(len(s), 128)

	for s[i] != '/' {
		if !otherMTByte(s[i]) {
			return
		}

		if i++; i == n {
			return
		}
	}

	if main, sub = s[:i], s[i+1:]; len(sub) == 0 || len(sub) > 127 || !firstMTByte(sub[0]) {
		return
	}

	for i = 1; i < len(sub); i++ {
		if !otherMTByte(sub[i]) {
			return
		}
	}

	ok = true
	return
}

func firstMTByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func otherMTByte(b byte) bool {
	return firstMTByte(b) || (strings.IndexByte("!#$&-^_.+", b) >= 0)
}
