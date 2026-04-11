package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeJson(t *testing.T) {
	tests := []struct {
		name            string
		acceptHeader    string
		fn              func(*json.Encoder) error
		wantStatus      int
		wantContentType string
		wantBody        string
		wantErr         bool
	}{
		{
			name:         "exact match - application/json accepted",
			acceptHeader: "application/json",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
		{
			name:         "wildcard subtype - application/* accepts json",
			acceptHeader: "application/*",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
		{
			name:         "full wildcard - */* accepts json",
			acceptHeader: "*/*",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
		{
			name:         "multiple types - json later in list",
			acceptHeader: "text/html, application/json;q=0.9, */*;q=0.8",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
		{
			name:         "empty accept header - accepts everything",
			acceptHeader: "",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
		{
			name:         "no matching accept type",
			acceptHeader: "text/html, application/xml",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus: http.StatusNotAcceptable,
			wantBody:   "Not Acceptable\n",
			wantErr:    true,
		},
		{
			name:         "q=0 for json - not acceptable",
			acceptHeader: "application/json;q=0, text/html",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus: http.StatusNotAcceptable,
			wantBody:   "Not Acceptable\n",
			wantErr:    true,
		},
		{
			name:         "malformed accept header - skips and finds match",
			acceptHeader: "invalid;q=1.0, application/json",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
		{
			name:         "fn returns error",
			acceptHeader: "application/json",
			fn: func(enc *json.Encoder) error {
				return errors.New("encoding failed")
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Internal Server Error\n",
			wantErr:    true,
		},
		{
			name:         "case insensitive match",
			acceptHeader: "Application/Json",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
		{
			name:         "json with charset parameter",
			acceptHeader: "application/json;charset=utf-8",
			fn: func(enc *json.Encoder) error {
				return enc.Encode(map[string]string{"key": "value"})
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"key":"value"}` + "\n",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)

			if tt.acceptHeader != "" {
				r.Header.Set("Accept", tt.acceptHeader)
			}

			err := ServeJson(w, r, tt.fn)

			if (err != nil) != tt.wantErr {
				t.Errorf("ServeJson() error = %v, wantErr %v", err, tt.wantErr)
			}

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("ServeJson() status = %v, want %v", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantContentType != "" {
				if ct := resp.Header.Get("Content-Type"); ct != tt.wantContentType {
					t.Errorf("ServeJson() Content-Type = %v, want %v", ct, tt.wantContentType)
				}
			}

			if tt.wantBody != "" {
				if body, _ := io.ReadAll(resp.Body); string(body) != tt.wantBody {
					t.Errorf("ServeJson() body = %q, want %q", string(body), tt.wantBody)
				}
			}
		})
	}
}
