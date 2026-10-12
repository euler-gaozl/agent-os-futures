package futures

import (
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSeparateUserIdentity(t *testing.T) {
	service := strings.Repeat("s", 32)
	user := strings.Repeat("u", 32)
	for _, token := range []string{"", "short", service} {
		if ValidateUserToken(servicehttp.Config{Token: service, AdminToken: token, DevLocal: true}) == nil {
			t.Fatal("unsafe token accepted")
		}
	}
	if err := ValidateUserToken(servicehttp.Config{Token: service, AdminToken: user}); err != nil {
		t.Fatal(err)
	}
}
func TestWebBoundaryAndLoopbackProtection(t *testing.T) {
	calls := 0
	token := strings.Repeat("u", 32)
	handler := servicehttp.New(servicehttp.Config{Role: "futures", Token: strings.Repeat("s", 32), AdminToken: token, UserFacing: true, PublicPath: PublicPath, DevLocal: true}, nil, func(m *http.ServeMux) {
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) })
	})
	for _, tc := range []struct {
		name, method, path, peer, host, origin, client, token string
		want                                                  int
	}{
		{name: "static public", method: "GET", path: "/assets/app.js", peer: "203.0.113.1:1", host: "example.test", want: 204},
		{name: "remote API", method: "GET", path: "/api/agents", peer: "203.0.113.1:1", host: "127.0.0.1", want: 401},
		{name: "loopback API", method: "GET", path: "/api/agents", peer: "127.0.0.1:1", host: "127.0.0.1", want: 204},
		{name: "foreign host", method: "GET", path: "/api/agents", peer: "127.0.0.1:1", host: "attacker.test", want: 401},
		{name: "foreign origin", method: "GET", path: "/api/agents", peer: "127.0.0.1:1", host: "127.0.0.1", origin: "https://attacker.test", want: 401},
		{name: "unmarked write", method: "POST", path: "/api/tasks", peer: "127.0.0.1:1", host: "127.0.0.1", want: 401},
		{name: "marked same origin write", method: "POST", path: "/api/tasks", peer: "127.0.0.1:1", host: "127.0.0.1", origin: "http://127.0.0.1", client: "web", want: 204},
		{name: "asset POST protected", method: "POST", path: "/assets/app.js", peer: "203.0.113.1:1", host: "example.test", want: 401},
		{name: "bare API protected", method: "GET", path: "/api", peer: "203.0.113.1:1", host: "example.test", want: 401},
		{name: "metrics protected", method: "GET", path: "/metrics", peer: "203.0.113.1:1", host: "example.test", want: 401},
		{name: "ready protected", method: "GET", path: "/readyz", peer: "203.0.113.1:1", host: "example.test", want: 401},
		{name: "token API", method: "GET", path: "/api/agents", peer: "203.0.113.1:1", host: "example.test", token: token, want: 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
			req.RemoteAddr = tc.peer
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("X-AgentOS-Client", tc.client)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			before := calls
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, req)
			if out.Code != tc.want {
				t.Fatalf("status %d, want %d", out.Code, tc.want)
			}
			if tc.want == 401 && calls != before {
				t.Fatal("unauthorized request reached routes")
			}
		})
	}
}
