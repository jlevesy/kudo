package controller_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/controller"
	"github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := controller.LoadConfigurationFromFile("./testdata/config.yaml")
	require.NoError(t, err)

	wantConfig := controller.Config{
		Audit: controller.AuditConfig{
			Sinks: []v1alpha1.ValueWithKind{
				v1alpha1.MustEncodeValueWithKind(
					"K8sEvents",
					controller.K8sEventsConfig{
						Namespace: "some-namespace",
					},
				),
			},
		},
		Controller: controller.ControllerConfig{
			ResyncInterval:         metav1.Duration{Duration: 50 * time.Second},
			RetryInterval:          metav1.Duration{Duration: 10 * time.Second},
			InformerResyncInterval: metav1.Duration{Duration: 30 * time.Minute},
			Threadiness:            50,
		},
		Webhook: controller.WebhookConfig{
			CertPath:     "/some/path/cert.pem",
			KeyPath:      "/some/path/key.pem",
			Addr:         ":8444",
			ReadTimeout:  metav1.Duration{Duration: 50 * time.Second},
			WriteTimeout: metav1.Duration{Duration: 20 * time.Second},
		},
	}

	assert.Equal(t, wantConfig, cfg)
}

func TestLoadConfig_AppliesDefaultConfig(t *testing.T) {
	cfg, err := controller.LoadConfigurationFromFile("./testdata/partial_config.yaml")
	require.NoError(t, err)

	wantConfig := controller.Config{
		Audit: controller.AuditConfig{
			Sinks: []v1alpha1.ValueWithKind{
				v1alpha1.MustEncodeValueWithKind(
					"K8sEvents",
					controller.K8sEventsConfig{
						Namespace: "some-namespace",
					},
				),
			},
		},
		Controller: controller.ControllerConfig{
			ResyncInterval:         metav1.Duration{Duration: 30 * time.Second},
			RetryInterval:          metav1.Duration{Duration: 10 * time.Second},
			InformerResyncInterval: metav1.Duration{Duration: time.Hour},
			Threadiness:            10,
		},
		Webhook: controller.WebhookConfig{
			CertPath:     "/var/run/certs/tls.crt",
			KeyPath:      "/var/run/certs/tls.key",
			Addr:         ":8443",
			ReadTimeout:  metav1.Duration{Duration: 20 * time.Second},
			WriteTimeout: metav1.Duration{Duration: 20 * time.Second},
		},
	}

	assert.Equal(t, wantConfig, cfg)
}
