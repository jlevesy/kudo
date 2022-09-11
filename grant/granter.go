package grant

import (
	"context"
	"errors"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

var (
	ErrTampered = errors.New("kudo managed resource has been tampered with")
)

// Granter allows to create or reclaim a grant.
type Granter interface {
	// Create provision a new grant. It is expected to be idempotent for an escalation and a grant.
	Create(context.Context, *kudov1alpha1.Escalation, kudov1alpha1.ValueWithKind) (kudov1alpha1.EscalationGrantRef, error)

	// Reclaim reclaims a given grant.
	Reclaim(context.Context, kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error)

	// Validate returns an error if the given escalation is not compatible with the given grant.
	// It is used in webhook early catch configuration issues.
	Validate(context.Context, *kudov1alpha1.Escalation, kudov1alpha1.ValueWithKind) error
}
