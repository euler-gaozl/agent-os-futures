package futures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func exampleReport() ResearchReport {
	return ResearchReport{Schema: reportSchema, AsOf: "2026-10-09", Questions: []ReportQuestion{{ID: "q1", Question: "已披露的到港估计覆盖哪些月份？", FactIDs: []string{"f1"}, ComparisonIDs: []string{}, Missing: []string{"各月实际到港量及统计口径"}}}, Facts: []ReportFact{{ID: "f1", Metric: "大豆到港", Value: "1018.55", Unit: "万吨", Scope: "全国全样本油厂", Measure: "flow", Nature: "estimate", Period: ReportPeriod{"2026-09-01", "2026-09-30"}, Source: ReportSource{"doc-1", "2026年9月份，全国全样本油厂大豆到港预估1018.55万吨。", "2026年9月份"}}}, Comparisons: []ReportComparison{}}
}
func encodeReport(t *testing.T, r ResearchReport) string {
	t.Helper()
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func acceptedQuote(context.Context, ReportSource) (quoteProof, error) {
	return quoteProof{Operation: "task/read", Version: strings.Repeat("a", 64), Offset: 42}, nil
}
func issue(check ReportCheck, code string) bool {
	for _, i := range check.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestReportPastEstimateNeverBecomesFutureOrActual(t *testing.T) {
	r := exampleReport()
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if c.Status != "consistent" || c.Facts[0].Group != "已结束区间 · 第三方估计" || c.Facts[0].QuoteOffset != 42 || len(c.ResultSHA256) != 64 {
		t.Fatalf("%+v", c)
	}
	for _, tc := range []struct{ month, start, end, group string }{{"2026年10月", "2026-10-01", "2026-10-31", "覆盖基准日 · 第三方估计"}, {"2026年11月", "2026-11-01", "2026-11-30", "未来区间 · 第三方估计"}} {
		r = exampleReport()
		r.Facts[0].Source.Quote = strings.ReplaceAll(r.Facts[0].Source.Quote, "2026年9月份", tc.month)
		r.Facts[0].Source.PeriodText = tc.month
		r.Facts[0].Period = ReportPeriod{tc.start, tc.end}
		c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
		if c.Status != "consistent" || c.Facts[0].Group != tc.group {
			t.Fatalf("%+v", c)
		}
	}
	r.Facts[0].Nature = "reported"
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "future_actual") || !issue(c, "estimated_as_actual") {
		t.Fatal(c)
	}
}
func TestReportRejectsUnsupportedRelationships(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		change     func(*ResearchReport)
	}{
		{"swapped quantities", "anchor", func(r *ResearchReport) {
			f := &r.Facts[0]
			f.Metric = "豆粕库存"
			f.Value = "873"
			f.Source.Quote = "2026年9月份，全国全样本油厂大豆库存预估873万吨、豆粕库存预估124万吨。"
		}},
		{"numeric substring", "anchor", func(r *ResearchReport) { r.Facts[0].Value = "1018" }},
		{"swapped dates", "anchor", func(r *ResearchReport) {
			r.Facts[0].Source.Quote = "2026年9月份，全国全样本油厂大豆到港预估99万吨。2026年10月，大豆到港预估1018.55万吨。"
		}},
		{"yearless later month", "anchor", func(r *ResearchReport) {
			r.Facts[0].Source.Quote = "2026年9月份，全国全样本油厂大豆到港预估99万吨；10月份大豆到港预估1018.55万吨。"
		}},
		{"different subject in same quote", "anchor", func(r *ResearchReport) {
			r.Facts[0].Scope = "甲样本"
			r.Facts[0].Source.Quote = "2026年9月份，甲样本大豆到港预估99万吨；乙样本大豆到港预估1018.55万吨。"
		}},
		{"plan is not estimate", "estimate_anchor", func(r *ResearchReport) {
			r.Facts[0].Source.Quote = strings.ReplaceAll(r.Facts[0].Source.Quote, "预估", "计划")
		}},
		{"negated expression", "anchor", func(r *ResearchReport) {
			r.Facts[0].Source.Quote = "并非" + r.Facts[0].Source.Quote
		}},
		{"estimate marker belongs to other amount", "estimate_anchor", func(r *ResearchReport) {
			r.Facts[0].Source.Quote = "2026年9月份，全国全样本油厂大豆到港1018.55万吨；下月预计减少。"
		}},
		{"invented scope", "anchor", func(r *ResearchReport) { r.Facts[0].Scope = "全部库存环节" }},
		{"missing year", "period_anchor", func(r *ResearchReport) {
			r.Facts[0].Source.Quote = strings.ReplaceAll(r.Facts[0].Source.Quote, "2026年9月份", "9月份")
			r.Facts[0].Source.PeriodText = "9月份"
		}},
		{"quarter relabelled as month", "period_anchor", func(r *ResearchReport) { r.Facts[0].Period = ReportPeriod{"2026-07-01", "2026-09-30"} }},
		{"unproven main channel", "unsupported_assertion", func(r *ResearchReport) { r.Questions[0].Question = "为什么累库主要发生在油厂？" }},
		{"global absence", "unsupported_assertion", func(r *ResearchReport) { r.Questions[0].Missing = []string{"全库没有任何渠道库存数据"} }},
		{"zero-fill missing", "anchor", func(r *ResearchReport) { r.Facts[0].Value = "0" }},
		{"nature promotion", "estimated_as_actual", func(r *ResearchReport) { r.Facts[0].Nature = "reported" }},
		{"duplicate id", "identity", func(r *ResearchReport) { r.Facts = append(r.Facts, r.Facts[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := exampleReport()
			tc.change(&r)
			c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
			if c.Status == "consistent" || !issue(c, tc.code) {
				t.Fatalf("%+v", c)
			}
		})
	}
}
func TestReportCalendarExpressions(t *testing.T) {
	for _, tc := range []struct{ input, start, end string }{{"2024年2月", "2024-02-01", "2024-02-29"}, {"2026年6月底", "2026-06-30", "2026-06-30"}, {"2026年9月18日", "2026-09-18", "2026-09-18"}, {"2025年", "2025-01-01", "2025-12-31"}, {"2026-09-26至2026-10-02", "2026-09-26", "2026-10-02"}} {
		p, ok := sourcePeriod(tc.input)
		if !ok || p != (ReportPeriod{tc.start, tc.end}) {
			t.Fatal(tc, p)
		}
	}
	for _, v := range []string{"2026年2月30日", "2026年13月", "去年9月", "2026-02-30", "2026-9-1", "2026年9月18日至6月底"} {
		if _, ok := sourcePeriod(v); ok {
			t.Fatal("accepted ambiguous/invalid date", v)
		}
	}
}

func TestReportNatureIsBoundToItsOwnExpression(t *testing.T) {
	for _, tc := range []struct{ quote, nature string }{
		{"2026年9月份，全国全样本油厂大豆到港1018.55万吨；下月仍是计划。", "reported"},
		{"2026年9月份，全国全样本油厂计划大豆到港1018.55万吨。", "plan"},
		{"2026年9月份，全国全样本油厂预估大豆到港1018.55万吨。", "estimate"},
	} {
		r := exampleReport()
		r.Facts[0].Source.Quote = tc.quote
		r.Facts[0].Nature = tc.nature
		c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
		if c.Status != "consistent" {
			t.Fatalf("%+v", c)
		}
	}
}
func comparisonReport() ResearchReport {
	r := exampleReport()
	q := "同口径，全国固定样本油厂2026年6月底豆粕库存60.10万吨；2026年9月18日全国固定样本油厂豆粕库存100.15万吨。"
	base := ReportFact{ID: "old", Metric: "豆粕库存", Value: "60.10", Unit: "万吨", Scope: "全国固定样本油厂", Measure: "stock", Nature: "reported", Period: ReportPeriod{"2026-06-30", "2026-06-30"}, Source: ReportSource{"doc-1", q, "2026年6月底"}}
	current := base
	current.ID = "new"
	current.Value = "100.15"
	current.Period = ReportPeriod{"2026-09-18", "2026-09-18"}
	current.Source.PeriodText = "2026年9月18日"
	r.Facts = []ReportFact{base, current}
	r.Comparisons = []ReportComparison{{ID: "c1", Current: "new", Baseline: "old"}}
	r.Questions[0].FactIDs = []string{"old", "new"}
	r.Questions[0].ComparisonIDs = []string{"c1"}
	return r
}
func TestReportComparisonPreservesEndpointDatesAndExactArithmetic(t *testing.T) {
	r := comparisonReport()
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if c.Status != "consistent" || c.Comparisons[0].Difference != "40.05" || !strings.Contains(c.Comparisons[0].From, "2026-06-30") || !strings.Contains(c.Comparisons[0].To, "2026-09-18") {
		t.Fatalf("%+v", c)
	}
	r.Facts[1].Source.DocumentID = "different-survey"
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "comparability") || c.Comparisons[0].Difference != "" {
		t.Fatal(c)
	}
	r = comparisonReport()
	r.Facts[1].Measure = "cumulative_flow"
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "basis") {
		t.Fatal(c)
	}
	r = comparisonReport()
	for i := range r.Facts {
		r.Facts[i].Source.Quote = strings.ReplaceAll(r.Facts[i].Source.Quote, "同口径，", "不同口径，")
	}
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "comparability") || c.Comparisons[0].Consistent {
		t.Fatal(c)
	}
}

func TestReportComparisonDeclarationMustCoverBothObservations(t *testing.T) {
	for _, quote := range []string{
		"同口径，全国固定样本油厂2026年6月底豆粕库存60.10万吨；不同口径，2026年9月18日全国固定样本油厂豆粕库存100.15万吨。",
		"同口径，其他统计如下。全国固定样本油厂2026年6月底豆粕库存60.10万吨；2026年9月18日全国固定样本油厂豆粕库存100.15万吨。",
		"同口径，全国固定样本油厂2026年6月底豆粕库存60.10万吨；中间另换样本。2026年9月18日全国固定样本油厂豆粕库存100.15万吨。",
	} {
		r := comparisonReport()
		for i := range r.Facts {
			r.Facts[i].Source.Quote = quote
		}
		c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
		if !c.Facts[0].Consistent || !c.Facts[1].Consistent || !issue(c, "comparability") || c.Comparisons[0].Consistent || c.Comparisons[0].Difference != "" {
			t.Fatalf("%+v", c)
		}
	}
}

func TestTaskSubmissionPinsClassificationDateAndProtocol(t *testing.T) {
	var received map[string]json.RawMessage
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/apps/futures/runs" || r.Header.Get("Idempotency-Key") != "fixed-key" {
			t.Error("scope/idempotency lost")
		}
		if e := json.NewDecoder(r.Body).Decode(&received); e != nil {
			t.Error(e)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"task"}`)
	}))
	defer peer.Close()
	s := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: strings.Repeat("s", 32)}}
	mux := http.NewServeMux()
	s.Routes(mux)
	body := `{"agent":{"id":"a","revision":1},"commodity":"豆粕","period":"2026年9—12月","question":"核对供给与库存","as_of":"2026-10-09"}`
	r := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
	r.Header.Set("Idempotency-Key", "fixed-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	var meta map[string]string
	var task string
	json.Unmarshal(received["metadata"], &meta)
	json.Unmarshal(received["task"], &task)
	if w.Code != 200 || meta["as_of"] != "2026-10-09" || meta["report_format"] != reportSchema || !strings.Contains(task, reportSchema) {
		t.Fatal(w.Code, meta, task)
	}
	row := map[string]any{"metadata": map[string]any{"commodity": "豆粕", "as_of": "2026-10-09", "report_format": reportSchema, "private": "not forwarded"}}
	project(row)
	if row["as_of"] != "2026-10-09" || row["report_format"] != reportSchema || row["metadata"] != nil {
		t.Fatal(row)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/api/tasks", strings.NewReader(strings.ReplaceAll(body, "2026-10-09", "2026-02-30")))
	r.Header.Set("Idempotency-Key", "fixed-key")
	mux.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestReportStrictFormatCutoffAndQuoteFailure(t *testing.T) {
	raw := encodeReport(t, exampleReport())
	for _, bad := range []string{"# old Markdown", raw + " {}", strings.Replace(raw, `"schema":`, `"schema":"wrong","schema":`, 1), strings.Replace(raw, `"schema":`, `"summary":"fake fact","schema":`, 1), strings.Replace(raw, `"comparisons":[]`, `"comparisons":null`, 1)} {
		c := checkReport(context.Background(), bad, "2026-10-09", acceptedQuote)
		if c.Status != "unstructured" {
			t.Fatal(c)
		}
	}
	if c := checkReport(context.Background(), raw, "2026-09-09", acceptedQuote); !issue(c, "cutoff") {
		t.Fatal(c)
	}
	c := checkReport(context.Background(), raw, "2026-10-09", func(context.Context, ReportSource) (quoteProof, error) {
		return quoteProof{}, errors.New("not read in this task")
	})
	if c.Facts[0].QuoteMatched || !issue(c, "retained_quote") {
		t.Fatal(c)
	}
}
func TestReportHTTPUsesOnlyTaskScopedRetainedReceipts(t *testing.T) {
	r := exampleReport()
	quote := r.Facts[0].Source.Quote
	prefix := strings.Repeat("😀", 8190)
	full := prefix + quote
	seen := []string{}
	token := strings.Repeat("t", 32)
	version := strings.Repeat("a", 64)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing service token")
		}
		seen = append(seen, req.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/v1/apps/futures/runs/task-1":
			json.NewEncoder(w).Encode(map[string]any{"id": "task-1", "status": "completed", "result": encodeReport(t, r), "metadata": map[string]string{"as_of": r.AsOf, "report_format": reportSchema}})
		case "/v1/apps/futures/runs/task-1/evidence":
			io.WriteString(w, `{"items":[{"operation_id":"task-1/read","document_id":"doc-1"}],"next_cursor":""}`)
		case "/v1/apps/futures/runs/task-1/evidence/record":
			chars := []rune(full)
			offset := 0
			if req.URL.Query().Get("offset") != "0" {
				offset = 8192
				if req.URL.Query().Get("version") != version {
					t.Error("missing version")
				}
			}
			end := offset + 8192
			if end > len(chars) {
				end = len(chars)
			}
			var next *int
			if end < len(chars) {
				v := end
				next = &v
			}
			json.NewEncoder(w).Encode(retainedPart{OperationID: "task-1/read", DocumentID: "doc-1", Text: string(chars[offset:end]), Offset: offset, NextOffset: next, Version: version, ReadOffset: 100, ReadEnd: 100 + len(chars)})
		default:
			t.Error("unexpected unscoped request", req.URL)
			http.NotFound(w, req)
		}
	}))
	defer peer.Close()
	server := &Server{Client: serviceclient.Client{RuntimeURL: peer.URL, Token: token}}
	mux := http.NewServeMux()
	server.Routes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/task-1/report-check", nil))
	var out ReportCheck
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e, w.Body.String())
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || out.Status != "consistent" || out.Facts[0].QuoteOffset != 8290 || len(seen) != 4 {
		t.Fatalf("%+v %v", out, seen)
	}
}

func TestRetainedSourceLabelsStayBoundedAndDoNotGuessPartialMetadata(t *testing.T) {
	for _, tc := range []struct {
		text, source, wantURL, wantPublished string
		offset                               int
		truncated                            bool
	}{
		{"来源：https://source.invalid/page\n发布日期：2026-09-24\n原始采集时间：2026-10-09T12:00:00+08:00\n报数", "upload:doc.md", "https://source.invalid/page", "2026-09-24", 0, false},
		{"报数\n发布日期：2026-09", "https://source.invalid/page", "https://source.invalid/page", "", 0, false},
		{"来源：https://source.invalid/not-header\n报数", "upload:doc.md", "", "", 100, false},
		{"报数", "https://source.invalid/truncated", "", "", 0, true},
		{"来源：javascript:alert(1)\n报数", "upload:doc.md", "", "", 0, false},
	} {
		rr := &reportEvidenceReader{cache: map[string][]retainedPart{"doc": {{OperationID: "op", DocumentID: "doc", Version: strings.Repeat("a", 64), ReadOffset: tc.offset, Text: tc.text, Source: tc.source, SourceTruncated: tc.truncated}}}}
		p, e := rr.quote(context.Background(), ReportSource{DocumentID: "doc", Quote: "报数"})
		if e != nil || p.SourceURL != tc.wantURL || p.Published != tc.wantPublished {
			t.Fatalf("%+v %v", p, e)
		}
	}
}
func TestReportRejectsForeignAndChangedSegments(t *testing.T) {
	for _, kind := range []string{"foreign-document", "foreign-operation", "version-change", "stalled-cursor"} {
		t.Run(kind, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/evidence") {
					io.WriteString(w, `{"items":[{"operation_id":"op","document_id":"doc-1"}],"next_cursor":""}`)
					return
				}
				v := strings.Repeat("a", 64)
				doc, op := "doc-1", "op"
				n := 1
				if kind == "foreign-document" {
					doc = "doc-2"
				}
				if kind == "foreign-operation" {
					op = "other"
				}
				if kind == "stalled-cursor" {
					n = 0
				}
				if r.URL.Query().Get("offset") == "1" {
					v = strings.Repeat("b", 64)
				}
				offset := 0
				if r.URL.Query().Get("offset") == "1" {
					offset = 1
				}
				fmt.Fprintf(w, `{"operation_id":%q,"document_id":%q,"version":%q,"offset":%d,"text":"x","next_offset":%d}`, op, doc, v, offset, n)
			}))
			defer peer.Close()
			rr := &reportEvidenceReader{server: &Server{Client: serviceclient.Client{}}, base: peer.URL, cache: map[string][]retainedPart{}}
			if _, e := rr.quote(context.Background(), ReportSource{DocumentID: "doc-1", Quote: "x"}); e == nil {
				t.Fatal("accepted unsafe receipt")
			}
		})
	}
}
