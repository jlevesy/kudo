package v1alpha1

import (
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/jlevesy/kudo/pkg/generics"
)

const Version = "v1alpha1"

const (
	KindEscalation       = "Escalation"
	KindEscalationPolicy = "EscalationPolicy"
)

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EscalationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec EscalationPolicySpec `json:"spec"`
}

type EscalationPolicySpec struct {
	Subjects   []rbacv1.Subject      `json:"subjects"`
	Challenges []EscalationChallenge `json:"challenges"`
	Target     EscalationTargetSpec  `json:"target"`
}

type EscalationChallenge struct {
	Kind      string           `json:"kind"`
	Reviewers []rbacv1.Subject `json:"reviewers"`
}

type EscalationTargetSpec struct {
	Duration metav1.Duration   `json:"duration"`
	Grants   []EscalationGrant `json:"grants"`
}

type EscalationGrant struct {
	Kind string `json:"kind"`

	// K8sRoleBinding configuration.
	DefaultNamespace  string         `json:"defaultNamespace"`
	AllowedNamespaces []string       `json:"allowedNamespaces"`
	RoleRef           rbacv1.RoleRef `json:"roleRef"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EscalationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []EscalationPolicy `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Escalation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EscalationSpec   `json:"spec"`
	Status EscalationStatus `json:"status"`
}

func (e *Escalation) AsOwnerRef() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         SchemeGroupVersion.String(),
		Kind:               KindEscalation,
		Name:               e.Name,
		UID:                e.UID,
		Controller:         generics.Ptr(true),
		BlockOwnerDeletion: generics.Ptr(true),
	}
}

type EscalationSpec struct {
	PolicyName string `json:"policyName"`
	Requestor  string `json:"requestor"`
	Reason     string `json:"reason"`
	Namespace  string `json:"namespace"`
}

type EscalationState string

const (
	StateUnknown  EscalationState = ""
	StatePending  EscalationState = "PENDING"
	StateDenied   EscalationState = "DENIED"
	StateAccepted EscalationState = "ACCEPTED"
	StateExpired  EscalationState = "EXPIRED"
)

type EscalationStatus struct {
	State         EscalationState      `json:"state"`
	StateDetails  string               `json:"stateDetails"`
	PolicyUID     types.UID            `json:"policyUid"`
	PolicyVersion string               `json:"policyVersion"`
	ExpiresAt     metav1.Time          `json:"expiresAt"`
	GrantRefs     []EscalationGrantRef `json:"grantRefs"`
}

func (e *EscalationStatus) AllGrantsInStatus(wantStatus GrantStatus) bool {
	if len(e.GrantRefs) == 0 {
		return false
	}

	for _, ref := range e.GrantRefs {
		if ref.Status != wantStatus {
			return false
		}
	}

	return true
}

type TransitionMutation func(st *EscalationStatus)

func WithDetails(details string) TransitionMutation {
	return func(st *EscalationStatus) {
		st.StateDetails = details
	}
}

func WithNewGrantRefs(grantRefs []EscalationGrantRef) TransitionMutation {
	return func(st *EscalationStatus) {
		st.GrantRefs = grantRefs
	}
}

func WithPolicyInfo(uid types.UID, version string) TransitionMutation {
	return func(st *EscalationStatus) {
		st.PolicyUID = uid
		st.PolicyVersion = version
	}
}

func WithExpiresAt(t time.Time) TransitionMutation {
	return func(st *EscalationStatus) {
		st.ExpiresAt = metav1.Time{Time: t}
	}
}

func (e *EscalationStatus) TransitionTo(state EscalationState, mutations ...TransitionMutation) EscalationStatus {
	newStatus := EscalationStatus{
		State:         state,
		StateDetails:  e.StateDetails,
		GrantRefs:     e.GrantRefs,
		PolicyUID:     e.PolicyUID,
		PolicyVersion: e.PolicyVersion,
		ExpiresAt:     e.ExpiresAt,
	}

	for _, mut := range mutations {
		mut(&newStatus)
	}

	return newStatus
}

type GrantStatus string

const (
	GrantStatusUnknown   GrantStatus = ""
	GrantStatusCreated   GrantStatus = "CREATED"
	GrantStatusReclaimed GrantStatus = "RECLAIMED"
)

type EscalationGrantRef struct {
	Kind            string      `json:"kind"`
	Name            string      `json:"name"`
	Namespace       string      `json:"namespace"`
	UID             types.UID   `json:"uid"`
	ResourceVersion string      `json:"resourceVersion"`
	Status          GrantStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EscalationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Escalation `json:"items"`
}
