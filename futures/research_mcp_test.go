package futures

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
)

func TestResearchDraftExposesRealFailureClassesBeforeSourceCheck(t *testing.T) {
	r := comparisonReport()
	r.Facts[0].Metric = "9月开机率"
	r.Facts[0].Measure, r.Facts[0].Unit = "capacity", "%"
	r.Facts[1].Metric, r.Facts[1].Measure = "未执行合同", "stock"
	r.Facts[1].Nature = "estimate"
	r.Questions[0].Question = "到港计划为123万吨，与二十万吨的压榨是否一致？"
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, nil)
	for _, code := range []string{"period_in_label", "ratio_measure", "commitment_measure", "basis", "prose_quantity", "prose_nature", "retained_unchecked"} {
		if !issue(c, code) {
			t.Fatalf("missing %s: %+v", code, c.Issues)
		}
	}
	for _, f := range c.Facts {
		if f.Consistent || f.QuoteMatched || f.OperationID != "" {
			t.Fatal("draft invented source proof")
		}
	}
	if c.Comparisons[0].Difference != "" {
		t.Fatal("draft calculated unverified difference")
	}
	feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	if feedback.Status != "revision_required" || len(feedback.Revise) < 6 || len(feedback.SourceReview) == 0 {
		t.Fatal(feedback)
	}
}

func TestResearchDraftFeedbackNeverClaimsRetainedVerification(t *testing.T) {
	r := exampleReport()
	feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	if feedback.Status != "source_review_required" || len(feedback.Revise) != 0 || feedback.Phase != "draft_only" || len(feedback.DraftSHA256) != 64 {
		t.Fatal(feedback)
	}
	b, _ := json.Marshal(feedback)
	for _, leaked := range []string{r.Facts[0].Source.Quote, `"report":`, `"quote_matched":`, `"consistent":`} {
		if strings.Contains(string(b), leaked) {
			t.Fatalf("unneeded/unverified field returned: %s", leaked)
		}
	}
	for _, raw := range []string{`{"schema":"futures-evidence-v1","schema":"other"}`, `[]`, `null`} {
		if f := reviewDraft(context.Background(), raw, r.AsOf); f.Status != "revision_required" {
			t.Fatal(f)
		}
	}
}

func TestResearchMCPServiceAuthProtocolAndNoOutboundAccess(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer upstream.Close()
	service, user := strings.Repeat("s", 32), strings.Repeat("u", 32)
	s := Server{HTTP: servicehttp.Config{Token: service, AdminToken: user, UserFacing: true, PublicPath: PublicPath, DevLocal: true}, Client: serviceclient.Client{RuntimeURL: upstream.URL, RegistryURL: upstream.URL}}
	h := s.Handler()
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:8098"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", "http://127.0.0.1:8098")
		r.Header.Set("X-AgentOS-Client", "web")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, method := range []string{"GET", "HEAD", "POST"} {
		for _, token := range []string{"", user, strings.Repeat("x", 32)} {
			if w := request(method, "/mcp/research", token, `{}`); w.Code != 401 {
				t.Fatalf("Web bypass accepted: %s %d", method, w.Code)
			}
		}
	}
	if w := request("GET", "/mcp/research", service, `{}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/mcp/research/extra", service, `{}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, method := range []string{"initialize", "tools/list"} {
		w := request("POST", "/mcp/research", service, `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{}}`)
		if w.Code != 200 || strings.Contains(w.Body.String(), `"error"`) {
			t.Fatal(w.Body.String())
		}
	}
	if w := request("POST", "/mcp/research", service, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); w.Code != 202 {
		t.Fatal(w.Code)
	}
	r := exampleReport()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": researchTools[0].Name, "arguments": map[string]string{"draft": encodeReport(t, r), "as_of": r.AsOf}}})
	w := request("POST", "/mcp/research", service, string(body))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "source_review_required") || strings.Contains(w.Body.String(), `"error"`) {
		t.Fatal(w.Body.String())
	}
	for _, bad := range []string{
		strings.Replace(string(body), `"as_of":"2026-10-09"`, `"as_of":"2026-10-09","run_id":"foreign"`, 1),
		strings.Replace(string(body), `"as_of":"2026-10-09"`, `"as_of":"2026-10-09","as_of":"2025-01-01"`, 1),
		strings.Replace(string(body), researchTools[0].Name, "fetch_page", 1),
	} {
		w := request("POST", "/mcp/research", service, bad)
		if w.Code != 400 && !strings.Contains(w.Body.String(), `"error"`) {
			t.Fatal("bad arguments accepted")
		}
	}
	if w := request("POST", "/mcp/research?run_id=foreign", service, string(body)); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/mcp/research", service, strings.Repeat("x", 513<<10)); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if calls != 0 {
		t.Fatalf("draft tool made %d upstream calls", calls)
	}
}

func TestRatioDifferenceAndCommitmentDates(t *testing.T) {
	r := comparisonReport()
	for i := range r.Facts {
		f := &r.Facts[i]
		f.Metric, f.Measure, f.Unit = "开机率", "ratio", "%"
		f.Source.Quote = strings.NewReplacer("豆粕库存", "开机率", "万吨", "%").Replace(f.Source.Quote)
	}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if c.Status != "consistent" || c.Comparisons[0].Unit != "个百分点" || c.Comparisons[0].Difference != "40.05" {
		t.Fatal(c)
	}
	r.Facts[0].Metric, r.Facts[0].Measure, r.Facts[0].Unit = "未执行合同", "commitment", "万吨"
	r.Facts[0].Period.Start = "2026-06-01"
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "commitment_period") || c.Comparisons[0].Difference != "" {
		t.Fatal(c)
	}
}

func TestQuestionProseCannotHideUnboundQuantities(t *testing.T) {
	for _, text := range []string{"库存三十万吨是否正确？", "开机率百分之五十是否正确？", "10月预估880.5万吨，但11月900万吨？"} {
		r := exampleReport()
		r.Questions[0].Question = text
		if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, nil); !issue(c, "prose_quantity") {
			t.Fatal(text)
		}
	}
	for _, text := range []string{"2026年10月至12月的实际到港与预估有何差异？", "是否有一手资料可以独立核对？", "请核对以万吨为单位的原始序列"} {
		r := exampleReport()
		r.Questions[0].Question = text
		if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, nil); issue(c, "prose_quantity") {
			t.Fatalf("neutral inquiry rejected as quantity: %s", text)
		}
	}
}

func TestDraftFeedbackByteLimitAndInvalidIDs(t *testing.T) {
	r := exampleReport()
	r.Facts[0].ID = strings.Repeat("!", 80<<10)
	r.Facts[0].Measure, r.Facts[0].Nature, r.Facts[0].Metric = "", "", ""
	r.Questions[0].FactIDs = []string{}
	out := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	b, _ := json.Marshal(out)
	if len(b) > 8<<10 || strings.Contains(string(b), strings.Repeat("!", 100)) || !strings.Contains(string(b), "facts[0]") {
		t.Fatal("invalid ID amplified response")
	}
	r = exampleReport()
	for i := 0; i < 39; i++ {
		f := r.Facts[0]
		f.ID = strings.Repeat("x", 100) + string(rune('A'+i))
		f.Measure = "invalid"
		f.Source.Quote = "unmatched"
		r.Facts = append(r.Facts, f)
	}
	out = reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	b, _ = json.Marshal(out)
	if len(b) > 8<<10 || !out.IssuesTruncated || out.TotalIssues <= len(out.Revise)+len(out.SourceReview) {
		t.Fatal("dense feedback was not bounded with accurate counts")
	}
}
