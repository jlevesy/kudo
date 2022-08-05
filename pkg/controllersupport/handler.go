package controllersupport

import (
	"context"
	"errors"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// ErrTransientError can be returned by an EventHandler to trigger a retry.
var ErrTransientError = errors.New("transient error")

// EventHandler represents a object that handles mutation event  a k8s object.
// It is based on k8s-clen cache.EventHandler.
type EventHandler[T any] interface {
	OnAdd(obj *T) error
	OnUpdate(oldObj, newObj *T) error
	OnDelete(obj *T) error
}

// QueuedEventHandler implements cache.EventHandler over a workqueue.
// It also handles the type conversion of updated objects to pass them down to an EventHandler.
type QueuedEventHandler[T any] struct {
	workqueue workqueue.RateLimitingInterface
	name      string
	workers   int

	handler EventHandler[T]
}

// NewQueuedEventHandler returns a queued event handler
func NewQueuedEventHandler[T any](handler EventHandler[T], name string, workers int) *QueuedEventHandler[T] {
	return &QueuedEventHandler[T]{
		workqueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			name,
		),

		handler: handler,
		workers: workers,
		name:    name,
	}
}

// OnAdd enqueues an add event.
func (h *QueuedEventHandler[T]) OnAdd(obj any) {
	h.workqueue.Add(queueEvent{kind: kindAdd, object: obj})
}

// OnUpdate enqueues an update event.
func (h *QueuedEventHandler[T]) OnUpdate(oldObj, newObj any) {
	h.workqueue.Add(queueEvent{kind: kindUpdate, oldObj: oldObj, newObj: newObj})
}

// OnDelete enqueues an update event.
func (h *QueuedEventHandler[T]) OnDelete(obj any) {
	h.workqueue.Add(queueEvent{kind: kindDelete, object: obj})
}

// Run starts workers and waits until completion.
func (h *QueuedEventHandler[T]) Run(ctx context.Context) {
	defer utilruntime.HandleCrash()
	defer h.workqueue.ShutDown()

	klog.InfoS("Starting workers for handler", "name", h.name, "total", h.workers)

	for i := 0; i < h.workers; i++ {
		go wait.UntilWithContext(ctx, h.runWorker, time.Second)
	}

	klog.InfoS("Started workers for handler", "name", h.name)

	<-ctx.Done()

	klog.InfoS("Shutting down workers for handler", "name", h.name)
}

func (h *QueuedEventHandler[T]) runWorker(ctx context.Context) {
	for h.processItem(ctx) {
	}
}

func (h *QueuedEventHandler[T]) processItem(ctx context.Context) bool {
	obj, shutdown := h.workqueue.Get()

	if shutdown {
		return false
	}

	defer h.workqueue.Done(obj)

	event, ok := obj.(queueEvent)

	if !ok {
		h.workqueue.Forget(obj)
		return true
	}

	var err error

	switch event.kind {
	case kindAdd:
		typedObject, ok := event.object.(*T)
		if !ok {
			h.workqueue.Forget(obj)
			return true
		}

		err = h.handler.OnAdd(typedObject)
	case kindUpdate:
		typedOldObject, okOld := event.oldObj.(*T)
		typedNewObject, okNew := event.newObj.(*T)

		if !okOld || !okNew {
			h.workqueue.Forget(obj)
			return true
		}

		err = h.handler.OnUpdate(typedOldObject, typedNewObject)

	case kindDelete:
		typedObject, ok := event.object.(*T)
		if !ok {
			h.workqueue.Forget(obj)
			return true
		}

		err = h.handler.OnDelete(typedObject)

	default:
		h.workqueue.Forget(obj)
		return true
	}

	if errors.Is(err, ErrTransientError) {
		klog.ErrorS(err, "handler reported a transient error, requeuing...", "name", h.name)
		h.workqueue.AddRateLimited(obj)
		return true
	}

	if err != nil {
		klog.ErrorS(err, "handler reported an error", "name", h.name)
	}

	h.workqueue.Forget(obj)

	return true
}

type queueEventKind uint8

const (
	// forces assignation of an explicit value when using this enumeration.
	kindUnknown queueEventKind = iota //nolint:deadcode,varcheck
	kindAdd
	kindUpdate
	kindDelete
)

type queueEvent struct {
	kind   queueEventKind
	object any
	oldObj any
	newObj any
}
