package audit

import (
	"context"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"k8s.io/klog/v2"
)

type Sink interface {
	RecordCreate(ctx context.Context, esc *kudov1alpha1.Escalation)
	RecordUpdate(ctx context.Context, oldEsc, newEsc *kudov1alpha1.Escalation)
	RecordDelete(ctx context.Context, esc *kudov1alpha1.Escalation)
}

func MutliAsyncSink(sinks ...Sink) Sink { return multiAsyncSink(sinks) }

type multiAsyncSink []Sink

func (m multiAsyncSink) RecordCreate(ctx context.Context, esc *kudov1alpha1.Escalation) {
	m.asyncDo(func(s Sink) {
		s.RecordCreate(ctx, esc)
	})
}

func (m multiAsyncSink) RecordUpdate(ctx context.Context, oldEsc, newEsc *kudov1alpha1.Escalation) {
	m.asyncDo(func(s Sink) {
		s.RecordUpdate(ctx, oldEsc, newEsc)
	})
}

func (m multiAsyncSink) RecordDelete(ctx context.Context, esc *kudov1alpha1.Escalation) {
	m.asyncDo(func(s Sink) {
		s.RecordDelete(ctx, esc)
	})
}

func (m multiAsyncSink) asyncDo(callback func(Sink)) {
	for _, sink := range m {
		sink := sink
		go func() {
			defer func() {
				if err := recover(); err != nil {
					klog.Error(err, "recovered panic from sink")
				}
			}()

			callback(sink)
		}()
	}
}
