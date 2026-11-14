package identitymode

import (
	"net/http"

	"lokasolve/services/api-go/internal/platform/httpx"
)

// WritesClosed is set when identity writes have moved to the identity service.
var WritesClosed bool

func Blocked(w http.ResponseWriter, r *http.Request) bool {
	if !WritesClosed {
		return false
	}
	httpx.WriteError(w, r, httpx.New(http.StatusServiceUnavailable, "identity_unavailable", "Try again shortly."))
	return true
}
