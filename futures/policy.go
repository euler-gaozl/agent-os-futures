package futures

import (
	"errors"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"net/http"
	"strings"
)

// Only static GET/HEAD paths bypass the user token. No API or operational paths.
func PublicPath(r *http.Request) bool {
	return (r.Method == "GET" || r.Method == "HEAD") && r.URL.Path != "/api" && !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/mcp" && !strings.HasPrefix(r.URL.Path, "/mcp/") && r.URL.Path != "/readyz" && r.URL.Path != "/metrics"
}

// Validate after SDK token selection, including legacy BUSINESS_TOKEN -> ADMIN_TOKEN.
// Never allow its final fallback to the service identity, even in local mode.
func ValidateUserToken(c servicehttp.Config) error {
	if len(c.AdminToken) < 32 || c.AdminToken == c.Token {
		return errors.New("separate user access token of at least 32 bytes required; do not reuse SERVICE_TOKEN")
	}
	return nil
}
