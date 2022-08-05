package main

import (
	"net/http"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	clientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
	"github.com/jlevesy/kudo/webhook/escalation"
)

func main() {
	var (
		mux = http.NewServeMux()
		srv = &http.Server{
			Addr:           ":443",
			Handler:        mux,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20, // 1048576
		}
	)

	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		klog.Fatalf("Unable to build kube client configuration: %s", err.Error())
	}

	kudoClientSet, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Unable to build kubedo clientset: %s", err.Error())
	}

	webhookHandler := escalation.NewWebhookHandler(
		kudoClientSet.K8sV1alpha1().EscalationPolicies(),
	)
	klog.Info("Starting webhook handler on addr", srv.Addr)

	mux.Handle("/v1alpha1/escalations", webhooksupport.MustPost(webhookHandler))

	if err := srv.ListenAndServeTLS("/var/run/certs/tls.crt", "/var/run/certs/tls.key"); err != nil {
		klog.V(0).ErrorS(err, "Can't serve")
	}

	klog.V(0).Info("Webhook handler exited")
}
