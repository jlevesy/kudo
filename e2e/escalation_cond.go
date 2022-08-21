package e2e

import (
	"k8s.io/apimachinery/pkg/runtime"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

type escalationWaitCondSpec struct {
	state         kudov1alpha1.EscalationState
	grantStatuses []kudov1alpha1.GrantStatus
}

func condEscalationStatusMatchesSpec(spec escalationWaitCondSpec) updateCondition {
	return func(obj runtime.Object) bool {
		esc, ok := obj.(*kudov1alpha1.Escalation)
		if !ok {
			return false
		}

		return escalationStatusMatchesSpec(spec, esc.Status)
	}
}

func escalationStatusMatchesSpec(want escalationWaitCondSpec, got kudov1alpha1.EscalationStatus) bool {
	if want.state != got.State {
		return false
	}

	if len(want.grantStatuses) != len(got.GrantRefs) {
		return false
	}

	for i, refStatus := range want.grantStatuses {
		if refStatus != got.GrantRefs[i].Status {
			return false
		}
	}

	return true
}
