package futures

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func TestFinalDraftComparisonUsesLatestRootReceiptWithoutWriting(t *testing.T) {
	for _, mode := range []string{"matched", "changed", "whitespace", "not_found", "unavailable", "bad_hash", "bad_count", "child", "empty_cursor", "failed", "wrong_task"} {
		t.Run(mode, func(t *testing.T) {
			raw := "原始回答🙂\n"
			item := draftReceipt{OperationID: "task/draft", RunID: "task", RecordedAt: "2026-10-17T04:00:00Z", InputSHA256: textHash(raw), InputChars: utf8.RuneCountInString(raw), Version: strings.Repeat("a", 64)}
			switch mode {
			case "changed":
				item.InputSHA256 = textHash("先前草稿")
			case "whitespace":
				item.InputSHA256 = textHash(strings.TrimSpace(raw))
			case "bad_hash":
				item.InputSHA256 = "not-a-hash"
			case "bad_count":
				item.InputChars++
			case "child":
				item.RunID = "child"
			}
			calls := 0
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("mutation or missing service identity")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/apps/futures/runs/task":
					id, status := "task", "completed"
					if mode == "wrong_task" {
						id = "other"
					}
					if mode == "failed" {
						status = "failed"
					}
					json.NewEncoder(w).Encode(map[string]string{"id": id, "status": status, "result": raw})
				case "/v1/apps/futures/runs/task/receipts/" + draftProfile:
					if r.URL.RawQuery != "limit=1" {
						t.Error("must select latest only")
					}
					if mode == "unavailable" {
						http.Error(w, "private upstream details", 503)
						return
					}
					items, cursor := []draftReceipt{item}, "older-receipt"
					if mode == "not_found" {
						items, cursor = nil, ""
					}
					if mode == "empty_cursor" {
						items = nil
					}
					json.NewEncoder(w).Encode(map[string]any{"items": items, "next_cursor": cursor})
				default:
					t.Error("unscoped request", r.URL)
				}
			}))
			defer peer.Close()
			mux := http.NewServeMux()
			(&Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: "fixture-token"}}).Routes(mux)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/tasks/task/draft-match", nil))
			if mode == "wrong_task" {
				if response.Code != 502 || calls != 1 {
					t.Fatal(response.Code, calls)
				}
				return
			}
			var got DraftMatch
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || response.Code != 200 {
				t.Fatal(response.Code, err)
			}
			want := mode
			switch mode {
			case "whitespace":
				want = "changed"
			case "bad_hash", "bad_count", "child", "empty_cursor":
				want = "unavailable"
			case "failed":
				want = "not_final"
			}
			if got.Status != want || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private upstream") {
				t.Fatal(got, response.Body.String())
			}
			if mode == "failed" {
				if calls != 1 || got.ResultSHA256 != "" {
					t.Fatal("read drafts for non-final task")
				}
			} else if calls != 2 || got.ResultSHA256 != textHash(raw) {
				t.Fatal("lost original result identity", calls, got)
			}
		})
	}
}
