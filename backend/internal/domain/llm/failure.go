package llm

import (
	"context"
	"errors"
	"net"
)

// Failure says what a failed provider call means for the user, so the message
// can tell them what to do: wait, fix the key, or just try again.
type Failure string

// The failures that call for their own message; every other failure is
// FailureOther.
const (
	FailureOther       Failure = ""
	FailureRateLimited Failure = "rate_limited"
	FailureKeyRejected Failure = "key_rejected"
	FailureTimeout     Failure = "timeout"
)

// FailureOf classifies a provider call error: HTTP 429 and 529 (a limit or an
// overloaded provider), 401 and 403 (the key), and a deadline or network
// timeout.
func FailureOf(err error) Failure {
	var perr ProviderError
	if errors.As(err, &perr) {
		switch perr.ProviderStatus() {
		case 429, 529:
			return FailureRateLimited
		case 401, 403:
			return FailureKeyRejected
		}
	}
	var nerr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()) {
		return FailureTimeout
	}
	return FailureOther
}
