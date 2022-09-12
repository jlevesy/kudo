//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha1

import (
	v1 "k8s.io/api/rbac/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Escalation) DeepCopyInto(out *Escalation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Escalation.
func (in *Escalation) DeepCopy() *Escalation {
	if in == nil {
		return nil
	}
	out := new(Escalation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Escalation) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationChallenge) DeepCopyInto(out *EscalationChallenge) {
	*out = *in
	if in.Reviewers != nil {
		in, out := &in.Reviewers, &out.Reviewers
		*out = make([]v1.Subject, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationChallenge.
func (in *EscalationChallenge) DeepCopy() *EscalationChallenge {
	if in == nil {
		return nil
	}
	out := new(EscalationChallenge)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationGrantRef) DeepCopyInto(out *EscalationGrantRef) {
	*out = *in
	in.Ref.DeepCopyInto(&out.Ref)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationGrantRef.
func (in *EscalationGrantRef) DeepCopy() *EscalationGrantRef {
	if in == nil {
		return nil
	}
	out := new(EscalationGrantRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationList) DeepCopyInto(out *EscalationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Escalation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationList.
func (in *EscalationList) DeepCopy() *EscalationList {
	if in == nil {
		return nil
	}
	out := new(EscalationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EscalationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationPolicy) DeepCopyInto(out *EscalationPolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationPolicy.
func (in *EscalationPolicy) DeepCopy() *EscalationPolicy {
	if in == nil {
		return nil
	}
	out := new(EscalationPolicy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EscalationPolicy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationPolicyList) DeepCopyInto(out *EscalationPolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]EscalationPolicy, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationPolicyList.
func (in *EscalationPolicyList) DeepCopy() *EscalationPolicyList {
	if in == nil {
		return nil
	}
	out := new(EscalationPolicyList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EscalationPolicyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationPolicySpec) DeepCopyInto(out *EscalationPolicySpec) {
	*out = *in
	if in.Subjects != nil {
		in, out := &in.Subjects, &out.Subjects
		*out = make([]v1.Subject, len(*in))
		copy(*out, *in)
	}
	if in.Challenges != nil {
		in, out := &in.Challenges, &out.Challenges
		*out = make([]EscalationChallenge, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Target.DeepCopyInto(&out.Target)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationPolicySpec.
func (in *EscalationPolicySpec) DeepCopy() *EscalationPolicySpec {
	if in == nil {
		return nil
	}
	out := new(EscalationPolicySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationSpec) DeepCopyInto(out *EscalationSpec) {
	*out = *in
	out.Duration = in.Duration
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationSpec.
func (in *EscalationSpec) DeepCopy() *EscalationSpec {
	if in == nil {
		return nil
	}
	out := new(EscalationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationStatus) DeepCopyInto(out *EscalationStatus) {
	*out = *in
	in.ExpiresAt.DeepCopyInto(&out.ExpiresAt)
	if in.GrantRefs != nil {
		in, out := &in.GrantRefs, &out.GrantRefs
		*out = make([]EscalationGrantRef, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationStatus.
func (in *EscalationStatus) DeepCopy() *EscalationStatus {
	if in == nil {
		return nil
	}
	out := new(EscalationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EscalationTarget) DeepCopyInto(out *EscalationTarget) {
	*out = *in
	out.DefaultDuration = in.DefaultDuration
	out.MaxDuration = in.MaxDuration
	if in.Grants != nil {
		in, out := &in.Grants, &out.Grants
		*out = make([]ValueWithKind, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EscalationTarget.
func (in *EscalationTarget) DeepCopy() *EscalationTarget {
	if in == nil {
		return nil
	}
	out := new(EscalationTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *K8sRoleBindingGrant) DeepCopyInto(out *K8sRoleBindingGrant) {
	*out = *in
	if in.AllowedNamespaces != nil {
		in, out := &in.AllowedNamespaces, &out.AllowedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.RoleRef = in.RoleRef
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new K8sRoleBindingGrant.
func (in *K8sRoleBindingGrant) DeepCopy() *K8sRoleBindingGrant {
	if in == nil {
		return nil
	}
	out := new(K8sRoleBindingGrant)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *K8sRoleBindingGrantRef) DeepCopyInto(out *K8sRoleBindingGrantRef) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new K8sRoleBindingGrantRef.
func (in *K8sRoleBindingGrantRef) DeepCopy() *K8sRoleBindingGrantRef {
	if in == nil {
		return nil
	}
	out := new(K8sRoleBindingGrantRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ValueWithKind) DeepCopyInto(out *ValueWithKind) {
	*out = *in
	if in.payload != nil {
		in, out := &in.payload, &out.payload
		*out = make([]byte, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ValueWithKind.
func (in *ValueWithKind) DeepCopy() *ValueWithKind {
	if in == nil {
		return nil
	}
	out := new(ValueWithKind)
	in.DeepCopyInto(out)
	return out
}
