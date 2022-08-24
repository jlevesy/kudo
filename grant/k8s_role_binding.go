package grant

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/klog/v2"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

const (
	K8sRoleBindingKind = "KubernetesRoleBinding"
)

const (
	managedByLabel        = "app.kubernetes.io/created-by"
	defaultManagedByValue = "kudo"
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

func (g *k8sRoleBindingGranter) Create(ctx context.Context, esc *kudov1alpha1.Escalation, grant kudov1alpha1.EscalationGrant) (kudov1alpha1.EscalationGrantRef, error) {
	roleBinding, err := g.findRoleBinding(esc, grant)
	if err != nil {
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	if roleBinding != nil {
		return kudov1alpha1.EscalationGrantRef{
			Kind:            grant.Kind,
			Name:            roleBinding.Name,
			Namespace:       roleBinding.Namespace,
			Status:          kudov1alpha1.GrantStatusCreated,
			UID:             roleBinding.UID,
			ResourceVersion: roleBinding.ResourceVersion,
		}, nil
	}

	roleBinding, err = g.rbacClient.RoleBindings(grant.Namespace).Create(
		ctx,
		&rbacv1.RoleBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "RoleBinding",
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kudo-grant-",
				Namespace:    grant.Namespace,
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
				Kind:     grant.RoleRef.Kind,
				Name:     grant.RoleRef.Name,
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
		grant.Namespace,
		"roleRef",
		grant.RoleRef.Name,
		"roleBindingName",
		roleBinding.Name,
	)

	return kudov1alpha1.EscalationGrantRef{
		Kind:            grant.Kind,
		Name:            roleBinding.Name,
		Namespace:       roleBinding.Namespace,
		Status:          kudov1alpha1.GrantStatusCreated,
		UID:             roleBinding.UID,
		ResourceVersion: roleBinding.ResourceVersion,
	}, nil
}

func (g *k8sRoleBindingGranter) Reclaim(ctx context.Context, ref kudov1alpha1.EscalationGrantRef) (kudov1alpha1.EscalationGrantRef, error) {
	status := kudov1alpha1.EscalationGrantRef{
		Kind:            ref.Kind,
		Name:            ref.Name,
		Namespace:       ref.Namespace,
		Status:          kudov1alpha1.GrantStatusReclaimed,
		UID:             ref.UID,
		ResourceVersion: ref.ResourceVersion,
	}

	_, err := g.roleBindingLister.RoleBindings(ref.Namespace).Get(ref.Name)
	switch {
	case errors.IsNotFound(err):
		return status, nil

	case err != nil:
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	err = g.rbacClient.RoleBindings(ref.Namespace).Delete(ctx, ref.Name, metav1.DeleteOptions{})
	switch {
	case errors.IsNotFound(err):
		return status, nil

	case err != nil:
		return kudov1alpha1.EscalationGrantRef{}, err
	}

	klog.InfoS(
		"Deleted a role binding",
		"namespace",
		ref.Namespace,
		"roleBndingName",
		ref.Name,
	)

	return status, nil
}

func (g *k8sRoleBindingGranter) findRoleBinding(esc *kudov1alpha1.Escalation, grant kudov1alpha1.EscalationGrant) (*rbacv1.RoleBinding, error) {
	for _, grantRef := range esc.Status.GrantRefs {
		if grantRef.Kind != K8sRoleBindingKind || grantRef.Status != kudov1alpha1.GrantStatusCreated {
			continue
		}

		binding, err := g.roleBindingLister.RoleBindings(grant.Namespace).Get(grantRef.Name)
		switch {
		case errors.IsNotFound(err):
			continue
		case err != nil:
			return nil, err
		}

		// Lookup for a binding, check it's UID and ResourceVersion if it has been tampered, fail the escalation.
		if binding.UID != grantRef.UID || binding.ResourceVersion != grantRef.ResourceVersion {
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
