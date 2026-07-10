package requestmeta

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

var ErrBodyTooLarge = errors.New("request body too large")

// DecodeJSON applies a hard body limit and accepts exactly one JSON object with
// no unknown fields. Keeping this at the HTTP boundary avoids passing ambiguous
// or unexpectedly large input into application services.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return normalizeDecodeError(err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return normalizeDecodeError(err)
	}

	return nil
}

func normalizeDecodeError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return ErrBodyTooLarge
	}
	return err
}

// ClientIP prefers the CloudFront-generated viewer address. X-Forwarded-For is
// retained only as a fallback for direct/local execution where CloudFront is not
// present; production infrastructure forwards CloudFront-Viewer-Address.
func ClientIP(r *http.Request) string {
	if ip := parseIP(r.Header.Get("CloudFront-Viewer-Address")); ip != "" {
		return ip
	}

	if first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ","); first != "" {
		if ip := parseIP(first); ip != "" {
			return ip
		}
	}

	if ip := parseIP(r.RemoteAddr); ip != "" {
		return ip
	}
	return "unknown"
}

func parseIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")

	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return ""
}
