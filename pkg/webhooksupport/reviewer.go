package webhooksupport

import (
	"context"

	admissionv1 "k8s.io/api/admission/v1"
)

type AdmissionReviewer interface {
	ReviewAdmission(ctx context.Context, req *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error)
}
