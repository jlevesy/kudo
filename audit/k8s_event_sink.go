package audit

import (
	"context"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"k8s.io/client-go/tools/record"
)

type k8sEventSink struct {
	eventRecorder record.EventRecorder
}

func NewK8sEventSink(recorder record.EventRecorder) Sink {
	return &k8sEventSink{eventRecorder: recorder}
}

func (s *k8sEventSink) RecordCreate(ctx context.Context, escalation *kudov1alpha1.Escalation) {
	s.eventRecorder.Event(
		escalation,
		"Normal",
		"Create",
		"Escalation has been created",
	)
}

func (s *k8sEventSink) RecordUpdate(ctx context.Context, _, escalation *kudov1alpha1.Escalation) {
	s.eventRecorder.Eventf(
		escalation,
		"Normal",
		"Update",
		"New state %s, reason is: %s",
		escalation.Status.State,
		escalation.Status.StateDetails,
	)
}

func (s *k8sEventSink) RecordDelete(ctx context.Context, escalation *kudov1alpha1.Escalation) {
	s.eventRecorder.Event(
		escalation,
		"Warn",
		"Delete",
		"Escalation has been deleted",
	)
}
