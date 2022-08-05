package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	clientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
	"github.com/jlevesy/kudo/webhook/escalation"
)

var (
	masterURL  string
	kubeconfig string

	whCfg webhookConfig
)

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&whCfg.certPath, "webhook_cert", "", "Path to webhook TLS cert")
	flag.StringVar(&whCfg.keyPath, "webhook_key", "", "Path to webhook TLS key")
	flag.StringVar(&whCfg.addr, "webhook_addr", ":8080", "Webhook listening address")
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

	group.Go(func() error { return runWebhookHandler(ctx, kudoClientSet, whCfg) })

	if err := group.Wait(); err != nil {
		klog.Error("Controller reported an error")
	}

	klog.Info("Exited kudo controller")
}

type webhookConfig struct {
	certPath string
	keyPath  string
	addr     string
}

func runWebhookHandler(ctx context.Context, kudoClientSet clientset.Interface, cfg webhookConfig) error {
	var (
		mux = http.NewServeMux()
		srv = &http.Server{
			Addr:           cfg.addr,
			Handler:        mux,
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			MaxHeaderBytes: 1 << 20, // 1048576

		}
		serveFailed = make(chan error)
	)

	webhookHandler := escalation.NewWebhookHandler(
		kudoClientSet.K8sV1alpha1().EscalationPolicies(),
	)

	mux.Handle("/v1alpha1/escalations", webhooksupport.MustPost(webhookHandler))

	go func() {
		var err error

		if cfg.certPath == "" || cfg.keyPath == "" {
			klog.InfoS("Starting INSECURE webhook server over HTTP", "addr", srv.Addr)

			err = srv.ListenAndServe()
		} else {
			klog.InfoS(
				"Starting webhook server over HTTPS",
				"addr",
				srv.Addr,
				"cert_path",
				cfg.certPath,
				"key_path",
				cfg.keyPath,
			)

			err = srv.ListenAndServeTLS(cfg.certPath, cfg.keyPath)
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			// If the server fails to serve, we need to stop.
			serveFailed <- err
		}
	}()

	select {
	case err := <-serveFailed:
		klog.ErrorS(err, "Server exited reporting an error")
		return err
	case <-ctx.Done():
		klog.Info("Main context exited, gracefully stoping server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		klog.ErrorS(err, "shutdown reported an error, closing the server")

		_ = srv.Close()
	}

	klog.Info("Webhook server exited")

	return nil
}
