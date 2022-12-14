// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	k8skudodevv1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	versioned "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/jlevesy/kudo/pkg/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/jlevesy/kudo/pkg/generated/listers/k8s.kudo.dev/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// EscalationPolicyInformer provides access to a shared informer and lister for
// EscalationPolicies.
type EscalationPolicyInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.EscalationPolicyLister
}

type escalationPolicyInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewEscalationPolicyInformer constructs a new informer for EscalationPolicy type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewEscalationPolicyInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredEscalationPolicyInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredEscalationPolicyInformer constructs a new informer for EscalationPolicy type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredEscalationPolicyInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.K8sV1alpha1().EscalationPolicies().List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.K8sV1alpha1().EscalationPolicies().Watch(context.TODO(), options)
			},
		},
		&k8skudodevv1alpha1.EscalationPolicy{},
		resyncPeriod,
		indexers,
	)
}

func (f *escalationPolicyInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredEscalationPolicyInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *escalationPolicyInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&k8skudodevv1alpha1.EscalationPolicy{}, f.defaultInformer)
}

func (f *escalationPolicyInformer) Lister() v1alpha1.EscalationPolicyLister {
	return v1alpha1.NewEscalationPolicyLister(f.Informer().GetIndexer())
}
