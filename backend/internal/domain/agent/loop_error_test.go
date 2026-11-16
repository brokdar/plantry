package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

type provErr struct{}

func (provErr) Error() string                       { return "400 invalid_request_error" }
func (provErr) ProviderStatus() int                 { return 400 }
func (provErr) ProviderErrorType() string           { return "invalid_request_error" }
func (provErr) ProviderInfo() llm.ProviderErrorInfo { return llm.ProviderErrorInfo{} }

type failClient struct{ err error }

func (c failClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	close(out)
	return nil, c.err
}

func (c failClient) Complete(_ context.Context, _ llm.Request) (string, error) { return "", c.err }

// A provider 400 about the request is a provider error, not an interrupted
// stream, and the loop reports it exactly once.
func TestRun_ProviderErrorKey(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		err error
		key string
	}{
		"provider":  {provErr{}, "error.ai.provider_error"},
		"transport": {errors.New("connection reset"), "error.ai.stream_interrupted"},
	} {
		t.Run(name, func(t *testing.T) {
			ts := minimalToolSet(t, "noop", json.RawMessage(`{}`), ToolEffectNone, nil)
			out := make(chan llm.Event, 32)
			err := Run(context.Background(), makeReq(1), failClient{tc.err}, ts, &memRepo{}, out)
			close(out)
			require.Error(t, err)
			assert.True(t, ErrorEmitted(err))
			var keys []string
			for e := range out {
				if e.Type == llm.EventError {
					keys = append(keys, e.Payload.(llm.StreamErrorPayload).MessageKey)
				}
			}
			assert.Equal(t, []string{tc.key}, keys)
			_, key := chatOutcome(context.Background(), err)
			assert.Equal(t, tc.key, key)
		})
	}
}
