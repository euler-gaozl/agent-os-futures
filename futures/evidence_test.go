package futures

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func TestEvidenceProxyAppScopeAndErrors(t *testing.T) {
	token := strings.Repeat("s", 32)
	seen := []string{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing service authentication")
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/apps/futures/runs/fixture/evidence") {
			t.Error("incorrect app scope", r.URL.Path)
		}
		seen = append(seen, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/record") {
			w.WriteHeader(404)
			io.WriteString(w, `{"error":"document receipt not found in this task"}`)
			return
		}
		io.WriteString(w, `{"items":[],"next_cursor":""}`)
	}))
	defer peer.Close()
	s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: token}}
	mux := http.NewServeMux()
	s.Routes(mux)
	for _, tc := range []struct {
		path   string
		status int
	}{{"/api/tasks/fixture/sources?document_id=one&limit=20", 200}, {"/api/tasks/fixture/sources/record?operation_id=foreign%2Fread", 404}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Authorization", "Bearer user-fixture")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(seen) != 2 || seen[0] != "/v1/apps/futures/runs/fixture/evidence?document_id=one&limit=20" || seen[1] != "/v1/apps/futures/runs/fixture/evidence/record?operation_id=foreign%2Fread" {
		t.Fatal(seen)
	}
}
