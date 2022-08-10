package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Kind      string         `json:"kind"`
	Namespace string         `json:"namespace"`
	RoleRef   rbacv1.RoleRef `json:"roleRef"`
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

type EscalationSpec struct {
	PolicyName string `json:"policyName"`
	Requestor  string `json:"requestor"`
	Reason     string `json:"reason"`
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
	State        EscalationState      `json:"state"`
	StateDetails string               `json:"stateDetails"`
	GrantRefs    []EscalationGrantRef `json:"grantRefs"`
}

type GrantStatus string

const (
	GrantStatusUnknown   GrantStatus = ""
	GrantStatusCreated   GrantStatus = "CREATED"
	GrantStatusReclaimed GrantStatus = "RECLAIMED"
)

type EscalationGrantRef struct {
	Kind      string      `json:"kind"`
	Name      string      `json:"name"`
	Namespace string      `json:"namespace"`
	Status    GrantStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EscalationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Escalation `json:"items"`
}
