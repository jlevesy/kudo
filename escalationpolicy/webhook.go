package escalationpolicy

import (
	"net/http"

	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

func SetupWebhook(router *http.ServeMux) {
	router.Handle(
		"/v1alpha1/escalationpolicies",
		webhooksupport.NewHandler(
			NewAdmissionReviewer(),
		),
	)
}
