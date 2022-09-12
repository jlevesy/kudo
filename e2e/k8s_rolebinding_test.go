package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/grant"
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
				kudov1alpha1.MustEncodeValueWithKind(
					grant.K8sRoleBindingKind,
					kudov1alpha1.K8sRoleBindingGrant{
						DefaultNamespace: namespaces[0].Name,
						RoleRef: rbacv1.RoleRef{
							Kind: "Role",
							Name: roles[0].Name,
						},
					},
				),
				kudov1alpha1.MustEncodeValueWithKind(
					grant.K8sRoleBindingKind,
					kudov1alpha1.K8sRoleBindingGrant{
						DefaultNamespace: namespaces[1].Name,
						RoleRef: rbacv1.RoleRef{
							Kind: "Role",
							Name: roles[1].Name,
						},
					},
				),
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

	assertPolicyCreated(t, policy.Name)

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

	assertGrantedK8sResourcesCreated(t, *gotEsc, "rolebindings")

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
	assertGrantedK8sResourcesDeleted(t, *gotEsc, "rolebindings")

	// And our beloved user should get his ass kicked when listing pods again in the first namespace.
	_, err = userA.k8s.CoreV1().Pods(namespaces[0].Name).List(ctx, metav1.ListOptions{})
	require.Error(t, err)

	// And configmaps in the second namespace.
	_, err = userA.k8s.CoreV1().ConfigMaps(namespaces[1].Name).List(ctx, metav1.ListOptions{})
	require.Error(t, err)
}

// This test proves that kudo creates a role binding in the namespace asked by the user if the namespace is in the allowlist.
func TestEscalation_RoleBinding_UserAskedNamespace(t *testing.T) {
	t.Parallel()

	var (
		ctx       = context.Background()
		namespace = generateNamespace(t, 0)
		role      = generateRole(t, 0, namespace.Name, rbacv1.PolicyRule{
			Verbs:     []string{"list"},
			APIGroups: []string{""},
			Resources: []string{"pods"},
		})
		policy = generateEscalationPolicy(
			t,
			withGrants(
				kudov1alpha1.MustEncodeValueWithKind(
					grant.K8sRoleBindingKind,
					kudov1alpha1.K8sRoleBindingGrant{
						AllowedNamespaces: []string{
							namespace.Name,
						},
						RoleRef: rbacv1.RoleRef{
							Kind: "Role",
							Name: role.Name,
						},
					},
				),
			),
		)

		// This time the escalation is specifying a namespace.
		escalation    = generateEscalation(t, policy.Name, withNamespace(namespace.Name))
		badEscalation = generateEscalation(t, policy.Name, withNamespace("yo-all-the-los"))

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

	_, err = admin.kudo.K8sV1alpha1().EscalationPolicies().Create(ctx, &policy, metav1.CreateOptions{})
	require.NoError(t, err)

	assertPolicyCreated(t, policy.Name)

	// Webhook rejects an attempt on a non-authorized namespace.
	_, err = userA.kudo.K8sV1alpha1().Escalations().Create(ctx, &badEscalation, metav1.CreateOptions{})
	require.Error(t, err)

	_, err = userA.kudo.K8sV1alpha1().Escalations().Create(ctx, &escalation, metav1.CreateOptions{})
	require.NoError(t, err)

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
				},
			},
		),
		30*time.Second,
	)

	gotEsc := as[*kudov1alpha1.Escalation](t, rawEsc)

	assertGrantedK8sResourcesCreated(t, *gotEsc, "rolebindings")

	// Now user should be allowed to get the pods in the first test namespace.
	_, err = userA.k8s.CoreV1().Pods(namespace.Name).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	// After a while, escalation expires.
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

	// Bindings are reclaimed.
	assertGrantedK8sResourcesDeleted(t, *gotEsc, "rolebindings")

	// User can't access pods anymore.
	_, err = userA.k8s.CoreV1().Pods(namespace.Name).List(ctx, metav1.ListOptions{})
	require.Error(t, err)
}

// This test proves that kudo deny an escalation is one tries to tamper with a rolebinding created by kudo.
func TestEscalation_RoleBinding_DeniedIfTamperedWith(t *testing.T) {
	t.Parallel()

	var (
		ctx       = context.Background()
		namespace = generateNamespace(t, 0)
		role      = generateRole(t, 0, namespace.Name, rbacv1.PolicyRule{
			Verbs:     []string{"list"},
			APIGroups: []string{""},
			Resources: []string{"pods"},
		})
		policy = generateEscalationPolicy(
			t,
			withDefaultDuration(60*time.Minute), // Should not expire.
			withGrants(
				kudov1alpha1.MustEncodeValueWithKind(
					grant.K8sRoleBindingKind,
					kudov1alpha1.K8sRoleBindingGrant{
						DefaultNamespace: namespace.Name,
						RoleRef: rbacv1.RoleRef{
							Kind: "Role",
							Name: role.Name,
						},
					},
				),
			),
		)

		escalation = generateEscalation(t, policy.Name)

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

	_, err = admin.kudo.K8sV1alpha1().EscalationPolicies().Create(ctx, &policy, metav1.CreateOptions{})
	require.NoError(t, err)

	assertPolicyCreated(t, policy.Name)

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
				},
			},
		),
		30*time.Second,
	)

	gotEsc := as[*kudov1alpha1.Escalation](t, rawEsc)
	require.Len(t, gotEsc.Status.GrantRefs, 1)

	assertGrantedK8sResourcesCreated(t, *gotEsc, "rolebindings")

	ref := gotEsc.Status.GrantRefs[0]
	k8sRef, err := kudov1alpha1.DecodeValueWithKind[kudov1alpha1.K8sRoleBindingGrantRef](ref.Ref)
	require.NoError(t, err)

	// Fetch and patch the role binding to change the subject list.
	roleBinding, err := admin.k8s.RbacV1().RoleBindings(k8sRef.Namespace).Get(ctx, k8sRef.Name, metav1.GetOptions{})
	require.NoError(t, err)

	// Do something evil and try to reorient the granted permissions to all authenticated users.
	roleBinding.Subjects = []rbacv1.Subject{
		{
			Kind: rbacv1.GroupKind,
			Name: "system:authenticated",
		},
	}

	_, err = admin.k8s.RbacV1().RoleBindings(k8sRef.Namespace).Update(ctx, roleBinding, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Kudo should respond and deny the escalation.
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
				state: kudov1alpha1.StateDenied,
				grantStatuses: []kudov1alpha1.GrantStatus{
					kudov1alpha1.GrantStatusReclaimed,
				},
			},
		),
		30*time.Second,
	)

	// Bindings are reclaimed.
	assertGrantedK8sResourcesDeleted(t, *gotEsc, "rolebindings")
}
