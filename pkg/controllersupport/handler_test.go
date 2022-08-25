package controllersupport_test

import (
	"context"
	"testing"
	"time"

	"github.com/jlevesy/kudo/pkg/controllersupport"
	"github.com/jlevesy/kudo/pkg/generics"
	"github.com/stretchr/testify/assert"
)

func TestQueuedEventHandler_HandlesEvent(t *testing.T) {
	var (
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		handler     = testHandler[int]{
			wantCalls: 4,
			done:      cancel,
		}
		eventHandler = controllersupport.NewQueuedEventHandler[int](
			&handler,
			"test",
			1,
		)
	)

	defer cancel()

	// Enqueue some calls.
	eventHandler.OnAdd(generics.Ptr(4))
	// Make sure that invalid calls are ignored...
	eventHandler.OnAdd(nil)
	eventHandler.OnUpdate(generics.Ptr(4), generics.Ptr(4))
	eventHandler.OnUpdate(nil, nil)
	eventHandler.OnDelete(generics.Ptr(4))
	eventHandler.OnDelete(nil)

	// Run the queue until completion.
	eventHandler.Run(ctx)

	assert.True(t, handler.isComplete())
}

func TestQueuedEventHandler_RequeuesAfter(t *testing.T) {
	var (
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		v           = generics.Ptr(4)
		handler     = testHandler[int]{
			wantCalls: 2,
			insight: controllersupport.EventInsight[int]{
				ResyncAfter: time.Millisecond,
				Object:      v,
			},
			done: cancel,
		}
		eventHandler = controllersupport.NewQueuedEventHandler[int](
			&handler,
			"test",
			1,
		)
	)

	defer cancel()

	// Enqueue some callvs.
	eventHandler.OnUpdate(v, v)

	// Run the queue until completion.
	eventHandler.Run(ctx)

	assert.True(t, handler.isComplete())
}

type testHandler[T any] struct {
	addReceived    int
	updateReceived int
	deleteReceived int
	insight        controllersupport.EventInsight[T]

	wantCalls int
	done      func()
}

func (h *testHandler[T]) OnAdd(context.Context, *int) (controllersupport.EventInsight[T], error) {
	v := h.addReceived
	h.addReceived++
	// On first call, return transient error to test the retry behavior.
	if v == 0 {
		return controllersupport.EventInsight[T]{}, controllersupport.ErrTransientError
	}
	h.call()
	return h.insight, nil
}

func (h *testHandler[T]) OnUpdate(_ context.Context, _, _ *int) (controllersupport.EventInsight[T], error) {
	h.updateReceived++
	h.call()
	return h.insight, nil
}

func (h *testHandler[T]) OnDelete(context.Context, *int) (controllersupport.EventInsight[T], error) {
	h.deleteReceived++
	h.call()
	return h.insight, nil
}

func (h *testHandler[T]) call() {
	if h.isComplete() {
		h.done()
	}
}

func (h *testHandler[T]) isComplete() bool {
	return h.wantCalls == (h.addReceived + h.updateReceived + h.deleteReceived)
}
