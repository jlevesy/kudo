package grant

import (
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

func DefaultGranterFactory(
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	kubeClient kubernetes.Interface,
) Factory {
	var (
		factory = make(StaticFactory)
		// This is required to register the informer before startup.
		roleBindingLister = kubeInformerFactory.Rbac().V1().RoleBindings().Lister()
	)

	factory[K8sRoleBindingKind] = func() (Granter, error) {
		return newK8sRoleBindingGranter(
			kubeClient.RbacV1(),
			roleBindingLister,
		)
	}

	return factory
}
