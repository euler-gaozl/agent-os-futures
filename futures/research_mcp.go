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
	Description: "检查期货研究草稿的字段、日期、比较口径和问题文字。传入完整 futures-evidence-v1 JSON 字符串及任务固定 as_of。不会读取任务原文，结果不能证明引用真实或研究完成。根据 revise 修订，source_review 需查原文；最多两轮修订，保留关键报数和未决问题。",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"draft":{"type":"string","minLength":2,"maxLength":98304},"as_of":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}},"required":["draft","as_of"],"additionalProperties":false}`),
}}

type draftFeedback struct {
	Validator       string        `json:"validator"`
	DraftSHA256     string        `json:"draft_sha256"`
	Phase           string        `json:"phase"`
	Status          string        `json:"status"`
	Notice          string        `json:"notice"`
	Revise          []ReportIssue `json:"revise"`
	SourceReview    []ReportIssue `json:"source_review"`
	TotalIssues     int           `json:"total_issues"`
	IssuesTruncated bool          `json:"issues_truncated"`
}

func reviewDraft(ctx context.Context, raw, asOf string) draftFeedback {
	check := checkReport(ctx, raw, asOf, nil)
	out := draftFeedback{Validator: check.Validator, DraftSHA256: check.ResultSHA256, Phase: "draft_only", Status: "source_review_required",
		Notice: "仅检查传入草稿；未核对任务固定日期、阅读回执、原文真实性或资料覆盖。不要修改引用迎合规则或删掉重要事实；查原文处理 source_review。最终结果仍需留存引用核对。",
		Revise: []ReportIssue{}, SourceReview: []ReportIssue{}, TotalIssues: len(check.Issues)}
	for _, issue := range check.Issues {
		switch issue.Code {
		case "anchor", "period_anchor", "estimate_anchor", "plan_anchor", "comparability", "retained_unchecked":
			out.SourceReview = append(out.SourceReview, issue)
		default:
			out.Revise = append(out.Revise, issue)
		}
	}
	if len(out.Revise) > 0 || out.IssuesTruncated {
		out.Status = "revision_required"
	}
	// Cap actual UTF-8 JSON bytes, not just number of issues. Keep repairable
	// issues first; the notice always makes missing receipt verification clear.
	for {
		body, _ := json.Marshal(out)
		if len(body) <= 8<<10 {
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
		body, _ := json.Marshal(feedback)
		respond(map[string]any{"content": []map[string]string{{"type": "text", "text": string(body)}}, "isError": false}, 0, "")
	default:
		respond(nil, -32601, "method not found")
	}
}
