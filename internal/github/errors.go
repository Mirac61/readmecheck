package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is matched by HTTP 404 responses from the GitHub API.
var ErrNotFound = errors.New("github: not found")

// HTTPError is a non-success response from the provider.
type HTTPError struct {
	StatusCode int
	Message    string
	Path       string
}

func (e *HTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub returned HTTP %d for %s", e.StatusCode, e.Path)
	}
	return fmt.Sprintf("GitHub returned HTTP %d for %s: %s", e.StatusCode, e.Path, e.Message)
}

func (e *HTTPError) Is(target error) bool {
	return target == ErrNotFound && (e.StatusCode == http.StatusNotFound || e.StatusCode == http.StatusGone)
}

// RateLimitError reports a primary or secondary GitHub API rate limit.
type RateLimitError struct {
	Reset     time.Time
	Limit     int
	Remaining int
	Message   string
}

func (e *RateLimitError) Error() string {
	if e.Reset.IsZero() {
		return "GitHub API rate limit exceeded"
	}
	return fmt.Sprintf("GitHub API rate limit exceeded; it resets at %s", e.Reset.UTC().Format(time.RFC3339))
}

func newHTTPError(resp *http.Response, body []byte) error {
	if isRateLimited(resp, body) {
		return parseRateLimitError(resp, body)
	}
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Message:    apiMessage(body, resp.Status),
		Path:       resp.Request.URL.Path,
	}
}

func isRateLimited(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return false
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "" {
		return true
	}
	message := strings.ToLower(apiMessage(body, ""))
	return strings.Contains(message, "rate limit")
}

func parseRateLimitError(resp *http.Response, body []byte) *RateLimitError {
	err := &RateLimitError{Message: apiMessage(body, resp.Status)}
	err.Limit, _ = strconv.Atoi(resp.Header.Get("X-RateLimit-Limit"))
	err.Remaining, _ = strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	if unix, parseErr := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); parseErr == nil && unix > 0 {
		err.Reset = time.Unix(unix, 0).UTC()
	}
	if err.Reset.IsZero() {
		if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
			err.Reset = time.Now().UTC().Add(time.Duration(seconds) * time.Second)
		}
	}
	return err
}

func apiMessage(body []byte, fallback string) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Message != "" {
		return payload.Message
	}
	if fallback == "" {
		return http.StatusText(http.StatusBadRequest)
	}
	return fallback
}
