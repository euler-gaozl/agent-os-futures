package futures

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func TestRetainedDraftPairsExactBytesAndKeepsFailedStatus(t *testing.T) {
	for _, mode := range []string{"valid", "wrong feedback hash", "changed version", "wrong cursor", "wrong body hash", "missing page", "foreign operation"} {
		t.Run(mode, func(t *testing.T) {
			raw := strings.Repeat("豆🙂\n", 9000)
			feedback := draftFeedback{Validator: "fixture-check-v1", DraftSHA256: textHash(raw), Phase: "draft_only", Status: "revision_required", Revise: []draftIssue{{Code: "fixture", Message: "needs review"}}}
			if mode == "wrong feedback hash" {
				feedback.DraftSHA256 = strings.Repeat("0", 64)
			}
			fb, _ := json.Marshal(feedback)
			version := strings.Repeat("a", 64)
			item := draftReceipt{OperationID: "fixture/draft", RunID: "fixture", RecordedAt: "2026-01-01T00:00:00Z", InputSHA256: textHash(raw), OutputSHA256: textHash(string(fb)), InputChars: len([]rune(raw)), OutputChars: len([]rune(string(fb))), Version: version}
			token := strings.Repeat("s", 32)
			pages := 0
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+token {
					t.Error("mutation or wrong service identity")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/apps/futures/runs/fixture" {
					json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "status": "failed", "result": "", "metadata": map[string]string{"as_of": "2026-10-09", "report_format": reportSchema}})
					return
				}
				if r.URL.Path != "/v1/apps/futures/runs/fixture/receipts/"+draftProfile+"/record" {
					t.Error("unscoped route", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if mode == "foreign operation" {
					http.NotFound(w, r)
					return
				}
				q := r.URL.Query()
				if q.Get("version") != version || q.Get("operation_id") != item.OperationID {
					t.Error("missing stable pairing")
				}
				field := q.Get("field")
				text := raw
				if field == "output" {
					text = string(fb)
				}
				offset, _ := strconv.Atoi(q.Get("offset"))
				chars := []rune(text)
				end := min(len(chars), offset+16384)
				p := draftPart{draftReceipt: item, Field: field, Offset: offset, Text: string(chars[offset:end])}
				if end < len(chars) {
					p.NextOffset = &end
				}
				pages++
				switch mode {
				case "changed version":
					if offset > 0 {
						p.Version = strings.Repeat("b", 64)
					}
				case "wrong cursor":
					if p.NextOffset != nil {
						x := end - 1
						p.NextOffset = &x
					}
				case "wrong body hash":
					p.InputSHA256 = strings.Repeat("0", 64)
				case "missing page":
					p.NextOffset = nil
				}
				json.NewEncoder(w).Encode(p)
			}))
			defer peer.Close()
			mux := http.NewServeMux()
			(&Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: token}}).Routes(mux)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/fixture/drafts/record?operation_id="+url.QueryEscape(item.OperationID)+"&version="+version, nil))
			if mode != "valid" {
				if w.Code == 200 {
					t.Fatal("accepted unpaired/incomplete draft", mode)
				}
				return
			}
			var got retainedDraft
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
				t.Fatal(w.Code, w.Body.String(), err)
			}
			if got.Raw != raw || got.TaskStatus != "failed" || got.Phase != "retained_draft" || got.Feedback.DraftSHA256 != item.InputSHA256 || got.Feedback.SourceReview == nil || pages < 3 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("incorrect retained draft projection")
			}
		})
	}
}

func TestDraftRouteDoesNotAcceptArbitraryProjection(t *testing.T) {
	requests := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/apps/futures/runs/fixture/receipts/"+draftProfile {
			t.Error("unexpected route")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[],"next_cursor":""}`))
	}))
	defer peer.Close()
	mux := http.NewServeMux()
	(&Server{Client: serviceclient.Client{RuntimeURL: peer.URL}}).Routes(mux)
	for _, suffix := range []string{"", "?operation_id=x", "?operation_id=x&version=short", "?operation_id=x&version=" + strings.Repeat("a", 64) + "&profile=other", "?operation_id=x&operation_id=y&version=" + strings.Repeat("a", 64)} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/fixture/drafts/record"+suffix, nil))
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if requests != 0 {
		t.Fatal("invalid user projection reached upstream")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/fixture/drafts", nil))
	if w.Code != 200 || requests != 1 {
		t.Fatal(w.Code, requests)
	}
}
