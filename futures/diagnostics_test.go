package futures

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

// diagnosticTestServer provides the peer-backed Server plus its mux.
func diagnosticTestServer(t *testing.T, handler http.HandlerFunc) (*Server, *http.ServeMux, func()) {
	t.Helper()
	peer := httptest.NewServer(handler)
	s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: "fixture-token"}}
	mux := http.NewServeMux()
	s.Routes(mux)
	return s, mux, func() { peer.Close() }
}

func TestDiagnosticsBoundarySanitizesAndFallsBackSafely(t *testing.T) {
	run := strings.Repeat("a", 64)
	_, mux, closePeer := diagnosticTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/apps/futures/runs/" + run + "/diagnostics":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"protocol": taskDiagnosticsProtocol, "run_id": run, "status": "failed",
				"stage": "tool", "code": "parameter_rejected",
				"failure_version": "fvd1:" + strings.Repeat("a", 64),
				"reported_at":     "2026-10-20T12:00:00Z",
				"safe_message":    "PRIVATE raw exception https://provider.invalid",
				"cause_confirmed": true, "related_event_ids": []int64{24, -3, 25},
				"operation_id": run + "/dsh/14", "related_role": "tool", "tool": "kb_search",
				"safe_parameter":     map[string]string{"name": "query", "value": "豆"},
				"recommended_action": "correct_input", "retry_policy": "manual_after_correction",
				"usage":    map[string]any{"attempts": 2, "unknown_usage_attempts": 0, "prompt_tokens": 23667, "completion_tokens": 218, "token_budget": -5},
				"messages": []string{"PRIVATE model content"}, "error": "PRIVATE raw error",
			})
		case "/v1/apps/futures/runs/missing/diagnostics":
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"PRIVATE upstream detail"}`))
		case "/v1/apps/futures/runs/broken/diagnostics":
			w.WriteHeader(503)
			w.Write([]byte(`{"error":"PRIVATE upstream outage"}`))
		case "/v1/apps/futures/runs/wrong/diagnostics":
			_ = json.NewEncoder(w).Encode(map[string]any{"protocol": "other", "run_id": "someone-else"})
		case "/v1/apps/futures/run-failures":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"protocol": taskFailureProtocol, "app": "futures",
				"items": []map[string]any{
					{"run_id": run, "status": "failed", "created_at": "2026-10-20T22:06:57+08:00", "commodity": "豆粕", "period": "2026-07-20", "stage": "tool", "code": "parameter_rejected", "cause_confirmed": true, "operation_id": run + "/dsh/14", "tool": "kb_search", "failure_version": "fvd1:" + strings.Repeat("b", 64)},
					{"run_id": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "status": "failed", "created_at": "2026-10-20T22:06:57+08:00", "commodity": "x", "stage": "tool", "code": "unknown", "cause_confirmed": false},
					{"run_id": "second", "status": "completed", "created_at": "2026-10-20T22:06:57+08:00", "stage": "tool", "code": "unknown", "cause_confirmed": false},
					{"run_id": "third", "status": "failed", "created_at": "not-a-time", "stage": "bogus", "code": "bogus", "cause_confirmed": false},
				},
				"next_cursor": "cursor-value", "has_more": true,
				"coverage": map[string]any{"since": "2026-07-20T00:00:00Z", "until": "2026-10-20T00:00:00Z", "scanned": -1, "scan_limit": 200, "truncated": true, "complete": false},
			})
		default:
			t.Error("unexpected path", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	defer closePeer()
	do := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := do("/api/tasks/" + run + "/diagnostics")
	body := w.Body.String()
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, body)
	}
	if strings.Contains(body, "PRIVATE") || strings.Contains(body, "provider.invalid") || strings.Contains(body, "messages") {
		t.Fatal("raw runtime data crossed the BFF", body)
	}
	var diag taskDiagnostics
	if err := json.Unmarshal([]byte(body), &diag); err != nil {
		t.Fatal(err)
	}
	if !diag.Available || diag.Code != "parameter_rejected" || diag.SafeMessage != diagnosticMessages["parameter_rejected"] ||
		diag.OperationID != run+"/dsh/14" || diag.SafeParameter == nil || diag.SafeParameter.Value != "豆" ||
		len(diag.RelatedEventIDs) != 2 || diag.Usage == nil || diag.Usage.TokenBudget != 0 {
		t.Fatal("sanitized projection lost valid fields", diag)
	}
	if w := do("/api/tasks/missing/diagnostics"); w.Code != 200 || !strings.Contains(w.Body.String(), `"available":false`) || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal("old Runtime fallback", w.Code, w.Body.String())
	}
	if w := do("/api/tasks/broken/diagnostics"); w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal("runtime failure", w.Code, w.Body.String())
	}
	if w := do("/api/tasks/wrong/diagnostics"); w.Code != 502 || strings.Contains(w.Body.String(), "someone-else") {
		t.Fatal("protocol mismatch", w.Code, w.Body.String())
	}
	w = do("/api/task-failures")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal(w.Code, w.Body.String())
	}
	var page taskFailurePage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].RunID != run || page.Items[0].CreatedAt == "" || !page.HasMore || page.Coverage.Scanned != 0 {
		t.Fatal("list sanitization", page)
	}
	for _, query := range []string{
		"?status=completed", "?category=bogus", "?stage=bogus", "?limit=51", "?limit=0",
		"?since=bad", "?unknown=1", "?limit=2&limit=3", "?commodity=" + strings.Repeat("a", 101),
	} {
		if w := do("/api/task-failures" + query); w.Code != 400 {
			t.Fatal(query, w.Code, w.Body.String())
		}
	}
}

func TestTaskFailureQueryForwardsOnlyValidatedFilters(t *testing.T) {
	run := strings.Repeat("a", 64)
	var received string
	_, mux, closePeer := diagnosticTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		received = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"protocol": taskFailureProtocol, "app": "futures", "items": []any{}, "next_cursor": "", "has_more": false,
			"coverage": map[string]any{"since": "2026-07-20T00:00:00Z", "until": "2026-10-20T00:00:00Z", "scanned": 0, "scan_limit": 200, "truncated": false, "complete": true},
		})
	})
	defer closePeer()
	q := url.Values{"since": {"2026-07-20T00:00:00Z"}, "until": {"2026-10-20T00:00:00Z"}, "status": {"failed"}, "category": {"parameter_rejected"}, "stage": {"tool"}, "commodity": {"豆粕"}, "limit": {"25"}}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/task-failures?"+q.Encode(), nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(received, "commodity=%E8%B1%86%E7%B2%95") || !strings.Contains(received, "category=parameter_rejected") {
		t.Fatal("filters not forwarded", received)
	}
	if strings.Contains(received, "PRIVATE") {
		t.Fatal(received)
	}
	_ = run
}

func TestProjectNeverForwardsRawRuntimeErrors(t *testing.T) {
	cases := map[string]string{
		`service HTTP 400: {"error":"query requires at least two adjacent Han characters"}`: "upstream request rejected",
		"model HTTP 503": "model HTTP 503",
		"deepseek credential missing for key DEEPSEEK_API_KEY": "credential unavailable",
		"model token budget exhausted (accounted=1)":           "model token budget exhausted",
		"panic: database password rotated":                     "execution failed",
		"":                                                     "",
	}
	for raw, want := range cases {
		row := map[string]any{"id": "t", "error": raw}
		project(row)
		if row["error"] != want {
			t.Fatalf("project(%q)=%v want %s", raw, row["error"], want)
		}
	}
}
