package futures

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/euler-gaozl/agent-os-sdk/contracts"
	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func TestRevisionIncludesVerifiedSourceAndUsesApplicationBoundary(t *testing.T) {
	digest := strings.Repeat("a", 64)
	source := `{"schema":"futures-evidence-v1","facts":[]}`
	submitted := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("service identity missing")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/apps/futures/runs/source-task":
			json.NewEncoder(w).Encode(map[string]any{"id": "source-task", "result": source, "result_sha256": digest})
		case "POST /v1/apps/futures/runs":
			var body map[string]any
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Error(e)
			}
			if body["evidence_source_run"] != "source-task" || body["expected_source_result_sha256"] != digest || !strings.Contains(body["task"].(string), source) || !strings.Contains(body["task"].(string), "核对差异") {
				t.Error("revision lost source/input")
			}
			submitted++
			w.WriteHeader(201)
			w.Write([]byte(`{"id":"new-task"}`))
		case "POST /v1/apps/futures/runs/new-task/evidence-binding":
			w.Write([]byte(`{"mode":"retrospective"}`))
		default:
			t.Error("unexpected boundary", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer peer.Close()
	s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: "fixture-token"}}
	mux := http.NewServeMux()
	s.Routes(mux)
	request := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(string(contracts.JSON(map[string]any{"agent": contracts.Ref{ID: "published", Revision: 1}, "commodity": "豆粕", "question": "核对差异", "as_of": "2026-10-09", "evidence_source_run": "source-task"}))))
	request.Header.Set("Idempotency-Key", "revision-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, request)
	if w.Code != 201 || submitted != 1 {
		t.Fatal(w.Code, w.Body.String(), submitted)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/tasks/new-task/evidence-binding", strings.NewReader(`{"source_run_id":"source-task"}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "retrospective") {
		t.Fatal(w.Code, w.Body.String())
	}
}
