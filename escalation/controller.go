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

	"github.com/jlevesy/kudo/audit"
	"github.com/jlevesy/kudo/grant"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
)

const (
	PendingStateDetails              = "This escalation is being processed"
	AcceptedInProgressStateDetails   = "This escalation has been accepted, permissions are going to be granted in a few moments"
	AcceptedAppliedStateDetails      = "This escalation has been accepted, permissions are granted"
	ExpiredStateDetails              = "This escalation has expired, all granted permissions are reclaimed"
	ExpiredStateWillReclaimDetails   = "This escalation has expired, all granted permissions are going to be reclaimed"
	DeniedBadEscalationSpecDetails   = "This escalation does not have necessary information, it is denied"
	DeniedPolicyNotFoundStateDetails = "This escalation references a policy that do not exist anymore, all granted permissions are reclaimed"
	DeniedPolicyChangedStateDetails  = "This escalation references a policy that has changed, all granted permissions are reclaimed"
)

var statusZero = kudov1alpha1.EscalationStatus{}

type EscalationStatusUpdater interface {
	UpdateStatus(ctx context.Context, escalation *kudov1alpha1.Escalation, opts metav1.UpdateOptions) (*kudov1alpha1.Escalation, error)
}

type EventInsight = controllersupport.EventInsight[kudov1alpha1.Escalation]

type Controller struct {
	policiesGetter          EscalationPoliciesGetter
	escalationStatusUpdater EscalationStatusUpdater
	granterFactory          grant.Factory
	auditSink               audit.Sink

	nowFunc        func() time.Time
	resyncInterval time.Duration
	retryInterval  time.Duration
}

type ControllerOpt func(c *Controller)

func WithNowFunc(now func() time.Time) ControllerOpt {
	return func(c *Controller) {
		c.nowFunc = now
	}
}

func WithResyncInterval(d time.Duration) ControllerOpt {
	return func(c *Controller) {
		c.resyncInterval = d
	}
}

func WithRetryInterval(d time.Duration) ControllerOpt {
	return func(c *Controller) {
		c.retryInterval = d
	}
}

func NewController(
	policiesGetter EscalationPoliciesGetter,
	escalationStatusUpdater EscalationStatusUpdater,
	granterFactory grant.Factory,
	auditSink audit.Sink,
	opts ...ControllerOpt,
) *Controller {
	c := Controller{
		policiesGetter:          policiesGetter,
		escalationStatusUpdater: escalationStatusUpdater,
		granterFactory:          granterFactory,
		auditSink:               auditSink,
		nowFunc:                 time.Now,
		resyncInterval:          30 * time.Second,
		retryInterval:           5 * time.Second,
	}

	for _, opt := range opts {
		opt(&c)
	}

	return &c
}

func (c *Controller) OnAdd(ctx context.Context, escalation *kudov1alpha1.Escalation) (EventInsight, error) {
	c.auditSink.RecordCreate(ctx, escalation)

	if !escalation.Spec.IsValid() {
		_, err := c.updateStatus(
			ctx,
			escalation,
			escalation.Status.TransitionTo(
				kudov1alpha1.StateDenied,
				kudov1alpha1.WithDetails(DeniedBadEscalationSpecDetails),
			),
		)

		return EventInsight{}, err
	}

	policy, newStatus, updated, err := c.readPolicyAndCheckExpiration(ctx, escalation)
	if err != nil {
		return EventInsight{}, err
	}
	if updated {
		_, err = c.updateStatus(ctx, escalation, newStatus)
		return EventInsight{}, err
	}

	_, err = c.updateStatus(
		ctx,
		escalation,
		escalation.Status.TransitionTo(
			kudov1alpha1.StatePending,
			kudov1alpha1.WithDetails(PendingStateDetails),
			kudov1alpha1.WithPolicyInfo(policy.UID, policy.ResourceVersion),
		),
	)

	return EventInsight{}, err
}

func (c *Controller) OnUpdate(ctx context.Context, _, esc *kudov1alpha1.Escalation) (EventInsight, error) {
	status, err := c.reconcileState(ctx, esc)
	if err != nil {
		return EventInsight{}, err
	}

	updatedEsc, err := c.updateStatus(ctx, esc, status)
	if err != nil {
		return EventInsight{}, err
	}

	// If the resource has been updated, we'll get an update event, no need to reschedule anything.
	if esc.ResourceVersion != updatedEsc.ResourceVersion {
		return EventInsight{}, nil
	}

	// If no update, then let's pick an appropriate next schedule time.
	nextInsight := c.nextEventInsight(updatedEsc)

	if nextInsight.ResyncAfter > 0 {
		klog.InfoS("Next processing scheduled", "escalation", updatedEsc.Name, "in", nextInsight.ResyncAfter)
	}

	return nextInsight, nil
}

func (c *Controller) OnDelete(ctx context.Context, esc *kudov1alpha1.Escalation) (EventInsight, error) {
	klog.InfoS("Escalation deleted, reclaiming permissions", "escalation", esc.Name)

	c.auditSink.RecordDelete(ctx, esc)

	_, err := c.reclaimGrants(ctx, esc)
	return EventInsight{}, err
}

func (c *Controller) reconcileState(ctx context.Context, newEsc *kudov1alpha1.Escalation) (kudov1alpha1.EscalationStatus, error) {
	switch newEsc.Status.State {
	case kudov1alpha1.StatePending:
		policy, newStatus, updated, err := c.readPolicyAndCheckExpiration(ctx, newEsc)
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
			kudov1alpha1.WithExpiresAt(c.nowFunc().Add(policy.Spec.Target.Duration.Duration)),
			kudov1alpha1.WithDetails(AcceptedInProgressStateDetails),
		), nil

	case kudov1alpha1.StateAccepted:
		policy, newStatus, updated, err := c.readPolicyAndCheckExpiration(ctx, newEsc)
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

		return c.createGrants(ctx, newEsc, policy)

	case kudov1alpha1.StateExpired:
		grantRefs, err := c.reclaimGrants(ctx, newEsc)
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
		grantRefs, err := c.reclaimGrants(ctx, newEsc)
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

func (c *Controller) createGrants(ctx context.Context, esc *kudov1alpha1.Escalation, policy *kudov1alpha1.EscalationPolicy) (kudov1alpha1.EscalationStatus, error) {
	grantRefs := make([]kudov1alpha1.EscalationGrantRef, len(policy.Spec.Target.Grants))
	group, ctx := errgroup.WithContext(ctx)

	for i, grant := range policy.Spec.Target.Grants {
		i := i
		grant := grant

		group.Go(func() error {
			granter, err := c.granterFactory.Get(grant.Kind)
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

func (c *Controller) readPolicyAndCheckExpiration(ctx context.Context, esc *kudov1alpha1.Escalation) (*kudov1alpha1.EscalationPolicy, kudov1alpha1.EscalationStatus, bool, error) {
	// Does the referenced policy exists?
	policy, err := c.policiesGetter.Get(esc.Spec.PolicyName)
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
	if c.nowFunc().After(esc.CreationTimestamp.Add(policy.Spec.Target.Duration.Duration)) {
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

func (c *Controller) updateStatus(ctx context.Context, escalation *kudov1alpha1.Escalation, status kudov1alpha1.EscalationStatus) (*kudov1alpha1.Escalation, error) {
	clonedEscalation := escalation.DeepCopy()
	clonedEscalation.Status = status

	newEsc, err := c.escalationStatusUpdater.UpdateStatus(ctx, clonedEscalation, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	// When there's a state update, record it.
	if newEsc.ResourceVersion != escalation.ResourceVersion {
		c.auditSink.RecordUpdate(ctx, escalation, newEsc)
	}

	return newEsc, nil
}

func (c *Controller) nextEventInsight(esc *kudov1alpha1.Escalation) EventInsight {
	switch esc.Status.State {
	case kudov1alpha1.StateAccepted:
		if !esc.Status.AllGrantsInStatus(kudov1alpha1.GrantStatusCreated) {
			return EventInsight{
				ResyncAfter: c.retryInterval,
				Object:      esc,
			}
		}

		var (
			resyncDelay       = c.resyncInterval
			delayToExpiration = esc.Status.ExpiresAt.Sub(c.nowFunc())
		)

		if delayToExpiration < resyncDelay {
			resyncDelay = delayToExpiration
		}

		return EventInsight{
			ResyncAfter: resyncDelay,
			Object:      esc,
		}
	case kudov1alpha1.StateDenied, kudov1alpha1.StateExpired:
		if !esc.Status.AllGrantsInStatus(kudov1alpha1.GrantStatusReclaimed) {
			return EventInsight{
				ResyncAfter: c.retryInterval,
				Object:      esc,
			}
		}

		klog.InfoS("Not resyncing because denied / expired and all reclaimed", "escalation", esc.Name)
		return EventInsight{}
	default:
		klog.InfoS("Not resyncing because in unreschedulable state", "escalation", esc.Name, "state", esc.Status.State)
		return EventInsight{}
	}
}

func hasPolicyChanged(esc *kudov1alpha1.Escalation, policy *kudov1alpha1.EscalationPolicy) bool {
	return policy.UID != esc.Status.PolicyUID ||
		policy.ResourceVersion != esc.Status.PolicyVersion
}
