package escalation

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/klog/v2"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

type EscalationStatusUpdater interface {
	UpdateStatus(ctx context.Context, escalation *kudov1alpha1.Escalation, opts metav1.UpdateOptions) (*kudov1alpha1.Escalation, error)
}

type EventHandler struct {
	policiesGetter          EscalationPoliciesGetter
	escalationStatusUpdater EscalationStatusUpdater
	rbacv1                  rbacv1client.RbacV1Interface
}

func NewEventHandler(
	policiesGetter EscalationPoliciesGetter,
	escalationStatusUpdater EscalationStatusUpdater,
	rbacV1Client rbacv1client.RbacV1Interface,
) *EventHandler {
	return &EventHandler{
		policiesGetter:          policiesGetter,
		escalationStatusUpdater: escalationStatusUpdater,
		rbacv1:                  rbacV1Client,
	}
}

func (h *EventHandler) OnAdd(ctx context.Context, escalation *kudov1alpha1.Escalation) error {
	klog.Info("RECEIVED AN ADD ==> ", escalation.Name)

	clonedEscalation := escalation.DeepCopy()
	clonedEscalation.Status.State = kudov1alpha1.StatePending
	clonedEscalation.Status.StateDetails = "The escalation is being processed"

	policy, err := h.policiesGetter.Get(escalation.Spec.PolicyName)
	if err != nil {
		// set the escalation to failed and ignore the event.
		clonedEscalation.Status.State = kudov1alpha1.StateDenied
		clonedEscalation.Status.StateDetails = fmt.Sprintf("unable to retrieve the refeenced policy: %s", err)

		if _, err = h.escalationStatusUpdater.UpdateStatus(ctx, clonedEscalation, metav1.UpdateOptions{}); err != nil {
			klog.ErrorS(err, "unable to update the escalation status")
			return err
		}

		return nil
	}

	klog.InfoS(
		"Transitioning escalation to state",
		"escalation",
		clonedEscalation.Name,
		"state",
		clonedEscalation.Status.State,
	)

	//TODO(jly) moar tests and reconciliation checks.

	for _, target := range policy.Spec.Targets {
		roleBinding, err := h.rbacv1.RoleBindings(target.Namespace).Create(
			ctx,
			&rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "RoleBinding",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      policy.Name + "-deadbeef",
					Namespace: policy.Namespace,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind: rbacv1.UserKind,
						Name: escalation.Spec.Requestor,
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.SchemeGroupVersion.Group,
					Kind:     target.RoleRef.Kind,
					Name:     target.RoleRef.Name,
				},
			},
			metav1.CreateOptions{},
		)

		if err != nil {
			clonedEscalation.Status.State = kudov1alpha1.StateDenied
			clonedEscalation.Status.StateDetails = fmt.Sprintf(
				"unable to create the role binding: %s",
				err,
			)

			return err
		}

		clonedEscalation.Status.State = kudov1alpha1.StateAccepted
		clonedEscalation.Status.StateDetails = "Escalation is active"
		clonedEscalation.Status.TargetRefs = []kudov1alpha1.EscalationTargetRef{
			{
				Kind:      "RoleBinding",
				Name:      roleBinding.Name,
				Namespace: roleBinding.Namespace,
				APIGroup:  rbacv1.SchemeGroupVersion.String(),
			},
		}
	}

	if _, err = h.escalationStatusUpdater.UpdateStatus(ctx, clonedEscalation, metav1.UpdateOptions{}); err != nil {
		klog.ErrorS(err, "unable to update the escalation status")
		return err
	}

	return nil
}

func (h *EventHandler) OnUpdate(ctx context.Context, oldEsc, newEsc *kudov1alpha1.Escalation) error {
	klog.Info("RECEIVED AN UPDATE ==> ", oldEsc.Name)
	return nil
}

func (h *EventHandler) OnDelete(ctx context.Context, esc *kudov1alpha1.Escalation) error {
	klog.Info("RECEIVED A DELETE ==> ", esc.Name)
	return nil
}
