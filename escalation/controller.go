package escalation

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/jlevesy/kudo/grant"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

const (
	PendingStateDetails              = "This escalation is being processed"
	AcceptedInProgressStateDetails   = "This escalation has been accepted, permissions are going to be granted in a few moments"
	AcceptedAppliedStateDetails      = "This escalation has been accepted, permissions are granted"
	ExpiredStateDetails              = "This escalation has expired, all granted permissions are reclaimed"
	DeniedPolicyNotFoundStateDetails = "This escalation references a policy that do not exist anymore, all granted permissions are reclaimed"
	DeniedPolicyChangedStateDetails  = "This escalation references a policy that has changed, all granted permissions are reclaimed"
)

var statusZero = kudov1alpha1.EscalationStatus{}

type EscalationStatusUpdater interface {
	UpdateStatus(ctx context.Context, escalation *kudov1alpha1.Escalation, opts metav1.UpdateOptions) (*kudov1alpha1.Escalation, error)
}

type Controller struct {
	policiesGetter          EscalationPoliciesGetter
	escalationStatusUpdater EscalationStatusUpdater
	granterFactory          grant.Factory
}

func NewController(
	policiesGetter EscalationPoliciesGetter,
	escalationStatusUpdater EscalationStatusUpdater,
	granterFactory grant.Factory,
) *Controller {
	return &Controller{
		policiesGetter:          policiesGetter,
		escalationStatusUpdater: escalationStatusUpdater,
		granterFactory:          granterFactory,
	}
}

func (h *Controller) OnAdd(ctx context.Context, escalation *kudov1alpha1.Escalation) error {
	policy, newStatus, updated, err := h.readPolicyAndCheckExpiration(ctx, escalation)
	if err != nil {
		return err
	}
	if updated {
		return h.updateStatus(ctx, escalation, newStatus)
	}

	return h.updateStatus(
		ctx,
		escalation,
		escalation.Status.TransitionTo(
			kudov1alpha1.StatePending,
			kudov1alpha1.WithDetails(PendingStateDetails),
			kudov1alpha1.WithPolicyInfo(policy.UID, policy.ResourceVersion),
		),
	)
}

func (h *Controller) OnUpdate(ctx context.Context, oldEsc, newEsc *kudov1alpha1.Escalation) error {
	status, err := h.reconcileState(ctx, oldEsc, newEsc)

	if err != nil {
		return err
	}

	return h.updateStatus(ctx, newEsc, status)
}

func (h *Controller) OnDelete(ctx context.Context, esc *kudov1alpha1.Escalation) error {
	// TODO(jly) try to reclaim.
	return nil
}

func hasPolicyChanged(esc *kudov1alpha1.Escalation, policy *kudov1alpha1.EscalationPolicy) bool {
	return policy.UID != esc.Status.PolicyUID ||
		policy.ResourceVersion != esc.Status.PolicyVersion
}

func (h *Controller) reconcileState(ctx context.Context, _, newEsc *kudov1alpha1.Escalation) (kudov1alpha1.EscalationStatus, error) {
	switch newEsc.Status.State {
	case kudov1alpha1.StatePending:
		policy, newStatus, updated, err := h.readPolicyAndCheckExpiration(ctx, newEsc)
		if err != nil {
			return statusZero, err
		}
		if updated {
			return newStatus, nil
		}

		// Has policy changed since the escalation was created? If so, deny the escalation.
		if hasPolicyChanged(newEsc, policy) {
			return newEsc.Status.TransitionTo(
				kudov1alpha1.StateDenied,
				kudov1alpha1.WithDetails(DeniedPolicyChangedStateDetails),
			), nil
		}

		// Policies challenges will be evaluated here.

		// if ok, transition to accepted.
		return newEsc.Status.TransitionTo(
			kudov1alpha1.StateAccepted,
			kudov1alpha1.WithDetails(AcceptedInProgressStateDetails),
		), nil

	case kudov1alpha1.StateAccepted:
		policy, newStatus, updated, err := h.readPolicyAndCheckExpiration(ctx, newEsc)
		if err != nil {
			return statusZero, err
		}
		if updated {
			return newStatus, nil
		}

		if hasPolicyChanged(newEsc, policy) {
			return newEsc.Status.TransitionTo(
				kudov1alpha1.StateDenied,
				kudov1alpha1.WithDetails(DeniedPolicyChangedStateDetails),
			), nil
		}

		return h.createGrants(ctx, newEsc, policy)

	case kudov1alpha1.StateExpired:
		grantRefs, err := h.reclaimGrants(ctx, newEsc)
		if err != nil {
			return newEsc.Status.TransitionTo(
				kudov1alpha1.StateExpired,
				kudov1alpha1.WithDetails(
					fmt.Sprintf(
						"This escalation has expired, but grants have been partially reclaimed. Reason is: %s",
						err.Error(),
					),
				),
				kudov1alpha1.WithNewGrantRefs(grantRefs),
			), nil
		}

		return newEsc.Status.TransitionTo(
			kudov1alpha1.StateExpired,
			kudov1alpha1.WithNewGrantRefs(grantRefs),
		), nil

	case kudov1alpha1.StateDenied:
		grantRefs, err := h.reclaimGrants(ctx, newEsc)
		if err != nil {
			return newEsc.Status.TransitionTo(
				kudov1alpha1.StateDenied,
				kudov1alpha1.WithDetails(
					fmt.Sprintf(
						"This escalation is denied, but grants have been partially reclaimed. Reason is: %s",
						err.Error(),
					),
				),
				kudov1alpha1.WithNewGrantRefs(grantRefs),
			), nil
		}

		return newEsc.Status.TransitionTo(
			kudov1alpha1.StateDenied,
			kudov1alpha1.WithNewGrantRefs(grantRefs),
		), nil

	default:
		return statusZero, fmt.Errorf("unsupported status %q, ignoring event", newEsc.Status.State)
	}
}

func (h *Controller) createGrants(ctx context.Context, esc *kudov1alpha1.Escalation, policy *kudov1alpha1.EscalationPolicy) (kudov1alpha1.EscalationStatus, error) {
	grantRefs := make([]kudov1alpha1.EscalationGrantRef, len(policy.Spec.Target.Grants))
	group, ctx := errgroup.WithContext(ctx)

	for i, grant := range policy.Spec.Target.Grants {
		i := i
		grant := grant

		group.Go(func() error {
			granter, err := h.granterFactory.Get(grant.Kind)
			if err != nil {
				return err
			}

			grantRefs[i], err = granter.Create(ctx, esc, grant)

			return err
		})
	}

	// If we fail to apply one target, it'll be retried in the next resync.
	if err := group.Wait(); err != nil {
		klog.ErrorS(
			err,
			"Granter reports an issue while creating",
			"escalation",
			esc.Name,
		)

		// If one of the granter being used reports that a kudo managed resource has been tampered with,
		// fail the escalation and reclaim the grants.
		if stderrors.Is(err, grant.ErrTampered) {
			return esc.Status.TransitionTo(
				kudov1alpha1.StateDenied,
				kudov1alpha1.WithDetails(
					fmt.Sprintf("Escalation has been denied, reason is: %s", err.Error()),
				),
			), nil
		}

		return esc.Status.TransitionTo(
			kudov1alpha1.StateAccepted,
			kudov1alpha1.WithDetails(
				fmt.Sprintf("Escalation is partially active, reason is: %s", err.Error()),
			),
			kudov1alpha1.WithNewGrantRefs(grantRefs),
		), nil
	}

	return esc.Status.TransitionTo(
		kudov1alpha1.StateAccepted,
		kudov1alpha1.WithDetails(AcceptedAppliedStateDetails),
		kudov1alpha1.WithNewGrantRefs(grantRefs),
	), nil
}

func (h *Controller) reclaimGrants(ctx context.Context, esc *kudov1alpha1.Escalation) ([]kudov1alpha1.EscalationGrantRef, error) {
	grantRefs := make([]kudov1alpha1.EscalationGrantRef, len(esc.Status.GrantRefs))
	group, ctx := errgroup.WithContext(ctx)

	for i, grantRef := range esc.Status.GrantRefs {
		i := i
		grantRef := grantRef

		group.Go(func() error {
			granter, err := h.granterFactory.Get(grantRef.Kind)
			if err != nil {
				return err
			}

			grantRefs[i], err = granter.Reclaim(ctx, grantRef)

			return err
		})
	}

	// If we fail to apply one target, it'll be retried in the next resync.
	if err := group.Wait(); err != nil {
		klog.ErrorS(
			err,
			"One or more reclaims have failed",
			"escalation",
			esc.Name,
		)

		return esc.Status.GrantRefs, err
	}

	return grantRefs, nil
}

func (h *Controller) readPolicyAndCheckExpiration(ctx context.Context, esc *kudov1alpha1.Escalation) (*kudov1alpha1.EscalationPolicy, kudov1alpha1.EscalationStatus, bool, error) {
	now := time.Now()

	// Does the referenced policy exists?
	policy, err := h.policiesGetter.Get(esc.Spec.PolicyName)
	switch {
	case errors.IsNotFound(err):
		return nil,
			esc.Status.TransitionTo(
				kudov1alpha1.StateDenied,
				kudov1alpha1.WithDetails(DeniedPolicyNotFoundStateDetails),
			),
			true,
			nil
	case err != nil:
		return nil, statusZero, false, err
	default:
	}

	// Is the escalation already expired? If so, transition its state and abort.
	if now.After(esc.CreationTimestamp.Add(policy.Spec.Target.Duration.Duration)) {
		return nil,
			esc.Status.TransitionTo(
				kudov1alpha1.StateExpired,
				kudov1alpha1.WithDetails(ExpiredStateDetails),
			),
			true,
			nil
	}

	return policy, statusZero, false, nil
}

func (h *Controller) updateStatus(ctx context.Context, escalation *kudov1alpha1.Escalation, status kudov1alpha1.EscalationStatus) error {
	clonedEscalation := escalation.DeepCopy()
	clonedEscalation.Status = status

	if escalation.Status.State != status.State {
		klog.InfoS(
			"Transitioning escalation",
			"escalation",
			clonedEscalation.Name,
			"oldState",
			escalation.Status.State,
			"newState",
			clonedEscalation.Status.State,
		)
	}

	_, err := h.escalationStatusUpdater.UpdateStatus(ctx, clonedEscalation, metav1.UpdateOptions{})

	return err
}
