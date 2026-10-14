package futures

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/euler-gaozl/agent-os-sdk/contracts"
	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func TestLongResearchInputPreserved(t *testing.T) {
	question := strings.Repeat("供给库存资料。", 250000) + "末尾核对标记"
	token := strings.Repeat("s", 32)
	calls := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/apps/futures/runs" || r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("Idempotency-Key") != "large-input" {
			t.Error("incorrect task scope or identity")
		}
		var body struct {
			Task     string            `json:"task"`
			Agent    contracts.Ref     `json:"agent"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		want := "品种：豆粕\n资料时间范围：2026年\n用户问题：" + question + reportInstructions("2026-10-09")
		if body.Task != want || body.Agent.ID != "published" || body.Agent.Revision != 15 || body.Metadata["as_of"] != "2026-10-09" {
			t.Error("task input or metadata changed")
		}
		w.WriteHeader(201)
		io.WriteString(w, `{"id":"long-input"}`)
	}))
	defer peer.Close()
	s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: token}}
	mux := http.NewServeMux()
	s.Routes(mux)
	body := contracts.JSON(map[string]any{"agent": contracts.Ref{ID: "published", Revision: 15}, "commodity": "豆粕", "period": "2026年", "as_of": "2026-10-09", "question": question})
	r := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(string(body)))
	r.Header.Set("Idempotency-Key", "large-input")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 201 || calls != 1 {
		t.Fatal(w.Code, calls, w.Body.String())
	}
	if len(question) <= 4<<20 {
		t.Fatal("fixture did not cross old input transport")
	}
}

func TestLongTaskResultAndReportCheckPreserveRawHash(t *testing.T) {
	result := strings.Repeat("库存原文。", 650000) + "最后一段"
	if len(result) <= 9<<20 {
		t.Fatal("fixture too small")
	}
	token := strings.Repeat("s", 32)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/apps/futures/runs/long-result" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("scope or identity lost")
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "long-result", "status": "completed", "result": result, "metadata": map[string]string{"commodity": "豆粕", "as_of": "2026-10-09", "report_format": reportSchema}})
	}))
	defer peer.Close()
	s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: token}}
	mux := http.NewServeMux()
	s.Routes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/long-result", nil))
	var task map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || task["result"] != result || task["commodity"] != "豆粕" || task["metadata"] != nil {
		t.Fatal("detail truncated or projection changed", w.Code)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/long-result/report-check", nil))
	var checked ReportCheck
	if err := json.Unmarshal(w.Body.Bytes(), &checked); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(result))
	if w.Code != 200 || checked.ResultSHA256 != hex.EncodeToString(hash[:]) || len(checked.Issues) == 0 {
		t.Fatal("bounded checker lost original result identity", w.Code, checked.Status)
	}
}

func TestResearchRequestRejectsMalformedOrTransportOverflowBeforeForward(t *testing.T) {
	calls := 0
	peer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer peer.Close()
	s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL}}
	mux := http.NewServeMux()
	s.Routes(mux)
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unknown", `{"commodity":"豆粕","unknown":1}`, 400},
		{"extra", `{"commodity":"豆粕"} {}`, 400},
		{"large task", `{"commodity":"豆粕","question":"` + strings.Repeat("x", taskTextBytes) + `"}`, 413},
		{"large body", `{"commodity":"豆粕","question":"` + strings.Repeat("x", int(taskRequestBytes)) + `"}`, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(tc.body))
			r.Header.Set("Idempotency-Key", "key")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	if calls != 0 {
		t.Fatal("rejected input reached Runtime")
	}
}

func TestTaskDetailDoesNotHideUpstreamFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		incomplete bool
		want       int
	}{
		{"missing", 404, `{"secret":"not forwarded"}`, false, 404},
		{"wrong identity", 403, `{}`, false, 403},
		{"malformed", 200, `{"result":`, false, 502},
		{"incomplete", 200, `{"result":"short"}`, true, 502},
		{"no content", 204, ``, false, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.incomplete {
					w.Header().Set("Content-Length", fmt.Sprint(len(tc.body)+100))
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer peer.Close()
			s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL}}
			mux := http.NewServeMux()
			s.Routes(mux)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/task", nil))
			if w.Code != tc.want || strings.Contains(w.Body.String(), "not forwarded") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
