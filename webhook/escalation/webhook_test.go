package escalation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/generated/clientset/versioned/fake"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/pkg/generics"
	"github.com/jlevesy/kudo/webhook/escalation"
)

const requestUID = "request-uid"

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
			},
		},
	}

	unexpectedErrorReview = admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			Allowed: false,
			Result:  &escalation.UnexpectedErrorStatus,
		},
	}

	unexpectedErrorReviewWithUID = admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			UID:     requestUID,
			Allowed: false,
			Result:  &escalation.UnexpectedErrorStatus,
		},
	}
)

func TestEscalationWebhookHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		desc        string
		requestBody io.Reader

		wantStatusCode int
		wantReview     admissionv1.AdmissionReview
	}{
		{
			desc:           "raises bad request when request body is not JSON",
			requestBody:    bytes.NewBuffer([]byte("notjson")),
			wantStatusCode: http.StatusBadRequest,
			wantReview:     unexpectedErrorReview,
		},
		{
			desc: "raises an error if request is missing",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{},
			),
			wantStatusCode: http.StatusOK,
			wantReview:     unexpectedErrorReview,
		},
		{
			desc: "raises an error if kind is unexpected",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID: requestUID,
						Kind: metav1.GroupVersionKind{
							Group:   "something.bar",
							Version: "v1",
							Kind:    "Something",
						},
					},
				},
			),
			wantStatusCode: http.StatusOK,
			wantReview:     unexpectedErrorReviewWithUID,
		},
		{
			desc: "raises an error if operation is not CREATE",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID:       requestUID,
						Kind:      escalation.ExpectedKind,
						Operation: admissionv1.Delete,
					},
				},
			),
			wantStatusCode: http.StatusOK,
			wantReview:     unexpectedErrorReviewWithUID,
		},
		{
			desc: "raises an error if the policy is not known",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID:       requestUID,
						Kind:      escalation.ExpectedKind,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: encodeObject(
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
				},
			),
			wantStatusCode: http.StatusOK,
			wantReview: admissionv1.AdmissionReview{
				Response: &admissionv1.AdmissionResponse{
					UID:     requestUID,
					Allowed: false,
					Result: &metav1.Status{
						Status:  metav1.StatusFailure,
						Message: "Unknown policy: some-unknown-policy",
					},
				},
			},
		},
		{
			desc: "raises an error if the escalation has no reason",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID:       requestUID,
						Kind:      escalation.ExpectedKind,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: encodeObject(
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
				},
			),
			wantStatusCode: http.StatusOK,
			wantReview: admissionv1.AdmissionReview{
				Response: &admissionv1.AdmissionResponse{
					UID:     requestUID,
					Allowed: false,
					Result: &metav1.Status{
						Status:  metav1.StatusFailure,
						Message: "Please provide a reason for your escalation request",
					},
				},
			},
		},
		{
			desc: "raises an error if the user is not allowed to use the policy",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID:       requestUID,
						Kind:      escalation.ExpectedKind,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: encodeObject(
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
				},
			),
			wantStatusCode: http.StatusOK,
			wantReview: admissionv1.AdmissionReview{
				Response: &admissionv1.AdmissionResponse{
					UID:     requestUID,
					Allowed: false,
					Result: &metav1.Status{
						Status:  metav1.StatusFailure,
						Message: "User \"user\" is not allowed to use the escalation policy \"policy-1\"",
					},
				},
			},
		},
		{
			desc: "allows users by username",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID:       requestUID,
						Kind:      escalation.ExpectedKind,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: encodeObject(
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
				},
			),
			wantStatusCode: http.StatusOK,
			wantReview: admissionv1.AdmissionReview{
				Response: &admissionv1.AdmissionResponse{
					UID:       requestUID,
					Allowed:   true,
					Result:    &metav1.Status{Status: metav1.StatusSuccess},
					PatchType: generics.Ptr(admissionv1.PatchTypeJSONPatch),
					Patch:     []byte(`[{"op":"replace","path":"/spec/requestor","value":"user-c"}]`),
				},
			},
		},
		{
			desc: "allows users by group membership",
			requestBody: encodeObject(
				t,
				admissionv1.AdmissionReview{
					Request: &admissionv1.AdmissionRequest{
						UID:       requestUID,
						Kind:      escalation.ExpectedKind,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: encodeObject(
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
				},
			),
			wantStatusCode: http.StatusOK,
			wantReview: admissionv1.AdmissionReview{
				Response: &admissionv1.AdmissionResponse{
					UID:       requestUID,
					Allowed:   true,
					Result:    &metav1.Status{Status: metav1.StatusSuccess},
					PatchType: generics.Ptr(admissionv1.PatchTypeJSONPatch),
					Patch:     []byte(`[{"op":"replace","path":"/spec/requestor","value":"user-b"}]`),
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			var (
				ctx, cancel    = context.WithTimeout(context.Background(), time.Second)
				responseWriter = httptest.NewRecorder()
				request        = httptest.NewRequest(
					http.MethodPost,
					"/v1alpha1/escalations",
					test.requestBody,
				)

				fakeClient = fake.NewSimpleClientset(k8sStateFixtures...)

				informersFactories = kudoinformers.NewSharedInformerFactory(
					fakeClient,
					60*time.Second,
				)

				escalationPolicyInformer = informersFactories.K8s().V1alpha1().EscalationPolicies()

				handler = escalation.NewWebhookHandler(escalationPolicyInformer.Lister())
			)

			defer cancel()

			informersFactories.Start(ctx.Done())

			if ok := cache.WaitForCacheSync(ctx.Done(), escalationPolicyInformer.Informer().HasSynced); !ok {
				t.Fatal("Cache sync failed, failing test...")
			}

			handler.ServeHTTP(responseWriter, request)

			assert.Equal(t, test.wantStatusCode, responseWriter.Code)
			assert.Equal(t, "application/json", responseWriter.Header().Get("Content-Type"))

			var gotReview admissionv1.AdmissionReview

			err := json.NewDecoder(responseWriter.Body).Decode(&gotReview)
			require.NoError(t, err)

			// We don't care about the request here, let's drop it to ease assertions.
			gotReview.Request = nil

			assert.Equal(t, test.wantReview, gotReview)
		})
	}
}

func encodeObject(t *testing.T, object any) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	err := json.NewEncoder(&buf).Encode(&object)
	require.NoError(t, err)

	return &buf
}
