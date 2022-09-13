package webhooksupport_test

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/pkg/webhooksupport"
	"github.com/stretchr/testify/assert"
)

func TestRouteByOperation(t *testing.T) {
	var (
		createCalled bool

		router = webhooksupport.RouteByOperation(
			webhooksupport.HandleOperation(
				admissionv1.Create,
				webhooksupport.AdmissionReviewerFunc(func(context.Context, *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
					createCalled = true
					return &admissionv1.AdmissionResponse{}, nil
				}),
			),
		)
	)

	_, _ = router.ReviewAdmission(context.Background(), &admissionv1.AdmissionRequest{Operation: admissionv1.Create})
	assert.True(t, createCalled)

	resp, _ := router.ReviewAdmission(context.Background(), &admissionv1.AdmissionRequest{Operation: admissionv1.Update})
	assert.Equal(
		t,
		&admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "Unsupported operation",
			},
		},
		resp,
	)
}

func TestRequireKind(t *testing.T) {
	var (
		createCalled bool

		router = webhooksupport.RequireKind(
			metav1.GroupVersionKind{Group: "test"},
			webhooksupport.AdmissionReviewerFunc(func(context.Context, *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
				createCalled = true
				return &admissionv1.AdmissionResponse{}, nil
			}),
		)
	)

	_, _ = router.ReviewAdmission(context.Background(), &admissionv1.AdmissionRequest{Kind: metav1.GroupVersionKind{Group: "test"}})
	assert.True(t, createCalled)

	resp, _ := router.ReviewAdmission(context.Background(), &admissionv1.AdmissionRequest{Kind: metav1.GroupVersionKind{Group: "pastest"}})
	assert.Equal(
		t,
		&admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "Received unexpected kind pastest/, Kind=",
			},
		},
		resp,
	)
}
