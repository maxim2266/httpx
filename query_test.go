package httpx

import (
	"maps"
	"net/url"
	"slices"
	"testing"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    map[string][]string
		wantErr bool
	}{
		{"basic", "a=1&b=2", map[string][]string{"a": {"1"}, "b": {"2"}}, false},
		{"no-key", "a=1&b", map[string][]string{"a": {"1"}, "b": {""}}, false},
		{"multi", "a=1&a=2", map[string][]string{"a": {"1", "2"}}, false},
		{"empty", "", map[string][]string{}, false},
		{"invalid", "a=1&%zz", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter, err := ParseQuery(&url.URL{RawQuery: tt.query})

			if tt.wantErr && err == nil {
				t.Fatal("missing error")
			} else if !tt.wantErr && err != nil {
				t.Fatal("unexpected error: " + err.Error())
			}

			if !tt.wantErr {
				got := make(map[string][]string)

				for k, v := range iter {
					got[k] = append(got[k], v)
				}

				if !maps.EqualFunc(got, tt.want, slices.Equal) {
					t.Fatalf("got %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}
