package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSomething(t *testing.T) {
	nodes, err := k8sClientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)

	t.Log(nodes)
}
