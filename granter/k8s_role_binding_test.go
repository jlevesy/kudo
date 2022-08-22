package granter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/jlevesy/kudo/granter"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
)

var (
	testEscalation = kudov1alpha1.Escalation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-escalation",
		},
		Spec: kudov1alpha1.EscalationSpec{
			Requestor:  "jean-testor",
			PolicyName: "rule-the-world",
		},
		Status: kudov1alpha1.EscalationStatus{
			State: kudov1alpha1.StateAccepted,
		},
	}

	testEscalationAlreadyExistingBinding = kudov1alpha1.Escalation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-escalation",
		},
		Spec: kudov1alpha1.EscalationSpec{
			Requestor:  "jean-testor",
			PolicyName: "rule-the-world",
		},
		Status: kudov1alpha1.EscalationStatus{
			State: kudov1alpha1.StateAccepted,
			GrantRefs: []kudov1alpha1.EscalationGrantRef{
				{
					Kind:            granter.K8sRoleBindingGranterKind,
					Name:            "",
					Namespace:       "test-ns",
					Status:          kudov1alpha1.GrantStatusCreated,
					UID:             types.UID("aaaaa"),
					ResourceVersion: "340",
				},
			},
		},
	}

	testEscalationAlreadyExistingBindingTampered = kudov1alpha1.Escalation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-escalation",
		},
		Spec: kudov1alpha1.EscalationSpec{
			Requestor:  "jean-testor",
			PolicyName: "rule-the-world",
		},
		Status: kudov1alpha1.EscalationStatus{
			State: kudov1alpha1.StateAccepted,
			GrantRefs: []kudov1alpha1.EscalationGrantRef{
				{
					Kind:      granter.K8sRoleBindingGranterKind,
					Name:      "",
					Namespace: "test-ns",
					Status:    kudov1alpha1.GrantStatusCreated,
					UID:       types.UID("aaaaa"),
					// A change has been made. resource is version 340
					ResourceVersion: "339",
				},
			},
		},
	}

	testGrant = kudov1alpha1.EscalationGrant{
		Kind:      granter.K8sRoleBindingGranterKind,
		Namespace: "test-ns",
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "test-role",
		},
	}

	otherBinding = rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-binding",
			Namespace: "test-ns",
			Labels: map[string]string{
				"app.kubernetes.io/created-by": "kudo",
			},
			OwnerReferences: []metav1.OwnerReference{
				testEscalation.AsOwnerRef(),
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.UserKind,
				Name: "jean-testor",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "another-test-role",
		},
	}

	existingBindingNoUID = rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kudo-grant-",
			Namespace:    "test-ns",
			Labels: map[string]string{
				"app.kubernetes.io/created-by": "kudo",
			},
			OwnerReferences: []metav1.OwnerReference{
				testEscalation.AsOwnerRef(),
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.UserKind,
				Name: "jean-testor",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "test-role",
		},
	}

	existingBinding = rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kudo-grant-",
			Namespace:    "test-ns",
			Labels: map[string]string{
				"app.kubernetes.io/created-by": "kudo",
			},
			OwnerReferences: []metav1.OwnerReference{
				testEscalation.AsOwnerRef(),
			},
			UID:             types.UID("aaaaa"),
			ResourceVersion: "340",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.UserKind,
				Name: "jean-testor",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "test-role",
		},
	}
)

func TestK8sRoleBindingGranter_Create(t *testing.T) {
	testCases := []struct {
		desc string

		seed []runtime.Object

		escalation kudov1alpha1.Escalation
		grant      kudov1alpha1.EscalationGrant

		wantRef         kudov1alpha1.EscalationGrantRef
		wantCreateError error
		wantBindings    rbacv1.RoleBindingList
	}{
		{
			desc:       "creates a new role binding when none exists",
			seed:       []runtime.Object{&otherBinding},
			escalation: testEscalation,
			grant:      testGrant,
			wantRef: kudov1alpha1.EscalationGrantRef{
				Kind:      granter.K8sRoleBindingGranterKind,
				Name:      "", // testclient does not handle generate name.
				Namespace: "test-ns",
				Status:    kudov1alpha1.GrantStatusCreated,
			},
			wantBindings: rbacv1.RoleBindingList{
				Items: []rbacv1.RoleBinding{existingBindingNoUID, otherBinding},
			},
		},
		{
			desc:       "resuses existing binding",
			seed:       []runtime.Object{&existingBinding, &otherBinding},
			escalation: testEscalationAlreadyExistingBinding,
			grant:      testGrant,
			wantRef: kudov1alpha1.EscalationGrantRef{
				Kind:            granter.K8sRoleBindingGranterKind,
				Name:            "", // testclient does not handle generate name.
				Namespace:       "test-ns",
				Status:          kudov1alpha1.GrantStatusCreated,
				UID:             types.UID("aaaaa"),
				ResourceVersion: "340",
			},
			wantBindings: rbacv1.RoleBindingList{
				Items: []rbacv1.RoleBinding{existingBinding, otherBinding},
			},
		},
		{
			desc:            "detects if bindings has been tampered with",
			seed:            []runtime.Object{&existingBinding, &otherBinding},
			escalation:      testEscalationAlreadyExistingBindingTampered,
			grant:           testGrant,
			wantRef:         kudov1alpha1.EscalationGrantRef{},
			wantCreateError: granter.ErrTampered,
			wantBindings: rbacv1.RoleBindingList{
				Items: []rbacv1.RoleBinding{existingBinding, otherBinding},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				ctx                  = context.Background()
				factory, k8s, cancel = buildTestFactory(t, testCase.seed)
			)

			defer cancel()

			granter, err := factory.Get(granter.K8sRoleBindingGranterKind)
			require.NoError(t, err)

			gotRef, err := granter.Create(ctx, &testCase.escalation, testCase.grant)
			require.ErrorIs(t, err, testCase.wantCreateError)

			assert.Equal(t, testCase.wantRef, gotRef)

			gotBindings, err := k8s.
				kubeClientSet.
				RbacV1().
				RoleBindings(testCase.grant.Namespace).
				List(ctx, metav1.ListOptions{})

			require.NoError(t, err)
			assert.Equal(t, &testCase.wantBindings, gotBindings)
		})
	}
}

func TestK8sRoleBindingGranter_Reclaim(t *testing.T) {
	testCases := []struct {
		desc string

		seed []runtime.Object

		grantRef kudov1alpha1.EscalationGrantRef

		wantRef      kudov1alpha1.EscalationGrantRef
		wantBindings rbacv1.RoleBindingList
	}{
		{
			desc: "deletes the role binding if it exists",
			seed: []runtime.Object{&existingBinding, &otherBinding},
			grantRef: kudov1alpha1.EscalationGrantRef{
				Kind:      granter.K8sRoleBindingGranterKind,
				Name:      "",
				Namespace: "test-ns",
			},
			wantRef: kudov1alpha1.EscalationGrantRef{
				Kind:      granter.K8sRoleBindingGranterKind,
				Name:      "", // testclient does not handle generate name.
				Namespace: "test-ns",
				Status:    kudov1alpha1.GrantStatusReclaimed,
			},
			wantBindings: rbacv1.RoleBindingList{
				Items: []rbacv1.RoleBinding{otherBinding},
			},
		},
		{
			desc: "does not delete the role binding if it does not exists",
			seed: []runtime.Object{&otherBinding},
			grantRef: kudov1alpha1.EscalationGrantRef{
				Kind:      granter.K8sRoleBindingGranterKind,
				Name:      "",
				Namespace: "test-ns",
			},
			wantRef: kudov1alpha1.EscalationGrantRef{
				Kind:      granter.K8sRoleBindingGranterKind,
				Name:      "", // testclient does not handle generate name.
				Namespace: "test-ns",
				Status:    kudov1alpha1.GrantStatusReclaimed,
			},
			wantBindings: rbacv1.RoleBindingList{
				Items: []rbacv1.RoleBinding{otherBinding},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				ctx                  = context.Background()
				factory, k8s, cancel = buildTestFactory(t, testCase.seed)
			)

			defer cancel()

			granter, err := factory.Get(granter.K8sRoleBindingGranterKind)
			require.NoError(t, err)

			gotRef, err := granter.Reclaim(ctx, testCase.grantRef)
			require.NoError(t, err)

			assert.Equal(t, testCase.wantRef, gotRef)

			gotBindings, err := k8s.
				kubeClientSet.
				RbacV1().
				RoleBindings(testCase.grantRef.Namespace).
				List(ctx, metav1.ListOptions{})

			require.NoError(t, err)
			assert.Equal(t, &testCase.wantBindings, gotBindings)
		})
	}
}

type fakeK8s struct {
	kubeClientSet        kubernetes.Interface
	kubeInformersFactory kubeinformers.SharedInformerFactory
}

func buildTestFactory(t *testing.T, kubeSeed []runtime.Object) (granter.Factory, fakeK8s, func()) {
	t.Helper()

	var (
		kubeClientSet = kubefake.NewSimpleClientset(kubeSeed...)
		k8s           = fakeK8s{
			kubeClientSet: kubeClientSet,
			kubeInformersFactory: kubeinformers.NewSharedInformerFactory(
				kubeClientSet,
				60*time.Second,
			),
		}
		granterFactory = granter.DefaultGranterFactory(
			k8s.kubeInformersFactory,
			k8s.kubeClientSet,
		)
		done = make(chan struct{})
	)

	k8s.kubeInformersFactory.Start(done)

	err := controllersupport.CheckInformerSync(
		k8s.kubeInformersFactory.WaitForCacheSync(done),
	)
	require.NoError(t, err)

	return granterFactory, k8s, func() { close(done) }
}
