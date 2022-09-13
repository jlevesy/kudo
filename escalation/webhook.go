package escalation

import (
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jlevesy/kudo/grant"
	kudo "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	kudoinformers "github.com/jlevesy/kudo/pkg/generated/informers/externalversions"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

var (
	expectedKind = metav1.GroupVersionKind{
		Group:   kudo.GroupName,
		Version: kudov1alpha1.Version,
		Kind:    kudov1alpha1.KindEscalation,
	}
)

func SetupWebhook(router *http.ServeMux, kudoInformerFactory kudoinformers.SharedInformerFactory, granterFactory grant.Factory) {
	router.Handle(
		"/v1alpha1/escalations",
		webhooksupport.NewHandler(
			webhooksupport.RequireKind(
				expectedKind,
				webhooksupport.RouteByOperation(
					webhooksupport.HandleOperation(
						admissionv1.Create,
						NewCreateAdmissionReviewer(
							kudoInformerFactory.K8s().V1alpha1().EscalationPolicies().Lister(),
							granterFactory,
						),
					),
				),
			),
		),
	)
}
