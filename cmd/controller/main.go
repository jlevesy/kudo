package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/controllersupport"
	clientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/webhook"
)

var (
	masterURL  string
	kubeconfig string

	webhookConfig webhook.ServerConfig
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

	kudoClientSet, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Unable to build kudo clientset: %s", err.Error())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var (
		kudoInformerFactory = kudoinformers.NewSharedInformerFactory(kudoClientSet, defaultInformerResyncInterval)
		escalationsInformer = kudoInformerFactory.K8s().V1alpha1().Escalations().Informer()
		escalationHandler   = controllersupport.NewQueuedEventHandler[kudov1alpha1.Escalation](
			&logHandler{},
			kudov1alpha1.KindEscalation,
			2,
		)
	)

	escalationsInformer.AddEventHandler(escalationHandler)

	group, ctx := errgroup.WithContext(ctx)

	klog.Info("Starting informers, waiting for them to warm up...")

	kudoInformerFactory.Start(ctx.Done())

	if ok := cache.WaitForCacheSync(ctx.Done(), escalationsInformer.HasSynced); !ok {
		klog.Fatal("Cache sync failed, exiting...")
	}

	klog.Info("Informers warmed up, starting controller...")

	group.Go(func() error {
		return webhook.RunServer(ctx, kudoClientSet, webhookConfig)
	})

	group.Go(func() error {
		escalationHandler.Run(ctx)
		return nil
	})

	klog.Info("Controller up and running!")

	if err := group.Wait(); err != nil {
		klog.Error("Controller reported an error")
	}

	klog.Info("Exited kudo controller")
}

type logHandler struct{}

func (logHandler) OnAdd(escalation *kudov1alpha1.Escalation) error {
	klog.Info("RECEIVED AN ADD ==>", escalation.Name)
	return nil
}

func (logHandler) OnUpdate(oldEsc, newEsc *kudov1alpha1.Escalation) error {
	klog.Info("RECEIVED AN UPDATE ==>", oldEsc.Name)
	return nil
}

func (logHandler) OnDelete(esc *kudov1alpha1.Escalation) error {
	klog.Info("RECEIVED A DELETE ==>", esc.Name)
	return nil
}
