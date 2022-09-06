package escalation

import (
	"context"
	"encoding/json"
	"fmt"
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

type AdmissionReviewer struct {
	policiesGetter EscalationPoliciesGetter
	grantFactory   grant.Factory
}

func NewAdmissionReviewer(g EscalationPoliciesGetter, f grant.Factory) *AdmissionReviewer {
	return &AdmissionReviewer{policiesGetter: g, grantFactory: f}
}

func (r *AdmissionReviewer) ReviewAdmission(ctx context.Context, req *admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	if req.Kind != ExpectedKind {
		klog.Errorf(
			"Received unexpected review kind %q for user %q",
			req.Kind,
			req.UserInfo.Username,
		)

		return nil, webhooksupport.ErrUnexpectedKind
	}

	if req.Operation != admissionv1.Create {
		klog.Errorf(
			"Received unexpected operation %q for user %q",
			req.Operation,
			req.UserInfo.Username,
		)

		return nil, webhooksupport.ErrUnexpectedOperation
	}

	var escalation kudov1alpha1.Escalation

	if err := json.Unmarshal(req.Object.Raw, &escalation); err != nil {
		klog.ErrorS(err, "Can't unmarhal created object")

		return nil, err
	}

	if strings.TrimSpace(escalation.Spec.Reason) == "" {
		klog.InfoS(
			"User submitted an escalation request without any reason",
			"username",
			req.UserInfo.Username,
		)

		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "Please provide a reason for your escalation request",
			},
		}, nil
	}

	policy, err := r.policiesGetter.Get(escalation.Spec.PolicyName)

	switch {
	case errors.IsNotFound(err):
		klog.InfoS(
			"User submitted an escalation request refering to a policy that doesn't exist",
			usernameAndPolicyTags(
				req.UserInfo.Username,
				escalation.Spec.PolicyName,
			)...,
		)

		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Unknown policy: %s", escalation.Spec.PolicyName),
			},
		}, nil

	case err != nil:
		return nil, err
	default:
		// We're good.
	}

	if !userAllowed(*policy, req.UserInfo) {
		klog.InfoS(
			"User attempted to use an escalation policy, but is not part of the policy subjects",
			usernameAndPolicyTags(
				req.UserInfo.Username,
				req.Name,
			)...,
		)

		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status: metav1.StatusFailure,
				Message: fmt.Sprintf(
					"User %q is not allowed to use the escalation policy %q",
					req.UserInfo.Username,
					policy.Name,
				),
			},
		}, nil
	}

	if escalation.Spec.Duration.Duration > 0 && escalation.Spec.Duration.Duration > policy.Spec.Target.MaxDuration.Duration {
		klog.InfoS(
			"User attempted to escalate for a duration that exceeds the maximum duration of the policy",
			usernameAndPolicyTags(
				req.UserInfo.Username,
				policy.Name,
				"maxDuration",
				policy.Spec.Target.MaxDuration,
				"escalationDuration",
				escalation.Spec.Duration,
			)...,
		)

		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Status: metav1.StatusFailure,
				Message: fmt.Sprintf(
					"Wanted duration [%s] exceeds the maxium duration allowed by the policy [%s]",
					escalation.Spec.Duration.Duration,
					policy.Spec.Target.MaxDuration.Duration,
				),
			},
		}, nil
	}

	for _, grant := range policy.Spec.Target.Grants {
		granter, err := r.grantFactory.Get(grant.Kind)
		if err != nil {
			klog.InfoS(
				"Referred escalation policy has a grant that is not supported",
				usernameAndPolicyTags(
					req.UserInfo.Username,
					policy.Name,
				)...,
			)

			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Status: metav1.StatusFailure,
					Message: fmt.Sprintf(
						"Policy %q refers to an unsuported grant kind %q",
						policy.Name,
						grant.Kind,
					),
				},
			}, nil
		}

		if err = granter.Validate(ctx, &escalation, grant); err != nil {
			klog.ErrorS(
				err,
				"User submitted an invalid escalation",
				usernameAndPolicyTags(
					req.UserInfo.Username,
					policy.Name,
				),
			)

			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Status: metav1.StatusFailure,
					Message: fmt.Sprintf(
						"Escalation is impossible to grant, reason is: %s",
						err,
					),
				},
			}, nil
		}
	}

	patch, err := genObjectPatch(req.UserInfo)
	if err != nil {
		klog.ErrorS(
			err,
			"Unable to generate object patch",
			usernameAndPolicyTags(
				req.UserInfo.Username,
				policy.Name,
			)...,
		)

		return nil, err
	}

	klog.InfoS(
		"User submitted an escalation request",
		"requestor",
		req.UserInfo.Username,
		"policy",
		escalation.Spec.PolicyName,
	)

	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		Result:    &metav1.Status{Status: metav1.StatusSuccess},
		PatchType: generics.Ptr(admissionv1.PatchTypeJSONPatch),
		Patch:     patch,
	}, nil
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
