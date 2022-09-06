package e2e

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/generated/clientset/versioned/scheme"
	"github.com/jlevesy/kudo/pkg/generics"
)

func as[T runtime.Object](t *testing.T, obj runtime.Object) T {
	v, ok := obj.(T)
	if !ok {
		t.Fatal("unable to convert")
	}

	return v
}

func assertPolicyCreated(t *testing.T, name string) *kudov1alpha1.EscalationPolicy {
	return assertObjectCreated(t, admin.kudo.K8sV1alpha1().RESTClient(), resourceNameNamespace{
		resource: "escalationpolicies",
		name:     name,
		global:   true,
	}, 3*time.Second).(*kudov1alpha1.EscalationPolicy)
}

func assertObjectCreated(t *testing.T, client rest.Interface, resourceId resourceNameNamespace, timeout time.Duration) runtime.Object {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj, err := buildGetRequest(client, resourceId).Do(ctx).Get()

	switch {
	// Objects already exists, it has been created.
	case err == nil:
		return obj
	// Object hasn't yet been created, let's watch.
	case k8serrors.IsNotFound(err):
	default:
		t.Fatal("unexpected error during assertObjectCreated:", err)
	}

	watchHandle, err := buildWatchRequest(
		client,
		resourceId,
		metav1.ListOptions{
			Watch:         true,
			FieldSelector: "metadata.name==" + resourceId.name,
		},
		timeout,
	).Watch(ctx)

	require.NoError(t, err)
	defer watchHandle.Stop()

	obj, err = waitForEvent(ctx, watchHandle, watch.Added)
	require.NoError(t, err)

	return obj
}

func assertObjectDeleted(t *testing.T, client rest.Interface, resourceId resourceNameNamespace, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj, err := buildGetRequest(client, resourceId).Do(ctx).Get()

	switch {
	// Object has been already deleted, all good.
	case k8serrors.IsNotFound(err):
		return
	// Something went wrong, fail the test.
	case err != nil:
		t.Fatal("unexpected error during assertObjectDeleted:", err)
	// Object exists, let's wait for it to be deleted.
	default:
	}

	commonObj, ok := obj.(metav1.Common)
	if !ok {
		t.Fatal("unable to extract resource version")
	}

	watchHandle, err := buildWatchRequest(
		client,
		resourceId,
		metav1.ListOptions{
			Watch:           true,
			FieldSelector:   "metadata.name==" + resourceId.name,
			ResourceVersion: commonObj.GetResourceVersion(),
		},
		timeout,
	).Watch(ctx)

	require.NoError(t, err)
	defer watchHandle.Stop()

	_, err = waitForEvent(ctx, watchHandle, watch.Added)
	require.NoError(t, err)
}

type updateCondition func(runtime.Object) bool

func assertObjectUpdated(t *testing.T, client rest.Interface, resourceId resourceNameNamespace, cond updateCondition, timeout time.Duration) runtime.Object {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj, err := buildGetRequest(client, resourceId).
		Do(ctx).
		Get()

	switch {
	// Something went wrong, fail the test.
	case err != nil:
		t.Fatal("unexpected error during assertObjectUpdated:", err)
	//  Object is found and matches the condition, return.
	case cond(obj):
		return obj
	//  Object is found but does not match the condition, watch for changes.
	default:
	}

	commonObj, ok := obj.(metav1.Common)
	if !ok {
		t.Fatal("unable to extract resource version")
	}

	watchHandle, err := buildWatchRequest(
		client,
		resourceId,
		metav1.ListOptions{
			Watch:           true,
			FieldSelector:   "metadata.name==" + resourceId.name,
			ResourceVersion: commonObj.GetResourceVersion(),
		},
		timeout,
	).Watch(ctx)
	require.NoError(t, err)
	defer watchHandle.Stop()

	// As long as we don't get a update that matches the updateCondition, wait for the event to come
	for !cond(obj) {
		obj, err = waitForEvent(ctx, watchHandle, watch.Modified)
		require.NoError(t, err)
	}

	return obj
}

type resourceNameNamespace struct {
	resource  string
	name      string
	namespace string
	global    bool
}

func buildWatchRequest(client rest.Interface, resourceId resourceNameNamespace, opts metav1.ListOptions, timeout time.Duration) *rest.Request {
	return client.
		Get().
		Resource(resourceId.resource).
		VersionedParams(
			&opts,
			scheme.ParameterCodec,
		).
		Timeout(timeout)
}

func buildGetRequest(client rest.Interface, identifier resourceNameNamespace) *rest.Request {
	req := client.Get().
		Name(identifier.name).
		Resource(identifier.resource)

	if !identifier.global {
		req = req.Namespace(identifier.namespace)
	}

	return req
}

var (
	errWatchCtxDone = errors.New("requested event hasn't happened in a reasonable delay")
	errWatchAborted = errors.New("watch unexpectedly aborted")
)

func waitForEvent(ctx context.Context, watchHandle watch.Interface, acceptedEvent watch.EventType, ignoredEvents ...watch.EventType) (runtime.Object, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, errWatchCtxDone
		case evt, ok := <-watchHandle.ResultChan():
			if !ok {
				return nil, errWatchAborted
			}

			switch {
			case generics.Contains(ignoredEvents, evt.Type):
				continue
			case evt.Type == acceptedEvent:
				return evt.Object, nil
			default:
				return nil, fmt.Errorf("received unexpected event type %v", evt)
			}
		}
	}
}
