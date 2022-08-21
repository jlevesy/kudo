package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/granter"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

func TestEscalation_RoleBinding(t *testing.T) {
	t.Parallel()

	var (
		ctx       = context.Background()
		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: k8sTestName(t),
			},
		}
		role = rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sTestName(t),
				Namespace: namespace.Name,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{""},
					Resources: []string{"pods"},
				},
			},
		}
		policyName = "policy-" + k8sTestName(t)
		policy     = &kudov1alpha1.EscalationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: policyName,
			},
			Spec: kudov1alpha1.EscalationPolicySpec{
				Subjects: []rbacv1.Subject{
					{
						Kind: rbacv1.UserKind,
						Name: userA.userName,
					},
				},
				Target: kudov1alpha1.EscalationTargetSpec{
					Duration: metav1.Duration{Duration: 5 * time.Second},
					Grants: []kudov1alpha1.EscalationGrant{
						{
							Kind:      granter.K8sRoleBindingGranterKind,
							Namespace: namespace.Name,
							RoleRef: rbacv1.RoleRef{
								Kind: "Role",
								Name: role.Name,
							},
						},
					},
				},
			},
		}
		escalation = &kudov1alpha1.Escalation{
			ObjectMeta: metav1.ObjectMeta{
				Name: "escalation-" + k8sTestName(t),
			},
			Spec: kudov1alpha1.EscalationSpec{
				PolicyName: policyName,
				Reason:     "Needs moar powerrrr",
			},
		}

		err error
	)

	_, err = admin.k8s.CoreV1().Namespaces().Create(ctx, &namespace, metav1.CreateOptions{})
	require.NoError(t, err)

	_, err = admin.k8s.RbacV1().Roles(namespace.Name).Create(
		ctx,
		&role,
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	_, err = admin.kudo.K8sV1alpha1().EscalationPolicies().Create(ctx, policy, metav1.CreateOptions{})
	require.NoError(t, err)

	escalation, err = userA.kudo.K8sV1alpha1().Escalations().Create(ctx, escalation, metav1.CreateOptions{})
	require.NoError(t, err)

	// admin waits for escalation to reach state ACCEPTED, and grants are created
	rawEsc := assertObjectUpdated(
		t,
		admin.kudo.K8sV1alpha1().RESTClient(),
		resourceNameNamespace{
			resource: "escalations",
			name:     escalation.Name,
			global:   true,
		},
		condEscalationStatusMatchesSpec(
			escalationWaitCondSpec{
				state:         kudov1alpha1.StateAccepted,
				grantStatuses: []kudov1alpha1.GrantStatus{kudov1alpha1.GrantStatusCreated},
			},
		),
		30*time.Second,
	)

	esc := as[*kudov1alpha1.Escalation](t, rawEsc)
	require.Len(t, esc.Status.GrantRefs, 1)

	assertObjectCreated(
		t,
		admin.k8s.RbacV1().RESTClient(),
		resourceNameNamespace{
			resource:  "rolebindings",
			name:      esc.Status.GrantRefs[0].Name,
			namespace: esc.Status.GrantRefs[0].Namespace,
		},
		30*time.Second,
	)

	// Now user should be allowed to be get the pods in the test namespace.
	_, err = userA.k8s.CoreV1().Pods(namespace.Name).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	// At some point the escalation should expire.
	assertObjectUpdated(
		t,
		admin.kudo.K8sV1alpha1().RESTClient(),
		resourceNameNamespace{
			resource: "escalations",
			name:     escalation.Name,
			global:   true,
		},
		condEscalationStatusMatchesSpec(
			escalationWaitCondSpec{
				state: kudov1alpha1.StateExpired,
				grantStatuses: []kudov1alpha1.GrantStatus{
					kudov1alpha1.GrantStatusReclaimed,
				},
			},
		),
		30*time.Second,
	)

	assertObjectDeleted(
		t,
		admin.k8s.RbacV1().RESTClient(),
		resourceNameNamespace{
			resource:  "rolebindings",
			name:      esc.Status.GrantRefs[0].Name,
			namespace: esc.Status.GrantRefs[0].Namespace,
		},
		30*time.Second,
	)

	// And our beloved user should get his ass kicked when listing pods again.
	_, err = userA.k8s.CoreV1().Pods(namespace.Name).List(ctx, metav1.ListOptions{})
	require.Error(t, err)
}
