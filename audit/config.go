package audit

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/jlevesy/kudo/controller"
	"github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/generated/clientset/versioned/scheme"
)

const (
	K8sEventsSink = "K8sEvents"
)

func BuildSinkFromConfig(cfg controller.AuditConfig, kubeClient kubernetes.Interface) (Sink, error) {
	var sinks multiAsyncSink

	for _, sinkCfg := range cfg.Sinks {
		switch sinkCfg.Kind {
		case K8sEventsSink:
			k8sCfg, err := v1alpha1.DecodeValueWithKind[controller.K8sEventsConfig](sinkCfg)
			if err != nil {
				return nil, err
			}

			eventBroadcaster := record.NewBroadcaster()
			eventBroadcaster.StartStructuredLogging(0)
			eventBroadcaster.StartRecordingToSink(
				&typedcorev1.EventSinkImpl{
					Interface: kubeClient.CoreV1().Events(k8sCfg.Namespace),
				},
			)

			sinks = append(
				sinks,
				NewK8sEventSink(
					eventBroadcaster.NewRecorder(
						scheme.Scheme,
						corev1.EventSource{Component: "kudo-controller"},
					),
				),
			)
		default:
			return nil, fmt.Errorf("unsupported sink kind %q", sinkCfg.Kind)
		}

	}

	return sinks, nil
}
