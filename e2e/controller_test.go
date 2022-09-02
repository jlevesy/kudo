package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	eventsv1 "k8s.io/api/events/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/grant"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

// This test makes sure that audit events are properly recorded for an escalation.
func TestEscalation_Controller_RecordsEscalationEvent(t *testing.T) {
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
			withExpiration(5*time.Second),
			withGrants(
				kudov1alpha1.EscalationGrant{
					Kind:              grant.K8sRoleBindingKind,
					AllowedNamespaces: []string{namespace.Name},
					RoleRef: rbacv1.RoleRef{
						Kind: "Role",
						Name: role.Name,
					},
				},
			),
		)

		escalation = generateEscalation(t, policy.Name, withNamespace(namespace.Name))

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

	// Now list all event regarding this escalation and assert on them.
	events, err := admin.k8s.EventsV1().Events("").List(
		ctx,
		metav1.ListOptions{
			FieldSelector: "regarding.name=" + gotEsc.Name,
		},
	)
	require.NoError(t, err)

	wantEvents := []eventsv1.Event{
		{
			Reason: "Create",
			Note:   "Escalation has been created",
		},
		{
			Reason: "Update",
			Note:   "New state PENDING, reason is: This escalation is being processed",
		},
		{
			Reason: "Update",
			Note:   "New state ACCEPTED, reason is: This escalation has been accepted, permissions are going to be granted in a few moments",
		},
		{
			Reason: "Update",
			Note:   "New state ACCEPTED, reason is: This escalation has been accepted, permissions are granted",
		},
		{
			Reason: "Update",
			Note:   "New state EXPIRED, reason is: This escalation has expired, all granted permissions are reclaimed",
		},
	}

	for i, evt := range events.Items {
		assert.Equal(t, wantEvents[i].Reason, evt.Reason)
		assert.Equal(t, wantEvents[i].Note, evt.Note)
	}

}

// This test makes sure that kudo denies an active escalation whose policy has changed after the escalation
// has been created.
func TestEscalation_Controller_DenyEscalationIfPolicyChanges(t *testing.T) {
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
			withExpiration(60*time.Minute), // Should not expire.
			withGrants(
				kudov1alpha1.EscalationGrant{
					Kind:              grant.K8sRoleBindingKind,
					AllowedNamespaces: []string{namespace.Name},
					RoleRef: rbacv1.RoleRef{
						Kind: "Role",
						Name: role.Name,
					},
				},
			),
		)

		escalation = generateEscalation(t, policy.Name, withNamespace(namespace.Name))

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

	gotPolicy, err := admin.kudo.K8sV1alpha1().EscalationPolicies().Create(ctx, &policy, metav1.CreateOptions{})
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
				},
			},
		),
		30*time.Second,
	)

	gotEsc := as[*kudov1alpha1.Escalation](t, rawEsc)

	// Now admin mutates the policy.
	gotPolicy.Spec.Target.Grants = append(
		policy.Spec.Target.Grants,
		kudov1alpha1.EscalationGrant{
			Kind:             grant.K8sRoleBindingKind,
			DefaultNamespace: "foo",
			RoleRef: rbacv1.RoleRef{
				Kind: "ClusterRole",
				Name: "some-other-role",
			},
		},
	)

	_, err = admin.kudo.K8sV1alpha1().EscalationPolicies().Update(ctx, gotPolicy, metav1.UpdateOptions{})
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
