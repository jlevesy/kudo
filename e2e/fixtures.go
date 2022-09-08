package e2e

import (
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

func generateNamespaces(t *testing.T, amount int) []corev1.Namespace {
	t.Helper()

	nss := make([]corev1.Namespace, amount)

	for i := 0; i < amount; i++ {
		nss[i] = generateNamespace(t, i)
	}

	return nss
}

func generateNamespace(t *testing.T, i int) corev1.Namespace {
	t.Helper()

	return corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sFixtureName(t) + "-" + strconv.Itoa(i),
		},
	}
}

func generateRole(t *testing.T, i int, namespace string, rules ...rbacv1.PolicyRule) rbacv1.Role {
	t.Helper()

	return rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sFixtureName(t) + "-" + strconv.Itoa(i),
			Namespace: namespace,
		},
		Rules: rules,
	}
}

type escalationPolicyOption func(*kudov1alpha1.EscalationPolicy)

func withGrants(grants ...kudov1alpha1.EscalationGrant) escalationPolicyOption {
	return func(p *kudov1alpha1.EscalationPolicy) {
		p.Spec.Target.Grants = grants
	}
}

func withDefaultDuration(duration time.Duration) escalationPolicyOption {
	return func(p *kudov1alpha1.EscalationPolicy) {
		p.Spec.Target.DefaultDuration = metav1.Duration{Duration: duration}
	}
}

func withMaxDuration(duration time.Duration) escalationPolicyOption {
	return func(p *kudov1alpha1.EscalationPolicy) {
		p.Spec.Target.MaxDuration = metav1.Duration{Duration: duration}
	}
}

func generateEscalationPolicy(t *testing.T, opts ...escalationPolicyOption) kudov1alpha1.EscalationPolicy {
	t.Helper()

	policy := kudov1alpha1.EscalationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sFixtureName(t),
		},
		Spec: kudov1alpha1.EscalationPolicySpec{
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.UserKind,
					Name: userA.userName,
				},
			},
			Target: kudov1alpha1.EscalationTarget{
				DefaultDuration: metav1.Duration{Duration: 5 * time.Second},
				MaxDuration:     metav1.Duration{Duration: 1000 * time.Hour},
			},
		},
	}

	for _, opt := range opts {
		opt(&policy)
	}

	return policy
}

type escalationOption func(e *kudov1alpha1.Escalation)

func withNamespace(ns string) escalationOption {
	return func(e *kudov1alpha1.Escalation) {
		e.Spec.Namespace = ns
	}
}

func withDuration(duration time.Duration) escalationOption {
	return func(e *kudov1alpha1.Escalation) {
		e.Spec.Duration.Duration = duration
	}
}

func generateEscalation(t *testing.T, policyName string, opts ...escalationOption) kudov1alpha1.Escalation {
	t.Helper()

	e := kudov1alpha1.Escalation{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sFixtureName(t),
		},
		Spec: kudov1alpha1.EscalationSpec{
			PolicyName: policyName,
			Reason:     "Needs moar powerrrr",
		},
	}

	for _, opt := range opts {
		opt(&e)
	}

	return e

}

func k8sFixtureName(t *testing.T) string {
	return strings.ToLower(
		strings.ReplaceAll(
			strings.ReplaceAll(t.Name(), "_", "-"),
			"/", "-",
		),
	)
}
