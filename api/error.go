package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Error is a response from Discogs with a status of 400 or above. Message is
// the message field of the body when Discogs sent one, and the standard text
// for the status otherwise.
type Error struct {
	StatusCode int    `json:"status"`
	Message    string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("discogs: %s (HTTP %d)", e.Message, e.StatusCode)
}

// errEmptyBody is returned for a successful status with no body, which is how
// Discogs answers a request it does not accept the User-Agent of.
var errEmptyBody = errors.New("discogs: empty response body")

func newError(status int, body []byte) *Error {
	var payload struct {
		Message string `json:"message"`
	}
	msg := http.StatusText(status)
	if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Message) != "" {
		msg = payload.Message
	}
	return &Error{StatusCode: status, Message: msg}
}
