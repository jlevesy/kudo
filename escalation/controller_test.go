package escalation_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/jlevesy/kudo/escalation"
	"github.com/jlevesy/kudo/granter"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
	kudoclientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	kudofake "github.com/jlevesy/kudo/pkg/generated/clientset/versioned/fake"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testGrantKind = "TestGrantKind"

var (
	expiredCreationTimestamp = metav1.Time{
		Time: time.Date(2020, time.October, 3, 10, 20, 30, 0, time.UTC),
	}

	testPolicy = kudov1alpha1.EscalationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-policy",
		},
		Spec: kudov1alpha1.EscalationPolicySpec{
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.UserKind,
					Name: "jean-testeur",
				},
			},
			Challenges: []kudov1alpha1.EscalationChallenge{},
			Target: kudov1alpha1.EscalationTargetSpec{
				Duration: metav1.Duration{Duration: time.Hour},
				Grants: []kudov1alpha1.EscalationGrant{
					{
						Kind:      testGrantKind,
						Namespace: "test-ns-1",
						RoleRef: rbacv1.RoleRef{
							APIGroup: rbacv1.GroupName,
							Kind:     "ClusterRoleBinding",
							Name:     "woopy-woop",
						},
					},
					{
						Kind:      testGrantKind,
						Namespace: "test-ns-2",
						RoleRef: rbacv1.RoleRef{
							APIGroup: rbacv1.GroupName,
							Kind:     "ClusterRoleBinding",
							Name:     "woopy-wap",
						},
					},
				},
			},
		},
	}
)

func TestEscalationController_OnCreate(t *testing.T) {
	testCases := []struct {
		desc string

		createdEscalation    kudov1alpha1.Escalation
		wantEscalationStatus kudov1alpha1.EscalationStatus
	}{
		{
			desc: "sets escalation state to pending",
			createdEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StatePending,
				StateDetails: escalation.PendingStateDetails,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				ctx                   = context.Background()
				controller, k8s, done = buildController(
					t,
					granter.StaticFactory{},
					[]runtime.Object{&testCase.createdEscalation},
				)
			)

			defer done()

			err := controller.OnAdd(ctx, &testCase.createdEscalation)
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
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: escalation.DeniedPolicyNotFoundStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
			},
		},
		{
			desc:     "on pending state, transitions to expired if escalation lifetime exceeds policy duration",
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
					State:     kudov1alpha1.StatePending,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateExpired,
				StateDetails: escalation.ExpiredWillReclaimStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
			},
		},
		{
			desc:     "on pending state, transitions to accepted if all good",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:     kudov1alpha1.StatePending,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateAccepted,
				StateDetails: escalation.AcceptedInProgressStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
			},
		},
		{
			desc:     "on accepted state, transitions to denied if referenced policy doesn't exists",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: "nope nope",
				},
				Status: kudov1alpha1.EscalationStatus{
					State:     kudov1alpha1.StateAccepted,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: escalation.DeniedPolicyNotFoundStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
			},
		},
		{
			desc:     "on accepted state, transitions to expired if escalation lifetime exceeds policy duration",
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
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateExpired,
				StateDetails: escalation.ExpiredWillReclaimStateDetails,
				GrantRefs:    []kudov1alpha1.EscalationGrantRef{{}},
			},
		},
		{
			desc:     "on accepted state, provision grants if all is good",
			kudoSeed: []runtime.Object{&testPolicy},
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:     kudov1alpha1.StateAccepted,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateAccepted,
				StateDetails: escalation.AcceptedAppliedStateDetails,
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-1",
						Status: kudov1alpha1.GrantStatusCreated,
					},
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-2",
						Status: kudov1alpha1.GrantStatusCreated,
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
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State:     kudov1alpha1.StateAccepted,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{{}},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateAccepted,
				StateDetails: "Escalation is partially active, reason is: not today!",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-1",
						Status: kudov1alpha1.GrantStatusCreated,
					},
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-2",
						Status: kudov1alpha1.GrantStatusCreated,
					},
				},
			},
		},
		{
			desc:           "on accepted state, deny escalation if the granter reports that the resource has been tampered with",
			kudoSeed:       []runtime.Object{&testPolicy},
			upsertGrantErr: granter.ErrTampered,
			updatedEscalation: kudov1alpha1.Escalation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-escalation",
					CreationTimestamp: metav1.Time{
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.StateAccepted,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-1",
							Status: kudov1alpha1.GrantStatusCreated,
						},
					},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: "Escalation has been denied, reason is: kudo managed resource has been tampered with",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-1",
						Status: kudov1alpha1.GrantStatusCreated,
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
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.StateExpired,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-1",
							Status: kudov1alpha1.GrantStatusCreated,
						},
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-2",
							Status: kudov1alpha1.GrantStatusCreated,
						},
					},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateExpired,
				StateDetails: escalation.ExpiredReclaimedStateDetails,
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-1",
						Status: kudov1alpha1.GrantStatusReclaimed,
					},
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-2",
						Status: kudov1alpha1.GrantStatusReclaimed,
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
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.StateExpired,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-1",
							Status: kudov1alpha1.GrantStatusCreated,
						},
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-2",
							Status: kudov1alpha1.GrantStatusCreated,
						},
					},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateExpired,
				StateDetails: "This escalation has expired, but grants have been partially reclaimed. Reason is: nonono",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-1",
						Status: kudov1alpha1.GrantStatusCreated,
					},
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-2",
						Status: kudov1alpha1.GrantStatusCreated,
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
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.StateDenied,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-1",
							Status: kudov1alpha1.GrantStatusCreated,
						},
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-2",
							Status: kudov1alpha1.GrantStatusCreated,
						},
					},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: escalation.DeniedReclaimedStateDetails,
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-1",
						Status: kudov1alpha1.GrantStatusReclaimed,
					},
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-2",
						Status: kudov1alpha1.GrantStatusReclaimed,
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
						Time: time.Now(),
					},
				},
				Spec: kudov1alpha1.EscalationSpec{
					PolicyName: testPolicy.Name,
				},
				Status: kudov1alpha1.EscalationStatus{
					State: kudov1alpha1.StateDenied,
					GrantRefs: []kudov1alpha1.EscalationGrantRef{
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-1",
							Status: kudov1alpha1.GrantStatusCreated,
						},
						{
							Kind:   testGrantKind,
							Name:   "grant-test-ns-2",
							Status: kudov1alpha1.GrantStatusCreated,
						},
					},
				},
			},
			wantEscalationStatus: kudov1alpha1.EscalationStatus{
				State:        kudov1alpha1.StateDenied,
				StateDetails: "This escalation is denied, but grants have been partially reclaimed. Reason is: nonono",
				GrantRefs: []kudov1alpha1.EscalationGrantRef{
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-1",
						Status: kudov1alpha1.GrantStatusCreated,
					},
					{
						Kind:   testGrantKind,
						Name:   "grant-test-ns-2",
						Status: kudov1alpha1.GrantStatusCreated,
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
					CreateFn: func(_ *kudov1alpha1.Escalation, grant kudov1alpha1.EscalationGrant) (kudov1alpha1.EscalationGrantRef, error) {
						return kudov1alpha1.EscalationGrantRef{
							Kind:   testGrantKind,
							Name:   "grant-" + grant.Namespace,
							Status: kudov1alpha1.GrantStatusCreated,
						}, testCase.upsertGrantErr
					},
					ReclaimFn: func(ref kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error) {
						return kudov1alpha1.EscalationGrantRef{
							Kind:   ref.Kind,
							Name:   ref.Name,
							Status: kudov1alpha1.GrantStatusReclaimed,
						}, testCase.reclaimGrantErr
					},
				}

				controller, k8s, done = buildController(
					t,
					granter.StaticFactory{
						testGrantKind: injectMockGranter(&dummyGranter),
					},
					append(
						testCase.kudoSeed,
						&testCase.updatedEscalation,
					),
				)
			)

			defer done()

			err := controller.OnUpdate(ctx, nil, &testCase.updatedEscalation)
			if testCase.wantError != nil {
				require.Equal(t, testCase.wantError, err)
				return
			}
			require.NoError(t, err)

			gotEscalation, err := k8s.kudoClientSet.K8sV1alpha1().Escalations().Get(
				ctx,
				testCase.updatedEscalation.Name,
				metav1.GetOptions{},
			)
			require.NoError(t, err)

			sort.Slice(gotEscalation.Status.GrantRefs, func(i, j int) bool {
				return gotEscalation.Status.GrantRefs[i].Name <
					gotEscalation.Status.GrantRefs[j].Name
			})

			assert.Equal(t, testCase.wantEscalationStatus, gotEscalation.Status)
		})
	}
}

func injectMockGranter(g *mockGranter) func() (granter.Granter, error) {
	return func() (granter.Granter, error) { return g, nil }
}

type mockGranter struct {
	CreateFn  func(*kudov1alpha1.Escalation, kudov1alpha1.EscalationGrant) (kudov1alpha1.EscalationGrantRef, error)
	ReclaimFn func(kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error)
}

func (g *mockGranter) Create(_ context.Context, esc *kudov1alpha1.Escalation, grant kudov1alpha1.EscalationGrant) (kudov1alpha1.EscalationGrantRef, error) {
	return g.CreateFn(esc, grant)
}

func (g *mockGranter) Reclaim(_ context.Context, grantRef kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error) {
	return g.ReclaimFn(grantRef)
}

type fakeK8s struct {
	kudoClientSet        kudoclientset.Interface
	kudoInformersFactory kudoinformers.SharedInformerFactory
}

type doneFunc func()

func buildController(t *testing.T, granterFactory granter.Factory, kudoSeed []runtime.Object) (*escalation.Controller, fakeK8s, doneFunc) {
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
