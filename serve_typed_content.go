package httpx

import (
	"cmp"
	"errors"
	"io"
	"net/http"
	"strconv"
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

	return sendErrStr(
		w,
		http.StatusNotAcceptable,
		`client does not accept content of type "`+contentType+`"`,
	)
}

func sendErrStr(w http.ResponseWriter, code int, msg string) error {
	http.Error(w, http.StatusText(code), code)
	return errors.New("(" + strconv.Itoa(code) + ") " + msg)
}
