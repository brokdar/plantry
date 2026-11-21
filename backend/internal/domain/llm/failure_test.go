package llm_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

type statusErr int

func (e statusErr) Error() string                     { return fmt.Sprintf("status %d", int(e)) }
func (e statusErr) ProviderStatus() int               { return int(e) }
func (statusErr) ProviderErrorType() string           { return "" }
func (statusErr) ProviderInfo() llm.ProviderErrorInfo { return llm.ProviderErrorInfo{} }

type netTimeout struct{}

func (netTimeout) Error() string   { return "i/o timeout" }
func (netTimeout) Timeout() bool   { return true }
func (netTimeout) Temporary() bool { return true }

func TestFailureOf(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want llm.Failure
	}{
		"429":              {statusErr(429), llm.FailureRateLimited},
		"overloaded":       {statusErr(529), llm.FailureRateLimited},
		"401":              {statusErr(401), llm.FailureKeyRejected},
		"403 wrapped":      {fmt.Errorf("x: %w", statusErr(403)), llm.FailureKeyRejected},
		"400":              {statusErr(400), llm.FailureOther},
		"500":              {statusErr(500), llm.FailureOther},
		"deadline":         {fmt.Errorf("post: %w", context.DeadlineExceeded), llm.FailureTimeout},
		"network timeout":  {fmt.Errorf("post: %w", netTimeout{}), llm.FailureTimeout},
		"cancelled":        {context.Canceled, llm.FailureOther},
		"connection reset": {errors.New("connection reset by peer"), llm.FailureOther},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tc.want, llm.FailureOf(tc.err)) })
	}
}
