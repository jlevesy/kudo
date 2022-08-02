package main

import (
	"net/http"
	"time"

	"k8s.io/klog/v2"
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

	klog.V(0).Info("Starting webhook handler on addr", srv.Addr)

	mux.HandleFunc("/v1alpha1/escalations", handleV1Alpha1Escalations)

	if err := srv.ListenAndServeTLS("/var/run/certs/tls.crt", "/var/run/certs/tls.key"); err != nil {
		klog.V(0).ErrorS(err, "Can't serve")
	}

	klog.V(0).Info("Webhook handler exited")
}

func handleV1Alpha1Escalations(rw http.ResponseWriter, r *http.Request) {
	klog.V(0).Info("RECEIVED A WEBHOOK WOOOWOOOO")

	rw.WriteHeader(http.StatusBadRequest)
}
