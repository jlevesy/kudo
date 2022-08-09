package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/jlevesy/kudo/escalation"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
	clientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

var (
	masterURL  string
	kubeconfig string

	webhookConfig webhooksupport.ServerConfig
)

const defaultInformerResyncInterval = 30 * time.Second

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&webhookConfig.CertPath, "webhook_cert", "", "Path to webhook TLS cert")
	flag.StringVar(&webhookConfig.KeyPath, "webhook_key", "", "Path to webhook TLS key")
	flag.StringVar(&webhookConfig.Addr, "webhook_addr", ":8080", "Webhook listening address")
	klog.InitFlags(nil)

	flag.Parse()

	klog.Info("Starting kudo controller")

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Unable to build kube client configuration: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Unable to build the kubernetes clientset: %s", err.Error())
	}

	kudoClientSet, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Unable to build kudo clientset: %s", err.Error())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var (
		serveMux = http.NewServeMux()

		kudoInformerFactory = kudoinformers.NewSharedInformerFactory(kudoClientSet, defaultInformerResyncInterval)
		escalationsInformer = kudoInformerFactory.K8s().V1alpha1().Escalations().Informer()
		escalationsClient   = kudoClientSet.K8sV1alpha1().Escalations()
		policiesLister      = kudoInformerFactory.K8s().V1alpha1().EscalationPolicies().Lister()

		escalationEventsHandler = controllersupport.NewQueuedEventHandler[kudov1alpha1.Escalation](
			escalation.NewEventHandler(
				policiesLister,
				escalationsClient,
				kubeClient.RbacV1(),
			),
			kudov1alpha1.KindEscalation,
			2,
		)
		escalationWebhookHandler = escalation.NewWebhookHandler(policiesLister)
	)

	escalationsInformer.AddEventHandler(escalationEventsHandler)
	serveMux.Handle("/v1alpha1/escalations", webhooksupport.MustPost(escalationWebhookHandler))

	group, ctx := errgroup.WithContext(ctx)

	klog.Info("Starting informers...")

	kudoInformerFactory.Start(ctx.Done())

	klog.Info("Waiting for the informers to warm up...")

	syncResult := kudoInformerFactory.WaitForCacheSync(ctx.Done())

	for typ, ok := range syncResult {
		if !ok {
			klog.Fatalf("Cache sync failed for %s, exiting", typ.String())
		}
	}

	klog.Info("Informers warmed up, starting controller...")

	group.Go(func() error {
		return webhooksupport.Serve(ctx, webhookConfig, serveMux)
	})

	group.Go(func() error {
		escalationEventsHandler.Run(ctx)
		return nil
	})

	klog.Info("Controller is up and running")

	if err := group.Wait(); err != nil {
		klog.Error("Controller reported an error")
	}

	klog.Info("Exited kudo controller")
}
