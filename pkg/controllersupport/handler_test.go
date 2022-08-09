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
		handler     = testHandler{
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

type testHandler struct {
	addReceived    int
	updateReceived int
	deleteReceived int

	wantCalls int
	done      func()
}

func (h *testHandler) OnAdd(context.Context, *int) error {
	v := h.addReceived
	h.addReceived++
	// On first call, return transient error to test the retry behavior.
	if v == 0 {
		return controllersupport.ErrTransientError
	}
	h.call()
	return nil
}

func (h *testHandler) OnUpdate(_ context.Context, _, _ *int) error {
	h.updateReceived++
	h.call()
	return nil
}

func (h *testHandler) OnDelete(context.Context, *int) error {
	h.deleteReceived++
	h.call()
	return nil
}

func (h *testHandler) call() {
	if h.isComplete() {
		h.done()
	}
}

func (h *testHandler) isComplete() bool {
	return h.wantCalls == (h.addReceived + h.updateReceived + h.deleteReceived)
}
