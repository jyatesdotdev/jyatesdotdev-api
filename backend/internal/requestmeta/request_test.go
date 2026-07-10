package requestmeta

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name string
		body string
		max  int64
		want error
	}{
		{name: "valid", body: `{"name":"Ada"}`, max: 128},
		{name: "unknown field", body: `{"name":"Ada","admin":true}`, max: 128, want: assert.AnError},
		{name: "multiple values", body: `{"name":"Ada"} {"name":"Grace"}`, max: 128, want: assert.AnError},
		{name: "too large", body: `{"name":"Ada"}`, max: 4, want: ErrBodyTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			var got payload
			err := DecodeJSON(w, req, &got, tt.max)

			if tt.want == nil {
				assert.NoError(t, err)
				assert.Equal(t, "Ada", got.Name)
				return
			}
			if errors.Is(tt.want, ErrBodyTooLarge) {
				assert.ErrorIs(t, err, ErrBodyTooLarge)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		cloudFront string
		xff        string
		remoteAddr string
		want       string
	}{
		{name: "cloudfront ipv4 with port", cloudFront: "198.51.100.4:43210", xff: "203.0.113.8", remoteAddr: "192.0.2.2:80", want: "198.51.100.4"},
		{name: "cloudfront ipv6 with port", cloudFront: "[2001:db8::1]:43210", want: "2001:db8::1"},
		{name: "forwarded fallback", xff: "203.0.113.8, 192.0.2.1", want: "203.0.113.8"},
		{name: "remote fallback", remoteAddr: "192.0.2.2:80", want: "192.0.2.2"},
		{name: "invalid", remoteAddr: "not-an-address", want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("CloudFront-Viewer-Address", tt.cloudFront)
			req.Header.Set("X-Forwarded-For", tt.xff)
			req.RemoteAddr = tt.remoteAddr
			assert.Equal(t, tt.want, ClientIP(req))
		})
	}
}
