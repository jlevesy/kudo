package escalation_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"github.com/jlevesy/kudo/audit"
	"github.com/jlevesy/kudo/escalation"
	"github.com/jlevesy/kudo/grant"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
	kudoclientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	kudofake "github.com/jlevesy/kudo/pkg/generated/clientset/versioned/fake"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
)

const testGrantKind = "TestGrantKind"

var (
	expiredCreationTimestamp = metav1.Time{
		Time: time.Date(2020, time.October, 3, 10, 20, 30, 0, time.UTC),
	}

	testPolicy = kudov1alpha1.EscalationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-policy",
			UID:             "eeee-eeee-eee",
			ResourceVersion: "43333",
		},
		Spec: kudov1alpha1.EscalationPolicySpec{
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.UserKind,
					Name: "jean-testeur",
				},
			},
			Challenges: []kudov1alpha1.EscalationChallenge{},
			Target: kudov1alpha1.EscalationTarget{
				DefaultDuration: metav1.Duration{Duration: time.Hour},
				Grants: []kudov1alpha1.ValueWithKind{
					kudov1alpha1.MustEncodeValueWithKind(
						testGrantKind,
						kudov1alpha1.K8sRoleBindingGrant{
							DefaultNamespace: "test-ns-1",
							RoleRef: rbacv1.RoleRef{
								APIGroup: rbacv1.GroupName,
								Kind:     "ClusterRoleBinding",
								Name:     "woopy-woop",
							},
						},
					),
					kudov1alpha1.MustEncodeValueWithKind(
						testGrantKind,
						kudov1alpha1.K8sRoleBindingGrant{
							DefaultNamespace: "test-ns-2",
							RoleRef: rbacv1.RoleRef{
								APIGroup: rbacv1.GroupName,
								Kind:     "ClusterRoleBinding",
								Name:     "woopy-wap",
							},
						},
					),
				},
			},
		},
	}

	creationTimestamp = time.Date(2022, time.October, 10, 1, 23, 1, 0, time.UTC)
	now               = time.Date(2022, time.October, 10, 1, 30, 1, 0, time.UTC)

	resyncDelay = 30 * time.Second
	retryDelay  = 10 * time.Second
)

func TestEscalationController_OnCreate(t *testing.T) {
	testCases := []struct {
		desc     string
		kudoSeed []runtime.Object

		createdEscalation    kudov1alpha1.Escalation
		wantEscalationStatus kudov1alpha1.EscalationStatus
	}{
		{
			desc: "denies escalation if policy does not exists",
			createdEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
					Requestor:  "john-claude",
					Reason:     "blah blah",
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: escalation.DeniedPolicyNotFoundStateDetails,
			},
		},
		{
			desc: "denies escalation if escalation spec isn't complete",
			createdEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
					Requestor:  "john-claude",
					Reason:     "         ",
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: escalation.DeniedBadEscalationSpecDetails,
			},
		},
		{
			desc:     "captures policy uid and version and sets transitions to pending",
			kudoSeed: []runtime.Object{&testPolicy},
			createdEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
					Requestor:  "john-claude",
					Reason:     "blah blah",
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StatePending,
				StateDetails:  escalation.PendingStateDetails,
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				ctx                   = context.Background()
				controller, k8s, done = buildController(
					t,
					grant.StaticFactory{},
					append(testCase.kudoSeed, &testCase.createdEscalation),
				)
			)

			defer done()

			_, err := controller.OnAdd(ctx, &testCase.createdEscalation)
			require.NoError(t, err)

			gotEscalation, err := k8s.kudoClientSet.K8sV1alpha1().Escalations().Get(
				ctx,
				testCase.createdEscalation.Name,
				metav1.GetOptions{},
			)
			require.NoError(t, err)

			assert.Equal(t, testCase.wantEscalationStatus, gotEscalation.Status)
		})
	}
}

func TestEscalationController_OnUpdate(t *testing.T) {
	testCases := []struct {
		desc string

		kudoSeed        []runtime.Object
		upsertGrantErr  error
		reclaimGrantErr error

		updatedEscalation kudov1alpha1.Escalation

		wantError            error
		wantNextResync       time.Duration
		wantEscalationStatus kudov1alpha1.EscalationStatus
	}{
		{
			desc: "raises an error on unknown state",
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.EscalationState("YOLO"),
				},
			},
			wantError: errors.New(`unsupported status "YOLO", ignoring event`),
		},
		{
			desc: "on pending state, transitions to denied if policy doesn't exist",
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
				},
				Status: kudov1alpha1.EscalationStatus{
					State:     kudov1alpha1.StatePending,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: escalation.DeniedPolicyNotFoundStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
			},
		},
		{
			desc:     "on pending state, transitions to denied if policy has changed since creation",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StatePending,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID,
					PolicyVersion: "3030303",
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateDenied,
				StateDetails:  escalation.DeniedPolicyChangedStateDetails,
				GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
				PolicyUID:     testPolicy.UID,
				PolicyVersion: "3030303",
			},
		},
		{
			desc:     "on pending state, transitions to denied if policy has been recreated since creation",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StatePending,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     "aaa-aaa-aaa",
					PolicyVersion: testPolicy.ResourceVersion,
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateDenied,
				StateDetails:  escalation.DeniedPolicyChangedStateDetails,
				GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
				PolicyUID:     "aaa-aaa-aaa",
				PolicyVersion: testPolicy.ResourceVersion,
			},
		},
		{
			desc:     "on pending state, transitions to accepted if all good",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: creationTimestamp,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StatePending,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID,
					PolicyVersion: testPolicy.ResourceVersion,
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateAccepted,
				StateDetails:  escalation.AcceptedInProgressStateDetails,
				GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion,
				ExpiresAt: metav1.Time{
					Time: now.Add(
						testPolicy.Spec.Target.DefaultDuration.Duration,
					),
				},
			},
		},
		{
			desc:     "on pending state, sets to updated according to escalation duration",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: creationTimestamp,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
					Duration:   metav1.Duration{Duration: 2 * time.Second},
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StatePending,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID,
					PolicyVersion: testPolicy.ResourceVersion,
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateAccepted,
				StateDetails:  escalation.AcceptedInProgressStateDetails,
				GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion,
				ExpiresAt: metav1.Time{
					Time: now.Add(2 * time.Second),
				},
			},
		},
		{
			desc:     "on accepted state, transitions to denied if referenced policy doesn't exists",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: "nope nope",
				},
				Status: kudov1alpha1.EscalationStatus{
					State:     kudov1alpha1.StateAccepted,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
					ExpiresAt: metav1.Time{
						Time: now.Add(50 * time.Second),
					},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: escalation.DeniedPolicyNotFoundStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
				ExpiresAt: metav1.Time{
					Time: now.Add(50 * time.Second),
				},
			},
		},
		{
			desc:     "on accepted state, transitions to expired if now is after expires at",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-escalation",
					CreationTimestamp: expiredCreationTimestamp,
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:     kudov1alpha1.StateAccepted,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
					ExpiresAt: metav1.Time{
						Time: now.Add(-50 * time.Second),
					},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateExpired,
				StateDetails: escalation.ExpiredStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
				ExpiresAt: metav1.Time{
					Time: now.Add(-50 * time.Second),
				},
			},
		},
		{
			desc:     "on accepted state, transition to denied if policy has changed",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StateAccepted,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID,
					PolicyVersion: testPolicy.ResourceVersion + "4444",
					ExpiresAt: metav1.Time{
						Time: now.Add(50 * time.Second),
					},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateDenied,
				StateDetails:  escalation.DeniedPolicyChangedStateDetails,
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion + "4444",
				GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
				ExpiresAt: metav1.Time{
					Time: now.Add(50 * time.Second),
				},
			},
		},
		{
			desc:     "on accepted state, transition to denied if policy has been replaced",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StateAccepted,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID + "33333",
					PolicyVersion: testPolicy.ResourceVersion,
					ExpiresAt: metav1.Time{
						Time: now.Add(50 * time.Second),
					},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateDenied,
				StateDetails:  escalation.DeniedPolicyChangedStateDetails,
				PolicyUID:     testPolicy.UID + "33333",
				PolicyVersion: testPolicy.ResourceVersion,
				GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
				ExpiresAt: metav1.Time{
					Time: now.Add(50 * time.Second),
				},
			},
		},
		{
			desc:     "on accepted state, provision grants if all is good",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StateAccepted,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID,
					PolicyVersion: testPolicy.ResourceVersion,
					ExpiresAt: metav1.Time{
						Time: now.Add(50 * time.Second),
					},
				},
			},
			wantNextResync: resyncDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateAccepted,
				StateDetails:  escalation.AcceptedAppliedStateDetails,
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion,
				ExpiresAt: metav1.Time{
					Time: now.Add(50 * time.Second),
				},
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-2",
							},
						),
					},
				},
			},
		},
		{
			desc:     "on accepted state, schedules next retry to expration date",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StateAccepted,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID,
					PolicyVersion: testPolicy.ResourceVersion,
					ExpiresAt: metav1.Time{
						Time: now.Add(10 * time.Second),
					},
				},
			},
			wantNextResync: 10 * time.Second,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateAccepted,
				StateDetails:  escalation.AcceptedAppliedStateDetails,
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion,
				ExpiresAt: metav1.Time{
					Time: now.Add(10 * time.Second),
				},
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-2",
							},
						),
					},
				},
			},
		},
		{
			desc:           "on accepted state, marks as partially granted if any the granter fails",
			kudoSeed:       []runtime.Object{&testPolicy},
			upsertGrantErr: errors.New("not today!"),
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:         kudov1alpha1.StateAccepted,
					GrantRefs:     []kudov1alpha1.EscalationGrantRef{{}},
					PolicyUID:     testPolicy.UID,
					PolicyVersion: testPolicy.ResourceVersion,
					ExpiresAt: metav1.Time{
						Time: now.Add(50 * time.Second),
					},
				},
			},
			wantNextResync: resyncDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:         kudov1alpha1.StateAccepted,
				StateDetails:  "Escalation is partially active, reason is: not today!",
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion,
				ExpiresAt: metav1.Time{
					Time: now.Add(50 * time.Second),
				},
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-2",
							},
						),
					},
				},
			},
		},
		{
			desc:           "on accepted state, deny escalation if the granter reports that the resource has been tampered with",
			kudoSeed:       []runtime.Object{&testPolicy},
			upsertGrantErr: grant.ErrTampered,
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					ExpiresAt: metav1.Time{
						Time: now.Add(50 * time.Second),
					},
					State:         kudov1alpha1.StateAccepted,
					PolicyUID:     testPolicy.UID,
					PolicyVersion: testPolicy.ResourceVersion,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-1",
								},
							),
						},
					},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				ExpiresAt: metav1.Time{
					Time: now.Add(50 * time.Second),
				},
				State:         kudov1alpha1.StateDenied,
				StateDetails:  "Escalation has been denied, reason is: kudo managed resource has been tampered with",
				PolicyUID:     testPolicy.UID,
				PolicyVersion: testPolicy.ResourceVersion,
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
				},
			},
		},
		{
			desc:     "on expired state, reclaims all the known grants",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:        kudov1alpha1.StateExpired,
					StateDetails: "expiration has expired",
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-1",
								},
							),
						},
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-2",
								},
							),
						},
					},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateExpired,
				StateDetails: "expiration has expired",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusReclaimed,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
					{
						Status: kudov1alpha1.GrantStatusReclaimed,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-2",
							},
						),
					},
				},
			},
		},
		{
			desc:            "on expired state, marks permission as partially reclaimed if one of the granter fails",
			kudoSeed:        []runtime.Object{&testPolicy},
			reclaimGrantErr: errors.New("nonono"),
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.StateExpired,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-1",
								},
							),
						},
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-2",
								},
							),
						},
					},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateExpired,
				StateDetails: "This escalation has expired, but grants have been partially reclaimed. Reason is: nonono",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-2",
							},
						),
					},
				},
			},
		},
		{
			desc:     "on denied state, reclaims all the known grants",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:        kudov1alpha1.StateDenied,
					StateDetails: "denied for some reason",
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-1",
								},
							),
						},
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-2",
								},
							),
						},
					},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: "denied for some reason",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusReclaimed,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
					{
						Status: kudov1alpha1.GrantStatusReclaimed,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-2",
							},
						),
					},
				},
			},
		},
		{
			desc:            "on denied state, marks permission as partially reclaimed if one of the granter fails",
			kudoSeed:        []runtime.Object{&testPolicy},
			reclaimGrantErr: errors.New("nonono"),
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: now,
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.StateDenied,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-1",
								},
							),
						},
						{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-test-ns-2",
								},
							),
						},
					},
				},
			},
			wantNextResync: retryDelay,
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: "This escalation is denied, but grants have been partially reclaimed. Reason is: nonono",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-1",
							},
						),
					},
					{
						Status: kudov1alpha1.GrantStatusCreated,
						Ref: kudov1alpha1.MustEncodeValueWithKind(
							testGrantKind,
							kudov1alpha1.K8sRoleBindingGrantRef{
								Name: "grant-test-ns-2",
							},
						),
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				ctx          = context.Background()
				dummyGranter = mockGranter{
					CreateFn: func(_ *kudov1alpha1.Escalation, grant kudov1alpha1.ValueWithKind) (kudov1alpha1.EscalationGrantRef, error) {
						k8sGrant, err := kudov1alpha1.DecodeValueWithKind[kudov1alpha1.K8sRoleBindingGrant](grant)
						require.NoError(t, err)

						return kudov1alpha1.EscalationGrantRef{
							Status: kudov1alpha1.GrantStatusCreated,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								testGrantKind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: "grant-" + k8sGrant.DefaultNamespace,
								},
							),
						}, testCase.upsertGrantErr
					},
					ReclaimFn: func(ref kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error) {
						k8sGrantRef, err := kudov1alpha1.DecodeValueWithKind[kudov1alpha1.K8sRoleBindingGrantRef](ref.Ref)
						require.NoError(t, err)

						return kudov1alpha1.EscalationGrantRef{
							Status: kudov1alpha1.GrantStatusReclaimed,
							Ref: kudov1alpha1.MustEncodeValueWithKind(
								ref.Ref.Kind,
								kudov1alpha1.K8sRoleBindingGrantRef{
									Name: k8sGrantRef.Name,
								},
							),
						}, testCase.reclaimGrantErr
					},
				}

				controller, k8s, done = buildController(
					t,
					grant.StaticFactory{
						testGrantKind: injectMockGranter(&dummyGranter),
					},
					append(
						testCase.kudoSeed,
						&testCase.updatedEscalation,
					),
				)
			)

			defer done()

			gotInsight, err := controller.OnUpdate(ctx, nil, &testCase.updatedEscalation)
			if testCase.wantError != nil {
				require.Equal(t, testCase.wantError, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, testCase.wantNextResync, gotInsight.ResyncAfter)

			gotEscalation, err := k8s.kudoClientSet.K8sV1alpha1().Escalations().Get(
				ctx,
				testCase.updatedEscalation.Name,
				metav1.GetOptions{},
			)
			require.NoError(t, err)

			sort.Slice(gotEscalation.Status.GrantRefs, func(i, j int) bool {
				vi, err := kudov1alpha1.DecodeValueWithKind[kudov1alpha1.K8sRoleBindingGrantRef](gotEscalation.Status.GrantRefs[i].Ref)
				require.NoError(t, err)
				vj, err := kudov1alpha1.DecodeValueWithKind[kudov1alpha1.K8sRoleBindingGrantRef](gotEscalation.Status.GrantRefs[j].Ref)
				require.NoError(t, err)

				return vi.Name < vj.Name
			})

			assert.Equal(t, testCase.wantEscalationStatus, gotEscalation.Status)
		})
	}
}

func injectMockGranter(g *mockGranter) func() (grant.Granter, error) {
	return func() (grant.Granter, error) { return g, nil }
}

type mockGranter struct {
	CreateFn   func(*kudov1alpha1.Escalation, kudov1alpha1.ValueWithKind) (kudov1alpha1.EscalationGrantRef, error)
	ReclaimFn  func(kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error)
	ValidateFn func(*kudov1alpha1.Escalation, kudov1alpha1.ValueWithKind) error
}

func (g *mockGranter) Create(_ context.Context, esc *kudov1alpha1.Escalation, grant kudov1alpha1.ValueWithKind) (kudov1alpha1.EscalationGrantRef, error) {
	return g.CreateFn(esc, grant)
}

func (g *mockGranter) Reclaim(_ context.Context, grantRef kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error) {
	return g.ReclaimFn(grantRef)
}

func (g *mockGranter) Validate(_ context.Context, esc *kudov1alpha1.Escalation, grant kudov1alpha1.ValueWithKind) error {
	return g.ValidateFn(esc, grant)
}

type fakeK8s struct {
	kudoClientSet        kudoclientset.Interface
	kudoInformersFactory kudoinformers.SharedInformerFactory
}

type doneFunc func()

func buildController(t *testing.T, granterFactory grant.Factory, kudoSeed []runtime.Object) (*escalation.Controller, fakeK8s, doneFunc) {
	t.Helper()

	var (
		kudoClientSet = kudofake.NewSimpleClientset(kudoSeed...)
		k8s           = fakeK8s{
			kudoClientSet: kudoClientSet,
			kudoInformersFactory: kudoinformers.NewSharedInformerFactory(
				kudoClientSet,
				60*time.Second,
			),
		}
		controller = escalation.NewController(
			k8s.kudoInformersFactory.K8s().V1alpha1().EscalationPolicies().Lister(),
			k8s.kudoClientSet.K8sV1alpha1().Escalations(),
			granterFactory,
			audit.NewK8sEventSink(&record.FakeRecorder{}),
			escalation.WithNowFunc(nowFunc),
			escalation.WithResyncInterval(resyncDelay),
			escalation.WithRetryInterval(retryDelay),
		)
		done = make(chan struct{})
	)

	k8s.kudoInformersFactory.Start(done)

	err := controllersupport.CheckInformerSync(
		k8s.kudoInformersFactory.WaitForCacheSync(done),
	)
	require.NoError(t, err)

	return controller, k8s, func() { close(done) }
}

func nowFunc() time.Time { return now }
