package httpx

import (
	"iter"
	"net/url"
)

// ParseQuery takes a raw query from the given URL, parses it, and returns an iterator
// over the result that yields a sequence of key/value pairs.
func ParseQuery(u *url.URL) (iter.Seq2[string, string], error) {
	m, err := url.ParseQuery(u.RawQuery)

	if err != nil {
		return nil, err
	}

	return func(yield func(string, string) bool) {
		for k, arr := range m {
			for _, v := range arr {
				if !yield(k, v) {
					return
				}
			}
		}
	}, nil
}
