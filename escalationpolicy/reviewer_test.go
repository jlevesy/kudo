package escalationpolicy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/jlevesy/kudo/escalationpolicy"
	kudo "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/webhooksupport/webhooktesting"
)

func TestAdmissionRevierer_ReviewAdmission(t *testing.T) {
	testCases := []struct {
		desc     string
		req      *admissionv1.AdmissionRequest
		wantResp *admissionv1.AdmissionResponse
		wantErr  error
	}{
		{
			desc: "denies if policy doesn't have a default duration",
			req: &admissionv1.AdmissionRequest{
				Kind: metav1.GroupVersionKind{
					Group:   kudo.GroupName,
					Version: kudov1alpha1.Version,
					Kind:    kudov1alpha1.KindEscalationPolicy,
				},
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.EscalationPolicy{
							Spec: kudov1alpha1.EscalationPolicySpec{
								Target: kudov1alpha1.EscalationTarget{
									MaxDuration: metav1.Duration{Duration: time.Second},
								},
							},
						},
					).Bytes(),
				},
			},
			wantResp: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "Escalation policy must have a default and a max duration",
				},
			},
		},
		{
			desc: "denies if policy doesn't have a max duration",
			req: &admissionv1.AdmissionRequest{
				Kind: metav1.GroupVersionKind{
					Group:   kudo.GroupName,
					Version: kudov1alpha1.Version,
					Kind:    kudov1alpha1.KindEscalationPolicy,
				},
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.EscalationPolicy{
							Spec: kudov1alpha1.EscalationPolicySpec{
								Target: kudov1alpha1.EscalationTarget{
									DefaultDuration: metav1.Duration{Duration: time.Second},
								},
							},
						},
					).Bytes(),
				},
			},
			wantResp: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "Escalation policy must have a default and a max duration",
				},
			},
		},
		{
			desc: "denies if default duration exceeds the max duration",
			req: &admissionv1.AdmissionRequest{
				Kind: metav1.GroupVersionKind{
					Group:   kudo.GroupName,
					Version: kudov1alpha1.Version,
					Kind:    kudov1alpha1.KindEscalationPolicy,
				},
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.EscalationPolicy{
							Spec: kudov1alpha1.EscalationPolicySpec{
								Target: kudov1alpha1.EscalationTarget{
									DefaultDuration: metav1.Duration{Duration: 2 * time.Second},
									MaxDuration:     metav1.Duration{Duration: time.Second},
								},
							},
						},
					).Bytes(),
				},
			},
			wantResp: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "Escalation policy default duration must not exceed max duration",
				},
			},
		},
		{
			desc: "accepts valid duration",
			req: &admissionv1.AdmissionRequest{
				Kind: metav1.GroupVersionKind{
					Group:   kudo.GroupName,
					Version: kudov1alpha1.Version,
					Kind:    kudov1alpha1.KindEscalationPolicy,
				},
				Object: runtime.RawExtension{
					Raw: webhooktesting.EncodeObject(
						t,
						kudov1alpha1.EscalationPolicy{
							Spec: kudov1alpha1.EscalationPolicySpec{
								Target: kudov1alpha1.EscalationTarget{
									DefaultDuration: metav1.Duration{Duration: time.Second},
									MaxDuration:     metav1.Duration{Duration: 2 * time.Second},
								},
							},
						},
					).Bytes(),
				},
			},
			wantResp: &admissionv1.AdmissionResponse{
				Allowed: true,
				Result: &metav1.Status{
					Status: "Success",
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				ctx      = context.Background()
				reviewer = escalationpolicy.NewAdmissionReviewer()
			)

			gotResp, err := reviewer.ReviewAdmission(ctx, testCase.req)
			assert.Equal(t, testCase.wantErr, err)
			assert.Equal(t, testCase.wantResp, gotResp)
		})
	}
}
