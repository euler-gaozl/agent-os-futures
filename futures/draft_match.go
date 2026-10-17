package futures

import (
	"context"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
)

// DraftMatch compares retained bytes, independently of content quality. It never
// replaces the model result or turns an unavailable receipt into a match.
type DraftMatch struct {
	Status        string `json:"status"`
	ResultSHA256  string `json:"result_sha256,omitempty"`
	CheckedSHA256 string `json:"checked_sha256,omitempty"`
	OperationID   string `json:"operation_id,omitempty"`
	Version       string `json:"version,omitempty"`
}

func (s *Server) draftMatchRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks/{id}/draft-match", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		if !reportID.MatchString(id) || r.URL.RawQuery != "" {
			return &servicehttp.StatusError{Status: 400, Message: "invalid draft comparison request"}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var task struct {
			ID, Status, Result string
		}
		if err := s.readTask(ctx, id, &task); err != nil {
			return err
		}
		if task.ID != id {
			return &servicehttp.StatusError{Status: 502, Message: "task identity mismatch"}
		}
		out := DraftMatch{Status: "not_final"}
		if task.Status == "completed" {
			out.ResultSHA256 = textHash(task.Result)
			out.Status = "unavailable"
			var page struct {
				Items []draftReceipt `json:"items"`
				Next  string         `json:"next_cursor"`
			}
			// Runtime orders completed, app-scoped receipts newest first. Request
			// one: never use an older matching draft to hide a changed latest one.
			endpoint := s.Client.RuntimeURL + "/v1/apps/futures/runs/" + url.PathEscape(id) + "/receipts/" + draftProfile + "?limit=1"
			if err := s.Client.Request(ctx, "GET", endpoint, nil, &page); err == nil {
				if len(page.Items) == 0 && page.Next == "" {
					out.Status = "not_found"
				} else if len(page.Items) == 1 {
					item := page.Items[0]
					_, dateErr := time.Parse(time.RFC3339Nano, item.RecordedAt)
					// Child drafts are not proof of the root's final check. Keep
					// this ambiguous case explicit instead of comparing unlike work.
					if item.RunID == id && item.OperationID != "" && len(item.OperationID) <= 512 && dateErr == nil &&
						receiptHash.MatchString(item.InputSHA256) && receiptHash.MatchString(item.Version) && item.InputChars >= 0 {
						out.Status = "changed"
						out.CheckedSHA256, out.OperationID, out.Version = item.InputSHA256, item.OperationID, item.Version
						if out.ResultSHA256 == item.InputSHA256 {
							out.Status = "unavailable"
							if utf8.RuneCountInString(task.Result) == item.InputChars {
								out.Status = "matched"
							}
						}
					}
				}
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		servicehttp.JSONResponse(w, 200, out)
		return nil
	}))
}
