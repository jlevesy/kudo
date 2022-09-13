package escalation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/jlevesy/kudo/escalation"
	"github.com/jlevesy/kudo/grant"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/generated/clientset/versioned/fake"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/pkg/generics"
	"github.com/jlevesy/kudo/pkg/webhooksupport/webhooktesting"
)

var (
	k8sStateFixtures = []runtime.Object{
		&kudov1alpha1.EscalationPolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       kudov1alpha1.KindEscalationPolicy,
				APIVersion: kudov1alpha1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "policy-1",
			},
			Spec: kudov1alpha1.EscalationPolicySpec{
				Subjects: []rbacv1.Subject{
					{
						Kind: rbacv1.GroupKind,
						Name: "group-b@org.com",
					},
					{
						Kind: rbacv1.UserKind,
						Name: "user-c",
					},
				},
				Target: kudov1alpha1.EscalationTarget{
					MaxDuration: metav1.Duration{Duration: time.Hour},
					Grants: []kudov1alpha1.ValueWithKind{
						kudov1alpha1.MustEncodeValueWithKind(testGrantKind, struct{}{}),
					},
				},
			},
		},
		&kudov1alpha1.EscalationPolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       kudov1alpha1.KindEscalationPolicy,
				APIVersion: kudov1alpha1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "policy-bad-grant-kind",
			},
			Spec: kudov1alpha1.EscalationPolicySpec{
				Subjects: []rbacv1.Subject{
					{
						Kind: rbacv1.GroupKind,
						Name: "group-b@org.com",
					},
					{
						Kind: rbacv1.UserKind,
						Name: "user-c",
					},
				},
				Target: kudov1alpha1.EscalationTarget{
					Grants: []kudov1alpha1.ValueWithKind{
						kudov1alpha1.MustEncodeValueWithKind("nonsense", struct{}{}),
					},
				},
			},
		},
	}
)

func TestCreateEscalationAdmissionReviewer_ReviewAdmission(t *testing.T) {
	testCases := []struct {
		desc    string
		request *admissionv1.AdmissionRequest

		grantValidateErr error

		wantError    error
		wantResponse *admissionv1.AdmissionResponse
	}{
		{
			desc: "denies if the policy is not known",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "some-unknown-policy",
								Reason:     "unlimited POWERRRRRRR!",
							},
						},
					).Bytes(),
				},
			},
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "Unknown policy: some-unknown-policy",
				},
			},
		},
		{
			desc: "denies if the escalation has no reason",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "policy-1",
							},
						},
					).Bytes(),
				},
				UserInfo: authenticationv1.UserInfo{
					Username: "user",
				},
			},
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "Please provide a reason for your escalation request",
				},
			},
		},
		{
			desc: "denies if the user is not allowed to use the policy",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "policy-1",
								Reason:     "I need moar power",
							},
						},
					).Bytes(),
				},
				UserInfo: authenticationv1.UserInfo{
					Username: "user",
				},
			},
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "User \"user\" is not allowed to use the escalation policy \"policy-1\"",
				},
			},
		},
		{
			desc: "denies if the duration asked by the user exceeds the policy max duration",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "policy-1",
								Reason:     "I need moar power",
								Duration:   metav1.Duration{Duration: 10 * time.Hour},
							},
						},
					).Bytes(),
				},
				UserInfo: authenticationv1.UserInfo{
					Username: "user-c",
				},
			},
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "Wanted duration [10h0m0s] exceeds the maxium duration allowed by the policy [1h0m0s]",
				},
			},
		},
		{
			desc: "denies if the refered policy has an unsupported grant kind",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "policy-bad-grant-kind",
								Reason:     "I need moar power",
							},
						},
					).Bytes(),
				},
				UserInfo: authenticationv1.UserInfo{
					Username: "user-c",
				},
			},
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "Policy \"policy-bad-grant-kind\" refers to an unsuported grant kind \"nonsense\"",
				},
			},
		},
		{
			desc: "denies if the escalation is not compatible with grant",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "policy-1",
								Reason:     "I need moar power",
							},
						},
					).Bytes(),
				},
				UserInfo: authenticationv1.UserInfo{
					Username: "user-c",
				},
			},
			grantValidateErr: errors.New("hahahaha"),
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "Escalation is impossible to grant, reason is: hahahaha",
				},
			},
		},
		{
			desc: "allows users by username",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "policy-1",
								Reason:     "I need moar power",
							},
						},
					).Bytes(),
				},
				UserInfo: authenticationv1.UserInfo{
					Username: "user-c",
				},
			},
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed:   true,
				Result:    &metav1.Status{Status: metav1.StatusSuccess},
				PatchType: generics.Ptr(admissionv1.PatchTypeJSONPatch),
				Patch:     []byte(`[{"op":"replace","path":"/spec/requestor","value":"user-c"}]`),
			},
		},
		{
			desc: "allows users by group membership",
			request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.Escalation{
							Spec: kudov1alpha1.EscalationSpec{
								PolicyName: "policy-1",
								Reason:     "I need moar power",
							},
						},
					).Bytes(),
				},
				UserInfo: authenticationv1.UserInfo{
					Username: "user-b",
					Groups:   []string{"group-b@org.com"},
				},
			},
			wantResponse: &admissionv1.AdmissionResponse{
				Allowed:   true,
				Result:    &metav1.Status{Status: metav1.StatusSuccess},
				PatchType: generics.Ptr(admissionv1.PatchTypeJSONPatch),
				Patch:     []byte(`[{"op":"replace","path":"/spec/requestor","value":"user-b"}]`),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)

				fakeClient         = fake.NewSimpleClientset(k8sStateFixtures...)
				informersFactories = kudoinformers.NewSharedInformerFactory(
					fakeClient,
					60*time.Second,
				)
				escalationPolicyInformer = informersFactories.K8s().V1alpha1().EscalationPolicies()

				dummyGranter = mockGranter{
					ValidateFn: func(_ *kudov1alpha1.Escalation, _ kudov1alpha1.ValueWithKind) error {
						return testCase.grantValidateErr
					},
				}

				reviewer = escalation.NewCreateAdmissionReviewer(
					escalationPolicyInformer.Lister(),
					grant.StaticFactory{
						testGrantKind: injectMockGranter(&dummyGranter),
					},
				)
			)

			defer cancel()

			informersFactories.Start(ctx.Done())

			if ok := cache.WaitForCacheSync(ctx.Done(), escalationPolicyInformer.Informer().HasSynced); !ok {
				t.Fatal("Cache sync failed, failing test...")
			}

			gotResp, err := reviewer.ReviewAdmission(ctx, testCase.request)

			assert.Equal(t, testCase.wantError, err)
			assert.Equal(t, testCase.wantResponse, gotResp)
		})
	}
}
