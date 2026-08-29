package httpx

import (
	"cmp"
	"errors"
	"io"
	"net/http"
)

// ServeTypedContent checks if the client accepts the given content type, and then
// serves the content via [ServeContent] function.
func ServeTypedContent(
	w http.ResponseWriter,
	r *http.Request,
	contentType string,
	fn func(io.Writer) error,
) error {
	if MatchContentType(r.Header.Values("Accept"), contentType) {
		w.Header().Set("Content-Type", cmp.Or(contentType, "application/octet-stream"))
		return ServeContent(w, r, fn)
	}

	sendHttpErr(w, http.StatusNotAcceptable)

	return errors.New(`httpx.ServeTypedContent: (406) client does not accept content of type "` +
		contentType + `"`)
}
