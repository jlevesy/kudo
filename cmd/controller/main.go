package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/jlevesy/kudo/audit"
	"github.com/jlevesy/kudo/controller"
	"github.com/jlevesy/kudo/escalation"
	"github.com/jlevesy/kudo/escalationpolicy"
	"github.com/jlevesy/kudo/grant"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
	clientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

var (
	configPath string
	kubeConfig string
	masterURL  string
)

func main() {
	flag.StringVar(&configPath, "config", "", "Path to Kudo Configuration")
	flag.StringVar(&kubeConfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	klog.InitFlags(nil)
	defer klog.Flush()

	flag.Parse()

	klog.Info("Loading Kudo controller configuration", "path", configPath)

	kudoCfg, err := controller.LoadConfigurationFromFile(configPath)
	if err != nil {
		klog.Fatalf("Unable to load kudo configuration: %s", err.Error())
	}

	klog.Info("Starting kudo controller")

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
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

	auditSink, err := audit.BuildSinkFromConfig(kudoCfg.Audit, kubeClient)
	if err != nil {
		klog.Fatalf("Unable to build audit sink: %s", err.Error())
	}

	var (
		serveMux = http.NewServeMux()

		kubeInformerFactory = kubeinformers.NewSharedInformerFactory(
			kubeClient,
			kudoCfg.Controller.InformerResyncInterval.Duration,
		)
		kudoInformerFactory = kudoinformers.NewSharedInformerFactory(
			kudoClientSet,
			kudoCfg.Controller.InformerResyncInterval.Duration,
		)
		escalationsInformer = kudoInformerFactory.K8s().V1alpha1().Escalations().Informer()
		escalationsClient   = kudoClientSet.K8sV1alpha1().Escalations()
		policiesLister      = kudoInformerFactory.K8s().V1alpha1().EscalationPolicies().Lister()

		granterFactory = grant.DefaultGranterFactory(kubeInformerFactory, kubeClient)

		escalationController = controllersupport.NewQueuedEventHandler[kudov1alpha1.Escalation](
			escalation.NewController(
				policiesLister,
				escalationsClient,
				granterFactory,
				auditSink,
				escalation.WithResyncInterval(kudoCfg.Controller.ResyncInterval.Duration),
				escalation.WithRetryInterval(kudoCfg.Controller.RetryInterval.Duration),
			),
			kudov1alpha1.KindEscalation,
			kudoCfg.Controller.Threadiness,
		)
	)

	escalationsInformer.AddEventHandler(escalationController)

	escalationpolicy.SetupWebhook(serveMux)
	escalation.SetupWebhook(serveMux, kudoInformerFactory, granterFactory)
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
		return webhooksupport.Serve(ctx, kudoCfg.Webhook, serveMux)
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
