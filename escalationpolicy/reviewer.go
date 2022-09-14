package escalationpolicy

import (
	"context"
	"encoding/json"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

type admissionReviewer struct{}

func NewAdmissionReviewer() webhooksupport.AdmissionReviewer {
	return &admissionReviewer{}
}

func (r *admissionReviewer) ReviewAdmission(ctx context.Context, req *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	var policy kudov1alpha1.EscalationPolicy

	if err := json.Unmarshal(req.Object.Raw, &policy); err != nil {
		klog.ErrorS(err, "Can't unmarhal object to review")

		return nil, err
	}

	if policy.Spec.Target.MaxDuration.Duration == 0 ||
		policy.Spec.Target.DefaultDuration.Duration == 0 {
		klog.Info("policy doesn't have a default or a max duration")

		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "Escalation policy must have a default and a max duration",
			},
		}, nil
	}

	if policy.Spec.Target.DefaultDuration.Duration > policy.Spec.Target.MaxDuration.Duration {
		klog.Info("policy has a default duration that exceeds the max duration")

		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "Escalation policy default duration must not exceed max duration",
			},
		}, nil
	}

	return &admissionv1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{Status: metav1.StatusSuccess},
	}, nil
}
