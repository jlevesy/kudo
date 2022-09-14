package escalationpolicy

import (
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kudo "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev"
	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	"github.com/jlevesy/kudo/pkg/webhooksupport"
)

var (
	expectedKind = metav1.GroupVersionKind{
		Group:   kudo.GroupName,
		Version: kudov1alpha1.Version,
		Kind:    kudov1alpha1.KindEscalationPolicy,
	}
)

func SetupWebhook(router *http.ServeMux) {
	reviewer := NewAdmissionReviewer()

	router.Handle(
		"/v1alpha1/escalationpolicies",
		webhooksupport.NewHandler(
			webhooksupport.RequireKind(
				expectedKind,
				webhooksupport.RouteByOperation(
					webhooksupport.HandleOperation(admissionv1.Create, reviewer),
					webhooksupport.HandleOperation(admissionv1.Update, reviewer),
				),
			),
		),
	)
}
