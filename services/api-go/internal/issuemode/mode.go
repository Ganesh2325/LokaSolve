package issuemode

import (
	"net/http"

	"lokasolve/services/api-go/internal/platform/httpx"
)

// WritesClosed is set when issue writes have moved to the issue service.
var WritesClosed bool

func Blocked(w http.ResponseWriter, r *http.Request) bool {
	if !WritesClosed {
		return false
	}
	httpx.WriteError(w, r, httpx.New(http.StatusServiceUnavailable, "issue_unavailable", "Try again shortly."))
	return true
}
