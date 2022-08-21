package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/granter"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

// TestEscalation_RoleBinding makes sure that kudo can provision two role bindings granting two different roles to
// user-A into two different namespaces.
// We first provision the namespaces and the roles,
// then trigger an escalation, wait for bindings to be provisioned, and make sure we read accessed resources.
// We then wait for expiration, make sure that  the bindings are correctly destroyed, then make sure that user-A can't access resources.
func TestEscalation_RoleBinding(t *testing.T) {
	t.Parallel()

	var (
		ctx        = context.Background()
		namespaces = generateNamespaces(t, 2)
		roles      = []rbacv1.Role{
			generateRole(t, 0, namespaces[0].Name, rbacv1.PolicyRule{
				Verbs:     []string{"list"},
				APIGroups: []string{""},
				Resources: []string{"pods"},
			}),
			generateRole(t, 1, namespaces[1].Name, rbacv1.PolicyRule{
				Verbs:     []string{"list"},
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
			}),
		}
		policy = generateEscalationPolicy(
			t,
			withGrants(
				kudov1alpha1.EscalationGrant{
					Kind:      granter.K8sRoleBindingGranterKind,
					Namespace: namespaces[0].Name,
					RoleRef: rbacv1.RoleRef{
						Kind: "Role",
						Name: roles[0].Name,
					},
				},
				kudov1alpha1.EscalationGrant{
					Kind:      granter.K8sRoleBindingGranterKind,
					Namespace: namespaces[1].Name,
					RoleRef: rbacv1.RoleRef{
						Kind: "Role",
						Name: roles[1].Name,
					},
				},
			),
		)

		escalation = generateEscalation(t, policy.Name)

		err error
	)

	for _, namespace := range namespaces {
		_, err = admin.k8s.CoreV1().Namespaces().Create(ctx, &namespace, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	for i, role := range roles {
		_, err = admin.k8s.RbacV1().Roles(namespaces[i].Name).Create(
			ctx,
			&role,
			metav1.CreateOptions{},
		)
		require.NoError(t, err)
	}

	_, err = admin.kudo.K8sV1alpha1().EscalationPolicies().Create(ctx, &policy, metav1.CreateOptions{})
	require.NoError(t, err)

	_, err = userA.kudo.K8sV1alpha1().Escalations().Create(ctx, &escalation, metav1.CreateOptions{})
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
				state: kudov1alpha1.StateAccepted,
				grantStatuses: []kudov1alpha1.GrantStatus{
					kudov1alpha1.GrantStatusCreated,
					kudov1alpha1.GrantStatusCreated,
				},
			},
		),
		30*time.Second,
	)

	gotEsc := as[*kudov1alpha1.Escalation](t, rawEsc)
	require.Len(t, gotEsc.Status.GrantRefs, 2)

	for _, ref := range gotEsc.Status.GrantRefs {
		assertObjectCreated(
			t,
			admin.k8s.RbacV1().RESTClient(),
			resourceNameNamespace{
				resource:  "rolebindings",
				name:      ref.Name,
				namespace: ref.Namespace,
			},
			30*time.Second,
		)
	}

	// Now user should be allowed to be get the pods in the first test namespace.
	_, err = userA.k8s.CoreV1().Pods(namespaces[0].Name).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	// Now user should be allowed to be get the pods in the first test namespace.
	_, err = userA.k8s.CoreV1().ConfigMaps(namespaces[1].Name).List(ctx, metav1.ListOptions{})
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
					kudov1alpha1.GrantStatusReclaimed,
				},
			},
		),
		30*time.Second,
	)

	// Bindings are reclaimed.
	for _, ref := range gotEsc.Status.GrantRefs {
		assertObjectDeleted(
			t,
			admin.k8s.RbacV1().RESTClient(),
			resourceNameNamespace{
				resource:  "rolebindings",
				name:      ref.Name,
				namespace: ref.Namespace,
			},
			30*time.Second,
		)
	}

	// And our beloved user should get his ass kicked when listing pods again in the first namespace.
	_, err = userA.k8s.CoreV1().Pods(namespaces[0].Name).List(ctx, metav1.ListOptions{})
	require.Error(t, err)

	// And configmaps in the second namespace.
	_, err = userA.k8s.CoreV1().ConfigMaps(namespaces[1].Name).List(ctx, metav1.ListOptions{})
	require.Error(t, err)
}
