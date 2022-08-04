package webhooksupport

import (
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
)

// WriteJSON writes a JSON response to an HTTP request.
func WriteJSON(rw http.ResponseWriter, statusCode int, payload any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(statusCode)

	if err := json.NewEncoder(rw).Encode(payload); err != nil {
		klog.ErrorS(err, "Can't write JSON payload")
		return
	}
}
