package grant

import (
	"context"
	stderrors "errors"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/klog/v2"

	"github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/generics"
)

const (
	managedByLabel        = "app.kubernetes.io/created-by"
	defaultManagedByValue = "kudo"
)

var (
	ErrNamespaceNotAllowed = stderrors.New("namespace is not allowed")
	ErrNoNamespace         = stderrors.New("no namespace could be picked")
)

type k8sRoleBindingGranter struct {
	rbacClient        rbacv1client.RbacV1Interface
	roleBindingLister rbacv1listers.RoleBindingLister
}

func newK8sRoleBindingGranter(rbacClient rbacv1client.RbacV1Interface, rbacLister rbacv1listers.RoleBindingLister) (*k8sRoleBindingGranter, error) {
	return &k8sRoleBindingGranter{
		rbacClient:        rbacClient,
		roleBindingLister: rbacLister,
	}, nil
}

func (g *k8sRoleBindingGranter) Create(ctx context.Context, esc *kudov1alpha1.Escalation, grant kudov1alpha1.ValueWithKind) (kudov1alpha1.EscalationGrantRef, error) {
	k8sGrant, err := kudov1alpha1.DecodeValueWithKind[kudov1alpha1.K8sRoleBindingGrant](grant)
	if err != nil {
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	roleBinding, err := g.findRoleBinding(esc, k8sGrant)
	if err != nil {
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	if roleBinding != nil {
		asValue, err := v1alpha1.EncodeValueWithKind(
			grant.Kind,
			kudov1alpha1.K8sRoleBindingGrantRef{
				Name:            roleBinding.Name,
				Namespace:       roleBinding.Namespace,
				UID:             roleBinding.UID,
				ResourceVersion: roleBinding.ResourceVersion,
			},
		)

		return kudov1alpha1.EscalationGrantRef{
			Ref:    asValue,
			Status: kudov1alpha1.GrantStatusCreated,
		}, err
	}

	ns, err := targetNamespace(esc, k8sGrant)
	if err != nil {
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	roleBinding, err = g.rbacClient.RoleBindings(ns).Create(
		ctx,
		&rbacv1.RoleBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "RoleBinding",
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kudo-grant-",
				Namespace:    ns,
				OwnerReferences: []metav1.OwnerReference{
					esc.AsOwnerRef(),
				},
				Labels: map[string]string{
					managedByLabel: defaultManagedByValue,
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.UserKind,
					Name: esc.Spec.Requestor,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.SchemeGroupVersion.Group,
				Kind:     k8sGrant.RoleRef.Kind,
				Name:     k8sGrant.RoleRef.Name,
			},
		},
		metav1.CreateOptions{},
	)

	if err != nil {
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	klog.InfoS(
		"Created a new role binding",
		"escalation",
		esc.Name,
		"namespace",
		ns,
		"roleRef",
		k8sGrant.RoleRef.Name,
		"roleBindingName",
		roleBinding.Name,
	)

	encodedRef, err := v1alpha1.EncodeValueWithKind(
		kudov1alpha1.GrantKindK8sRoleBinding,
		kudov1alpha1.K8sRoleBindingGrantRef{
			Name:            roleBinding.Name,
			Namespace:       roleBinding.Namespace,
			UID:             roleBinding.UID,
			ResourceVersion: roleBinding.ResourceVersion,
		},
	)

	if err != nil {
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	return kudov1alpha1.EscalationGrantRef{
		Status: kudov1alpha1.GrantStatusCreated,
		Ref:    encodedRef,
	}, nil
}

func (g *k8sRoleBindingGranter) Reclaim(ctx context.Context, ref kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error) {
	k8sRef, err := kudov1alpha1.DecodeValueWithKind[v1alpha1.K8sRoleBindingGrantRef](ref.Ref)
	if err != nil {
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	status := kudov1alpha1.EscalationGrantRef{
		Status: kudov1alpha1.GrantStatusReclaimed,
		Ref:    ref.Ref,
	}

	_, err = g.roleBindingLister.RoleBindings(k8sRef.Namespace).Get(k8sRef.Name)
	switch {
	case errors.IsNotFound(err):
		return status, nil
	case err != nil:
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	err = g.rbacClient.RoleBindings(k8sRef.Namespace).Delete(ctx, k8sRef.Name, metav1.DeleteOptions{})
	switch {
	case errors.IsNotFound(err):
		return status, nil
	case err != nil:
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	klog.InfoS(
		"Deleted a role binding",
		"namespace",
		k8sRef.Namespace,
		"roleBndingName",
		k8sRef.Name,
	)

	return status, nil
}

// Validate makes sure that the target namespace is properly defined.
func (g *k8sRoleBindingGranter) Validate(_ context.Context, esc *kudov1alpha1.Escalation, grant kudov1alpha1.ValueWithKind) error {
	k8sGrant, err := kudov1alpha1.DecodeValueWithKind[v1alpha1.K8sRoleBindingGrant](grant)
	if err != nil {
		return err
	}

	_, err = targetNamespace(esc, k8sGrant)
	return err
}

func (g *k8sRoleBindingGranter) findRoleBinding(esc *kudov1alpha1.Escalation, grant *kudov1alpha1.K8sRoleBindingGrant) (*rbacv1.RoleBinding, error) {
	for _, grantRef := range esc.Status.GrantRefs {
		if grantRef.Ref.Kind != kudov1alpha1.GrantKindK8sRoleBinding || grantRef.Status != kudov1alpha1.GrantStatusCreated {
			continue
		}

		k8sRef, err := v1alpha1.DecodeValueWithKind[kudov1alpha1.K8sRoleBindingGrantRef](grantRef.Ref)
		if err != nil {
			return nil, err
		}

		ns, err := targetNamespace(esc, grant)
		if err != nil {
			return nil, err
		}

		binding, err := g.roleBindingLister.RoleBindings(ns).Get(k8sRef.Name)
		switch {
		case errors.IsNotFound(err):
			continue
		case err != nil:
			return nil, err
		}

		// Lookup for a binding, check it's UID and ResourceVersion if it has been tampered, fail the escalation.
		if binding.UID != k8sRef.UID || binding.ResourceVersion != k8sRef.ResourceVersion {
			return nil, fmt.Errorf(
				"%w: Role binding %s in namespace %s",
				ErrTampered,
				binding.Name,
				binding.Namespace,
			)
		}

		// If the binding matches the grant we want to create the all good.
		if binding.RoleRef.Kind == grant.RoleRef.Kind &&
			binding.RoleRef.Name == grant.RoleRef.Name {
			return binding, nil
		}
	}

	return nil, nil
}

func targetNamespace(esc *kudov1alpha1.Escalation, grant *kudov1alpha1.K8sRoleBindingGrant) (string, error) {
	// If we don't have a namespace specified, then see if the grant specifies a default namespace.
	// It yes, use it, if not fail with panache.
	if esc.Spec.Namespace == "" {
		if grant.DefaultNamespace != "" {
			return grant.DefaultNamespace, nil
		}

		return "", ErrNoNamespace
	}

	// Now if we're using namespace requested by the user, make sure the policy allows it.
	if !generics.Contains(grant.AllowedNamespaces, esc.Spec.Namespace) {
		return "", fmt.Errorf(
			"%w namespace: %s, allowed values: %v",
			ErrNamespaceNotAllowed,
			esc.Spec.Namespace,
			grant.AllowedNamespaces,
		)
	}

	return esc.Spec.Namespace, nil
}
