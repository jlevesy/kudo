package webhooksupport

import (
	"encoding/json"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var (
	unexpectedErrorStatus = metav1.Status{
		Status:  metav1.StatusFailure,
		Message: "Unexpected error, see controller logs for details",
	}
)

type Handler struct {
	reviewer AdmissionReviewer
}

func NewHandler(reviewer AdmissionReviewer) *Handler {
	return &Handler{reviewer: reviewer}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var (
		err error

		review = admissionv1.AdmissionReview{
			Response: &admissionv1.AdmissionResponse{},
		}
	)

	// API server always sends POST, no need to expect something else.
	if r.Method != http.MethodPost {
		http.NotFound(rw, r)

		return
	}

	if err = json.NewDecoder(r.Body).Decode(&review); err != nil {
		klog.ErrorS(err, "Unable to decode webhook request")
		review.Response.Result = &unexpectedErrorStatus

		// Can't really say more here, as I don't have the admission request.
		writeJSON(rw, http.StatusBadRequest, &review)
		return
	}

	if review.Request == nil {
		klog.Error("Received malformed review payload without any request")
		review.Response.Result = &unexpectedErrorStatus

		writeJSON(rw, http.StatusOK, &review)
		return
	}

	resp, err := h.reviewer.ReviewAdmission(
		r.Context(),
		review.Request,
	)

	if err != nil {
		klog.ErrorS(err, "Reviewer reported an error")
		review.Response.Result = &unexpectedErrorStatus

		writeJSON(rw, http.StatusOK, &review)
		return
	}

	review.Response = resp
	review.Response.UID = review.Request.UID

	writeJSON(rw, http.StatusOK, &review)
}

func writeJSON(rw http.ResponseWriter, statusCode int, payload any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(statusCode)

	if err := json.NewEncoder(rw).Encode(payload); err != nil {
		klog.ErrorS(err, "Can't write JSON payload")
		return
	}
}
