package httpx

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
)

// Error constructs an error object with additional content to respond with,
// and of the given Content-Type. The actual content sending is handled in
// [ServeContent] function.
func Error(err error, contType, cont string) error {
	if err == nil {
		return errors.New("nil error in httpx.Error")
	}

	return &problem{
		err:      err,
		cont:     cont,
		contType: cmp.Or(contType, "text/plain; charset=utf-8"),
	}
}

// error type
type problem struct {
	err            error
	cont, contType string
}

// Error returns error message string
func (p *problem) Error() string {
	return p.err.Error()
}

// Unwrap returns underlying error object
func (p *problem) Unwrap() error {
	return p.err
}

// send error response; returns the actual status along with unwrapped and formatted error
func (p *problem) report(w http.ResponseWriter, status int, prefix string) (int, error) {
	h := w.Header()

	// set headers like http.Error does
	h.Del("Content-Length")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Type", p.contType)

	// status must be 4xx or 5xx
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}

	// write response
	w.WriteHeader(status)

	err := p.Unwrap()

	if _, e := writeString(w, p.cont); e != nil {
		err = errors.Join(err, e)
	}

	// error prefix
	if len(prefix) > 0 {
		err = fmt.Errorf("%s: %w", prefix, err)
	}

	return status, err
}
