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
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/jlevesy/kudo/escalation"
	"github.com/jlevesy/kudo/grant"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
	clientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

var (
	masterURL   string
	kubeconfig  string
	threadiness int

	webhookConfig webhooksupport.ServerConfig
)

const defaultInformerResyncInterval = 30 * time.Second

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&webhookConfig.CertPath, "webhook_cert", "", "Path to webhook TLS cert")
	flag.StringVar(&webhookConfig.KeyPath, "webhook_key", "", "Path to webhook TLS key")
	flag.StringVar(&webhookConfig.Addr, "webhook_addr", ":8080", "Webhook listening address")
	flag.IntVar(&threadiness, "threadiness", 10, "Amount of events processed in paralled")
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

		kubeInformerFactory = kubeinformers.NewSharedInformerFactory(kubeClient, defaultInformerResyncInterval)
		kudoInformerFactory = kudoinformers.NewSharedInformerFactory(kudoClientSet, defaultInformerResyncInterval)
		escalationsInformer = kudoInformerFactory.K8s().V1alpha1().Escalations().Informer()
		escalationsClient   = kudoClientSet.K8sV1alpha1().Escalations()
		policiesLister      = kudoInformerFactory.K8s().V1alpha1().EscalationPolicies().Lister()

		granterFactory = grant.DefaultGranterFactory(kubeInformerFactory, kubeClient)

		escalationController = controllersupport.NewQueuedEventHandler[kudov1alpha1.Escalation](
			escalation.NewController(
				policiesLister,
				escalationsClient,
				granterFactory,
			),
			kudov1alpha1.KindEscalation,
			threadiness,
		)
		escalationWebhookHandler = escalation.NewWebhookHandler(policiesLister, granterFactory)
	)

	escalationsInformer.AddEventHandler(escalationController)
	serveMux.Handle("/v1alpha1/escalations", webhooksupport.MustPost(escalationWebhookHandler))
	serveMux.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("ok"))
	})

	group, ctx := errgroup.WithContext(ctx)

	klog.Info("Starting informers...")

	kudoInformerFactory.Start(ctx.Done())
	kubeInformerFactory.Start(ctx.Done())

	klog.Info("Waiting for the informers to warm up...")

	controllersupport.MustSyncInformer(kudoInformerFactory.WaitForCacheSync(ctx.Done()))
	controllersupport.MustSyncInformer(kubeInformerFactory.WaitForCacheSync(ctx.Done()))

	klog.Info("Informers warmed up, starting controller...")

	group.Go(func() error {
		return webhooksupport.Serve(ctx, webhookConfig, serveMux)
	})

	group.Go(func() error {
		escalationController.Run(ctx)
		return nil
	})

	klog.Info("Controller is up and running")

	if err := group.Wait(); err != nil {
		klog.Error("Controller reported an error")
	}

	klog.Info("Exited kudo controller")
}
