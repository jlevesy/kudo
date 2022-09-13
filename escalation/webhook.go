package escalation

import (
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"

	"github.com/jlevesy/kudo/grant"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

func SetupWebhook(router *http.ServeMux, kudoInformerFactory kudoinformers.SharedInformerFactory, granterFactory grant.Factory) {
	reviewer := NewAdmissionReviewer(
		kudoInformerFactory.K8s().V1alpha1().EscalationPolicies().Lister(),
		granterFactory,
	)

	router.Handle(
		"/v1alpha1/escalations",
		webhooksupport.NewHandler(
			webhooksupport.RouteAdmissionRequests(
				webhooksupport.HandleOperation(
					admissionv1.Create, reviewer,
				),
			),
		),
	)
}
