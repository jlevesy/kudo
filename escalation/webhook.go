package escalation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/jlevesy/kudo/grant"
	kudo "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev"
	"github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/generics"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

var (
	ExpectedKind = metav1.GroupVersionKind{
		Group:   kudo.GroupName,
		Version: kudov1alpha1.Version,
		Kind:    kudov1alpha1.KindEscalation,
	}

	UnexpectedErrorStatus = metav1.Status{
		Status:  metav1.StatusFailure,
		Message: "Unexpected error, see controller logs for details",
	}
)

type EscalationPoliciesGetter interface {
	Get(name string) (*v1alpha1.EscalationPolicy, error)
}

type WebhookHandler struct {
	policiesGetter EscalationPoliciesGetter
	grantFactory   grant.Factory
}

func NewWebhookHandler(g EscalationPoliciesGetter, f grant.Factory) *WebhookHandler {
	return &WebhookHandler{policiesGetter: g, grantFactory: f}
}

func (h *WebhookHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	review := admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{},
	}

	klog.Info("Handing a mutation request")

	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		klog.ErrorS(err, "Unable to decode webhook request")
		review.Response.Result = &UnexpectedErrorStatus

		// Can't really say more here, as I don't have the admission request.
		webhooksupport.WriteJSON(rw, http.StatusBadRequest, &review)
		return
	}

	if review.Request == nil {
		klog.Error("Received malformed review payload without any request")
		review.Response.Result = &UnexpectedErrorStatus

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return
	}

	review.Response.UID = review.Request.UID

	if review.Request.Kind != ExpectedKind {
		klog.Errorf(
			"Received unexpected review kind %q for user %q",
			review.Request.Kind,
			review.Request.UserInfo.Username,
		)

		review.Response.Result = &UnexpectedErrorStatus

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return
	}

	if review.Request.Operation != admissionv1.Create {
		klog.Errorf(
			"Received unexpected operation %q for user %q",
			review.Request.Operation,
			review.Request.UserInfo.Username,
		)

		review.Response.Result = &UnexpectedErrorStatus

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return
	}

	var escalation kudov1alpha1.Escalation

	if err := json.Unmarshal(review.Request.Object.Raw, &escalation); err != nil {
		klog.ErrorS(err, "Can't unmarhal created object")

		review.Response.Result = &UnexpectedErrorStatus

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return
	}

	if strings.TrimSpace(escalation.Spec.Reason) == "" {
		klog.InfoS(
			"User submitted an escalation request without any reason",
			"username",
			review.Request.UserInfo.Username,
		)

		review.Response.Result = &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: "Please provide a reason for your escalation request",
		}

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return

	}

	policy, err := h.policiesGetter.Get(escalation.Spec.PolicyName)

	switch {
	case errors.IsNotFound(err):
		klog.InfoS(
			"User submitted an escalation request refering to a policy that doesn't exist",
			usernameAndPolicyTags(
				review.Request.UserInfo.Username,
				escalation.Spec.PolicyName,
			)...,
		)

		review.Response.Result = &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: fmt.Sprintf("Unknown policy: %s", escalation.Spec.PolicyName),
		}

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return
	case err != nil:
		klog.ErrorS(err, "Unable to retrieve escalation policy")
		review.Response.Result = &UnexpectedErrorStatus

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return
	default:
		// We're good.
	}

	if !userAllowed(*policy, review.Request.UserInfo) {
		klog.InfoS(
			"User attempted to use an escalation policy, but is not part of the policy subjects",
			usernameAndPolicyTags(
				review.Request.UserInfo.Username,
				policy.Name,
			)...,
		)

		review.Response.Result = &metav1.Status{
			Status: metav1.StatusFailure,
			Message: fmt.Sprintf(
				"User %q is not allowed to use the escalation policy %q",
				review.Request.UserInfo.Username,
				policy.Name,
			),
		}

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return
	}

	for _, grant := range policy.Spec.Target.Grants {
		granter, err := h.grantFactory.Get(grant.Kind)
		if err != nil {
			klog.InfoS(
				"Refered escalation policy has a grant that is not supported",
				usernameAndPolicyTags(
					review.Request.UserInfo.Username,
					policy.Name,
				)...,
			)

			review.Response.Result = &metav1.Status{
				Status: metav1.StatusFailure,
				Message: fmt.Sprintf(
					"Policy %q refers to an unsuported grant kind %q",
					policy.Name,
					grant.Kind,
				),
			}

			webhooksupport.WriteJSON(rw, http.StatusOK, &review)
			return
		}

		if err = granter.Validate(r.Context(), &escalation, grant); err != nil {
			klog.ErrorS(
				err,
				"User submitted an invalid escalation",
				usernameAndPolicyTags(
					review.Request.UserInfo.Username,
					policy.Name,
				),
			)

			review.Response.Result = &metav1.Status{
				Status: metav1.StatusFailure,
				Message: fmt.Sprintf(
					"Escalation is impossible to grant, reason is: %s",
					err,
				),
			}

			webhooksupport.WriteJSON(rw, http.StatusOK, &review)
			return
		}
	}

	if review.Response.Patch, err = genObjectPatch(review.Request.UserInfo); err != nil {
		klog.ErrorS(
			err,
			"Unable to generate object patch",
			usernameAndPolicyTags(
				review.Request.UserInfo.Username,
				policy.Name,
			)...,
		)

		review.Response.Result = &UnexpectedErrorStatus

		webhooksupport.WriteJSON(rw, http.StatusOK, &review)
		return

	}

	klog.InfoS(
		"User submitted an escalation request",
		"requestor",
		review.Request.UserInfo.Username,
		"policy",
		escalation.Spec.PolicyName,
	)

	review.Response.Allowed = true
	review.Response.Result = &metav1.Status{Status: metav1.StatusSuccess}
	review.Response.PatchType = generics.Ptr(admissionv1.PatchTypeJSONPatch)

	webhooksupport.WriteJSON(rw, http.StatusOK, &review)
}

// userAllowed returns true when an user is allowed to use an escalation policy based
// on the policy subjects.
// An user is allowed if and only if one of the policy subject:
// - is of Kind user with the same username.
// - is of Kind group and the user belongs to this group.
func userAllowed(policy kudov1alpha1.EscalationPolicy, user authenticationv1.UserInfo) bool {
	for _, subject := range policy.Spec.Subjects {
		switch subject.Kind {
		case rbacv1.GroupKind:
			if generics.Contains(user.Groups, subject.Name) {
				return true
			}
		case rbacv1.UserKind:
			if subject.Name == user.Username {
				return true
			}
		}
	}

	return false
}

func genObjectPatch(user authenticationv1.UserInfo) ([]byte, error) {
	patch := []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}{
		{
			Op:    "replace",
			Path:  "/spec/requestor",
			Value: user.Username,
		},
	}

	return json.Marshal(&patch)
}

func usernameAndPolicyTags(username, policy string, anything ...any) []any {
	return append(
		[]any{
			"username",
			username,
			"policy",
			policy,
		},
		anything...,
	)
}
