package futures

// Business-only draft feedback. This endpoint has no task ID, outbound request,
// database, model, or write effect. Tool intent/results are retained by Runtime.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/euler-gaozl/agent-os-sdk/contracts"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
)

var researchTools = []contracts.Tool{{
	Name:        "check_research_draft_v2",
	Description: "检查期货研究草稿的字段、日期、比较口径、问题文字和非数值原文说明。传入完整 futures-evidence-v1 JSON 字符串及任务固定 as_of。structuredContent.ready 只表示结构允许交付，不证明引用真实、研究完整或事实正确。根据 revise 修订，source_review 需查原文，保留关键报数和未决问题。配置了原文交付策略时，用返回的已提交回执调用 final_output_commit，勿再次复制或改写送检稿。",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"draft":{"type":"string","minLength":2,"maxLength":98304},"as_of":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}},"required":["draft","as_of"],"additionalProperties":false}`),
}}

type draftFeedback struct {
	Validator       string       `json:"validator"`
	DraftSHA256     string       `json:"draft_sha256"`
	Phase           string       `json:"phase"`
	Status          string       `json:"status"`
	Notice          string       `json:"notice"`
	Revise          []draftIssue `json:"revise"`
	SourceReview    []draftIssue `json:"source_review"`
	TotalIssues     int          `json:"total_issues"`
	IssuesTruncated bool         `json:"issues_truncated"`
}

// Identical explanations of an issue code are shared. Keep each affected path,
// but do not make the Agent page through an archive of duplicate explanations.
type draftIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func draftMCPResult(feedback draftFeedback) map[string]any {
	body, _ := json.Marshal(feedback)
	// Status is classified BEFORE bounded feedback truncation. Never infer
	// readiness from an emptied/truncated revise list, or from source warnings.
	return map[string]any{"content": []map[string]string{{"type": "text", "text": string(body)}}, "isError": false,
		"structuredContent": map[string]any{"protocol": "futures-draft-readiness-v1", "ready": feedback.Phase == "draft_only" && feedback.Status == "source_review_required" && len(feedback.Revise) == 0,
			"scope": "structure_only", "source_review_required": true, "draft_sha256": feedback.DraftSHA256}}
}

func reviewDraft(ctx context.Context, raw, asOf string) draftFeedback {
	check := checkReport(ctx, raw, asOf, nil)
	out := draftFeedback{Validator: check.Validator, DraftSHA256: check.ResultSHA256, Phase: "draft_only", Status: "source_review_required",
		Notice: "仅检查草稿，未核对任务日期、回执、来源真实性或覆盖。同 code 说明相同才省略重复 message；省略时复用首项说明。查原文处理 source_review；不要改引文或删关键事实换取通过。最终仍需留存引用核对。",
		Revise: []draftIssue{}, SourceReview: []draftIssue{}, TotalIssues: len(check.Issues)}
	explanations := map[string]string{}
	mixed := map[string]bool{}
	for _, issue := range check.Issues {
		if previous, ok := explanations[issue.Code]; ok && previous != issue.Message {
			mixed[issue.Code] = true
		}
		explanations[issue.Code] = issue.Message
	}
	seen := map[string]bool{}
	for _, issue := range check.Issues {
		item := draftIssue{Path: issue.Path, Code: issue.Code}
		if !seen[issue.Code] || mixed[issue.Code] {
			item.Message = issue.Message
			seen[issue.Code] = true
		}
		switch issue.Code {
		case "anchor", "period_anchor", "estimate_anchor", "plan_anchor", "comparability", "retained_unchecked", "ai_summary", "evidence_unverified", "period_inferred", "period_unknown", "source_limitations", "finding_review", "statement_ai_summary", "statement_unverified", "statement_limitations", "statement_review", "quote_normalized", "receipt_budget", "finding_period_review":
			out.SourceReview = append(out.SourceReview, item)
		default:
			out.Revise = append(out.Revise, item)
		}
	}
	if len(out.Revise) > 0 || out.IssuesTruncated {
		out.Status = "revision_required"
	}
	// Runtime archives tool results above 6000 bytes. Count the actual MCP
	// result including escaped JSON text, with room for transport formatting.
	// This does not change the immutable historical tool receipts or schemas.
	for {
		body, _ := json.Marshal(draftMCPResult(out))
		if len(body) <= 5900 {
			break
		}
		out.IssuesTruncated = true
		if len(out.SourceReview) > 0 {
			out.SourceReview = out.SourceReview[:len(out.SourceReview)-1]
		} else if len(out.Revise) > 0 {
			out.Revise = out.Revise[:len(out.Revise)-1]
		} else {
			break
		}
	}
	return out
}

func (s *Server) researchMCPHandler() http.Handler {
	// Never inherit the Web's user token, loopback exemption or public assets.
	c := servicehttp.Config{Role: "futures-research", Peers: map[string]string{}, MaxConcurrent: 2, RequestTimeout: 5 * time.Second,
		Authorize: func(caller, method, path string) bool {
			return caller == "futures" && method == "POST" && path == "/mcp/research"
		}}
	if len(s.HTTP.Token) >= 32 {
		c.Peers["futures"] = servicehttp.TokenHash(s.HTTP.Token)
	}
	return servicehttp.New(c, nil, func(mux *http.ServeMux) { mux.HandleFunc("POST /mcp/research", researchRPC) })
}

func strictJSON(raw []byte, out any) error {
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func researchRPC(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if r.URL.RawQuery != "" {
		servicehttp.JSONResponse(w, 400, map[string]string{"error": "query parameters are not supported"})
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
	if err != nil || strictJSON(raw, &in) != nil || in.Version != "2.0" || len(in.ID) > 128 {
		servicehttp.JSONResponse(w, 400, map[string]string{"error": "invalid or oversized MCP request"})
		return
	}
	if len(in.ID) == 0 {
		if in.Method == "notifications/initialized" {
			w.WriteHeader(202)
		} else {
			w.WriteHeader(400)
		}
		return
	}
	var id any
	if json.Unmarshal(in.ID, &id) != nil {
		w.WriteHeader(400)
		return
	}
	switch id.(type) {
	case string, float64:
	default:
		w.WriteHeader(400)
		return
	}
	respond := func(result any, code int, message string) {
		out := map[string]any{"jsonrpc": "2.0", "id": in.ID}
		if code != 0 {
			out["error"] = map[string]any{"code": code, "message": message}
		} else {
			out["result"] = result
		}
		servicehttp.JSONResponse(w, 200, out)
	}
	switch in.Method {
	case "initialize":
		respond(map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]bool{"listChanged": false}}, "serverInfo": map[string]string{"name": "agent-os-futures-research", "version": "1.0.0"}}, 0, "")
	case "tools/list":
		respond(map[string]any{"tools": researchTools}, 0, "")
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		var a struct {
			Draft string `json:"draft"`
			AsOf  string `json:"as_of"`
		}
		err := strictJSON(in.Params, &p)
		if err == nil && p.Name != researchTools[0].Name {
			err = errors.New("unknown tool")
		}
		if err == nil {
			err = strictJSON(p.Arguments, &a)
		}
		_, dateOK := validDate(a.AsOf)
		if err != nil || len(a.Draft) < 2 || len(a.Draft) > 96<<10 || !dateOK {
			respond(nil, -32602, "expected bounded draft and valid as_of; task IDs, URLs and extra fields are not accepted")
			return
		}
		feedback := reviewDraft(r.Context(), a.Draft, a.AsOf)
		respond(draftMCPResult(feedback), 0, "")
	default:
		respond(nil, -32601, "method not found")
	}
}
