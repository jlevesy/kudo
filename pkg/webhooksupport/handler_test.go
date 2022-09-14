package webhooksupport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/pkg/webhooksupport"
	"github.com/jlevesy/kudo/pkg/webhooksupport/webhooktesting"
)

func TestWebhookHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		desc string

		request         *http.Request
		reviewerError   error
		rewviewResponse *admissionv1.AdmissionResponse
		wantReview      *admissionv1.AdmissionReview
		wantStatus      int
	}{
		{
			desc:       "returns not found if method is not POST",
			request:    httptest.NewRequest(http.MethodGet, "/webhook", http.NoBody),
			wantStatus: http.StatusNotFound,
		},
		{
			desc:       "complains if fails to decoded json",
			request:    httptest.NewRequest(http.MethodPost, "/webhook", http.NoBody),
			wantStatus: http.StatusBadRequest,
			wantReview: &admissionv1.AdmissionReview{
				Response: &admissionv1.AdmissionResponse{
					Result: &metav1.Status{
						Status:  metav1.StatusFailure,
						Message: "Unexpected error, see controller logs for details",
					},
				},
			},
		},
		{
			desc: "complains if request is missing",
			request: httptest.NewRequest(http.MethodPost, "/webhook", webhooktesting.EncodeObject(
				t,
				&admissionv1.AdmissionReview{},
			)),
			wantStatus: http.StatusOK,
			wantReview: &admissionv1.AdmissionReview{
				Response: &admissionv1.AdmissionResponse{
					Result: &metav1.Status{
						Status:  metav1.StatusFailure,
						Message: "Unexpected error, see controller logs for details",
					},
				},
			},
		},
		{
			desc: "returns review response",
			request: httptest.NewRequest(http.MethodPost, "/webhook", webhooktesting.EncodeObject(
				t,
				&admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID: "uiduid",
					},
				},
			)),
			wantStatus: http.StatusOK,
			rewviewResponse: &admissionv1.AdmissionResponse{
				Allowed: true,
			},
			wantReview: &admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					UID: "uiduid",
				},
				Response: &admissionv1.AdmissionResponse{
					UID:     "uiduid",
					Allowed: true,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			var (
				reviewer = func(*admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
					return testCase.rewviewResponse, testCase.reviewerError
				}

				handler = webhooksupport.NewHandler(reviewerMock(reviewer))
				resp    = httptest.NewRecorder()
			)

			handler.ServeHTTP(resp, testCase.request)

			assert.Equal(t, testCase.wantStatus, resp.Code)

			if testCase.wantReview == nil {
				return
			}

			var gotReview admissionv1.AdmissionReview

			err := json.NewDecoder(resp.Body).Decode(&gotReview)
			require.NoError(t, err)

			assert.Equal(t, *testCase.wantReview, gotReview)
		})
	}
}

type reviewerMock func(*admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error)

func (r reviewerMock) ReviewAdmission(_ context.Context, req *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	return r(req)
}
