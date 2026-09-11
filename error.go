package httpx

import (
	"cmp"
)

// Error is useful only within a [ContentMaker] function. It combines the given error
// with additional content of the specified Content-Type. [ServeContent] will send
// the content instead of a generic text for the status code.
func Error(err error, contentType, content string) error {
	return &problem{
		err:      err,
		cont:     content,
		contType: cmp.Or(contentType, "text/plain; charset=utf-8"),
	}
}

// error type
type problem struct {
	err            error
	cont, contType string
}

// Error returns error message string
func (p *problem) Error() string {
	return p.Unwrap().Error()
}

// Unwrap returns underlying error object
func (p *problem) Unwrap() error {
	return p.err
}
