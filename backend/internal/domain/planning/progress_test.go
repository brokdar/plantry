package planning

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

func TestProgress_Lifecycle(t *testing.T) {
	p := NewProgress()
	w := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)

	require.NoError(t, p.reserve(w))
	assert.ErrorIs(t, p.reserve(w), domain.ErrGenerationRunning, "one run per window")
	_, running := p.Running(w)
	assert.False(t, running, "reserved but not started")

	p.release(w)
	require.NoError(t, p.reserve(w), "a released window is free")

	p.begin(w, 7)
	id, running := p.Running(w)
	assert.True(t, running)
	assert.Equal(t, int64(7), id)
	st, ok := p.Stage(7)
	assert.True(t, ok)
	assert.Equal(t, StageResolving, st)
	p.setStage(7, StageSaving)
	st, _ = p.Stage(7)
	assert.Equal(t, StageSaving, st)
	assert.NoError(t, p.reserve(w.AddDate(0, 0, 7)), "another window is independent")

	p.finish(7)
	_, running = p.Running(w)
	assert.False(t, running)
	_, ok = p.Stage(7)
	assert.False(t, ok)
	assert.NoError(t, p.reserve(w), "a finished run frees its window")

	p.setStage(99, StageChoosing) // unknown runs are ignored
	p.finish(99)
}

// B4-AC2: finish closes the run's done channel, which Await waits on.
func TestProgress_DoneClosesOnFinish(t *testing.T) {
	p := NewProgress()
	w := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	require.NoError(t, p.reserve(w))
	p.begin(w, 3)
	done, ok := p.doneCh(3)
	require.True(t, ok)
	select {
	case <-done:
		t.Fatal("done before finish")
	default:
	}
	p.finish(3)
	_, open := <-done
	assert.False(t, open, "finish closes done")
	_, ok = p.doneCh(3)
	assert.False(t, ok, "a finished run has no done channel")
	p.finish(3) // a second finish is a no-op
}

// Concurrent starts on one window: a start only wins a free window, and the
// registry ends empty and consistent under -race.
func TestProgress_Concurrent(t *testing.T) {
	p := NewProgress()
	w := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	var next atomic.Int64
	for range 50 {
		var wins atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if p.reserve(w) != nil {
					return
				}
				wins.Add(1)
				id := next.Add(1)
				p.begin(w, id)
				p.setStage(id, StageChoosing)
				_, _ = p.Stage(id)
				_, _ = p.Running(w)
				p.finish(id)
			}()
		}
		wg.Wait()
		assert.GreaterOrEqual(t, wins.Load(), int32(1))
		_, running := p.Running(w)
		assert.False(t, running)
	}
	assert.Empty(t, p.runs)
	assert.Empty(t, p.windows)
}
