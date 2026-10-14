package futures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/euler-gaozl/agent-os-futures/internal/webproxy"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
)

const draftProfile = "research-draft-v1"

type draftReceipt struct {
	OperationID  string `json:"operation_id"`
	RunID        string `json:"run_id"`
	RecordedAt   string `json:"recorded_at"`
	InputSHA256  string `json:"input_sha256"`
	OutputSHA256 string `json:"output_sha256"`
	InputChars   int    `json:"input_chars"`
	OutputChars  int    `json:"output_chars"`
	Version      string `json:"version"`
}
type draftPart struct {
	draftReceipt
	Field      string `json:"field"`
	Text       string `json:"text"`
	Offset     int    `json:"offset"`
	NextOffset *int   `json:"next_offset"`
}
type retainedDraft struct {
	draftReceipt
	Phase      string        `json:"phase"`
	TaskStatus string        `json:"task_status"`
	Raw        string        `json:"raw"`
	Feedback   draftFeedback `json:"feedback"`
	Check      ReportCheck   `json:"check"`
}

func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (s *Server) draftField(ctx context.Context, base, operation, version, field string) (string, draftReceipt, error) {
	var text strings.Builder
	var receipt draftReceipt
	offset := 0
	capacity := 96 << 10
	if field == "output" {
		capacity = 16 << 10
	}
	for page := 0; page < 8; page++ {
		q := url.Values{"operation_id": {operation}, "field": {field}, "version": {version}, "offset": {strconv.Itoa(offset)}, "limit": {"16384"}}
		var part draftPart
		if err := s.Client.Request(ctx, "GET", base+"/record?"+q.Encode(), nil, &part); err != nil {
			return "", receipt, err
		}
		if part.OperationID != operation || part.Version != version || part.Field != field || part.Offset != offset || !utf8.ValidString(part.Text) || utf8.RuneCountInString(part.Text) > 16384 {
			return "", receipt, errors.New("retained draft segment identity mismatch")
		}
		if page == 0 {
			receipt = part.draftReceipt
		} else if receipt != part.draftReceipt {
			return "", receipt, errors.New("retained draft metadata changed")
		}
		if text.Len()+len(part.Text) > capacity {
			return "", receipt, errors.New("retained draft exceeds supported tool capacity")
		}
		text.WriteString(part.Text)
		next := offset + utf8.RuneCountInString(part.Text)
		if part.NextOffset == nil {
			hash, chars := receipt.InputSHA256, receipt.InputChars
			if field == "output" {
				hash, chars = receipt.OutputSHA256, receipt.OutputChars
			}
			if next != chars || textHash(text.String()) != hash {
				return "", receipt, errors.New("retained draft content digest mismatch")
			}
			return text.String(), receipt, nil
		}
		if *part.NextOffset != next || next <= offset {
			return "", receipt, errors.New("retained draft cursor mismatch")
		}
		offset = next
	}
	return "", receipt, errors.New("retained draft pagination did not finish")
}

func (s *Server) draftRoutes(mux *http.ServeMux) {
	baseFor := func(id string) string { return s.Client.RuntimeURL + "/v1/apps/futures/runs/" + url.PathEscape(id) }
	mux.HandleFunc("GET /api/tasks/{id}/drafts", func(w http.ResponseWriter, r *http.Request) {
		if !reportID.MatchString(r.PathValue("id")) {
			servicehttp.JSONResponse(w, 400, map[string]string{"error": "invalid task"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		webproxy.Proxy(w, r, baseFor(r.PathValue("id"))+"/receipts/"+draftProfile, s.Client.Token)
	})
	mux.HandleFunc("GET /api/tasks/{id}/drafts/record", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || !reportID.MatchString(r.PathValue("id")) || len(q) != 2 || len(q["operation_id"]) != 1 || len(q["version"]) != 1 || q.Get("operation_id") == "" || len(q.Get("operation_id")) > 512 || len(q.Get("version")) != 64 || strings.Trim(q.Get("version"), "0123456789abcdef") != "" {
			return &servicehttp.StatusError{Status: 400, Message: "expected task draft operation and version"}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		var task struct {
			ID       string            `json:"id"`
			Status   string            `json:"status"`
			Metadata map[string]string `json:"metadata"`
		}
		if err = s.readTask(ctx, r.PathValue("id"), &task); err != nil {
			return err
		}
		if task.ID != r.PathValue("id") {
			return errors.New("task identity mismatch")
		}
		base := baseFor(task.ID)
		raw, receipt, err := s.draftField(ctx, base+"/receipts/"+draftProfile, q.Get("operation_id"), q.Get("version"), "input")
		if err != nil {
			return err
		}
		feedbackRaw, other, err := s.draftField(ctx, base+"/receipts/"+draftProfile, q.Get("operation_id"), q.Get("version"), "output")
		if err != nil {
			return err
		}
		if receipt != other {
			return errors.New("draft and feedback refer to different receipts")
		}
		var feedback draftFeedback
		if err = strictJSON([]byte(feedbackRaw), &feedback); err != nil || feedback.DraftSHA256 != receipt.InputSHA256 || feedback.Phase != "draft_only" || feedback.Validator == "" || (feedback.Status != "revision_required" && feedback.Status != "source_review_required") {
			return errors.New("retained feedback does not identify this draft")
		}
		// Historical empty arrays may be JSON null; keep the meaning (no
		// listed entries) while returning a stable renderable projection.
		if feedback.Revise == nil {
			feedback.Revise = []draftIssue{}
		}
		if feedback.SourceReview == nil {
			feedback.SourceReview = []draftIssue{}
		}
		asOf := task.Metadata["as_of"]
		if task.Metadata["report_format"] != reportSchema {
			asOf = ""
		}
		reader := &reportEvidenceReader{server: s, base: base, cache: map[string][]retainedPart{}}
		check := checkReport(ctx, raw, asOf, reader.quote)
		w.Header().Set("Cache-Control", "no-store")
		servicehttp.JSONResponse(w, 200, retainedDraft{draftReceipt: receipt, Phase: "retained_draft", TaskStatus: task.Status, Raw: raw, Feedback: feedback, Check: check})
		return nil
	}))
}
