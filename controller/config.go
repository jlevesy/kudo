package controller

import (
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

var defaultConfig = Config{
	Audit: AuditConfig{
		Sinks: []v1alpha1.ValueWithKind{
			v1alpha1.MustEncodeValueWithKind(
				"K8sEvents",
				K8sEventsConfig{
					Namespace: "",
				},
			),
		},
	},
	Controller: ControllerConfig{
		ResyncInterval:         metav1.Duration{Duration: 30 * time.Second},
		RetryInterval:          metav1.Duration{Duration: 10 * time.Second},
		InformerResyncInterval: metav1.Duration{Duration: 1 * time.Hour},
		Threadiness:            10,
	},
	Webhook: WebhookConfig{
		CertPath: "/var/run/certs/tls.crt",
		KeyPath:  "/var/run/certs/tls.key",
		Addr:     ":8443",
		ReadTimeout: metav1.Duration{
			Duration: 20 * time.Second,
		},
		WriteTimeout: metav1.Duration{
			Duration: 20 * time.Second,
		},
	},
}

type Config struct {
	Audit      AuditConfig      `json:"audit"`
	Controller ControllerConfig `json:"controller"`
	Webhook    WebhookConfig    `json:"webhook"`
}

type AuditConfig struct {
	Sinks []v1alpha1.ValueWithKind `json:"sinks"`
}

type ControllerConfig struct {
	ResyncInterval         metav1.Duration `json:"resyncInterval"`
	RetryInterval          metav1.Duration `json:"retryInterval"`
	InformerResyncInterval metav1.Duration `json:"informerResyncInterval"`
	Threadiness            int             `json:"threadiness"`
}

type WebhookConfig struct {
	Addr         string          `json:"addr"`
	CertPath     string          `json:"certPath"`
	KeyPath      string          `json:"keyPath"`
	ReadTimeout  metav1.Duration `json:"readTimeout"`
	WriteTimeout metav1.Duration `json:"writeTimeout"`
}

type K8sEventsConfig struct {
	Namespace string `json:"namespace"`
}

func LoadConfigurationFromFile(path string) (Config, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg = defaultConfig

	return cfg, yaml.Unmarshal(bytes, &cfg)
}
