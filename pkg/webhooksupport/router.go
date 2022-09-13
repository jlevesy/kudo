package webhooksupport

import (
	"context"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type OperationOption func(r *admissionReviewRouter)

func DefaultReviewer(reviewer AdmissionReviewer) OperationOption {
	return func(r *admissionReviewRouter) {
		r.defaultReviewer = reviewer
	}
}

func HandleOperation(op admissionv1.Operation, reviewer AdmissionReviewer) OperationOption {
	return func(r *admissionReviewRouter) {
		r.routes[op] = reviewer
	}
}

func RouteByOperation(opts ...OperationOption) AdmissionReviewer {
	router := admissionReviewRouter{
		routes: make(map[admissionv1.Operation]AdmissionReviewer),
		defaultReviewer: &denyReviewer{
			reason: "Unsupported operation",
		},
	}

	for _, opt := range opts {
		opt(&router)
	}

	return &router
}

func RequireKind(want metav1.GroupVersionKind, next AdmissionReviewer) AdmissionReviewer {
	return &requireKindReviewer{
		wantKind: want,
		next:     next,
	}
}

type admissionReviewRouter struct {
	routes          map[admissionv1.Operation]AdmissionReviewer
	defaultReviewer AdmissionReviewer
}

func (a *admissionReviewRouter) ReviewAdmission(ctx context.Context, r *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	reviewer, ok := a.routes[r.Operation]
	if !ok {
		reviewer = a.defaultReviewer
	}

	return reviewer.ReviewAdmission(ctx, r)
}

type requireKindReviewer struct {
	wantKind metav1.GroupVersionKind
	next     AdmissionReviewer
}

func (r *requireKindReviewer) ReviewAdmission(ctx context.Context, req *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	if req.Kind != r.wantKind {
		klog.Errorf(
			"Received unexpected review kind %q for user %q",
			req.Kind,
			req.UserInfo.Username,
		)

		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Received unexpected kind %s", req.Kind.String()),
			},
		}, nil
	}

	return r.next.ReviewAdmission(ctx, req)

}

type denyReviewer struct {
	reason string
}

func (d *denyReviewer) ReviewAdmission(ctx context.Context, r *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	return &admissionv1.AdmissionResponse{
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: d.reason,
		},
	}, nil
}
