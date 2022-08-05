package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	clientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	"github.com/jlevesy/kudo/webhook"
)

var (
	masterURL  string
	kubeconfig string

	webhookConfig webhook.ServerConfig
)

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

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error { return webhook.RunServer(ctx, kudoClientSet, webhookConfig) })

	if err := group.Wait(); err != nil {
		klog.Error("Controller reported an error")
	}

	klog.Info("Exited kudo controller")
}
