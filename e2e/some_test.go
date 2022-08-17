package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSomething(t *testing.T) {
	nodes, err := admin.k8s.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)

	t.Log(nodes)

	_, err = userA.k8s.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	require.True(t, errors.IsUnauthorized(err))
}
