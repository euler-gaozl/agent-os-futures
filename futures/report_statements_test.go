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
	"unicode/utf8"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func qualitativeReport() ResearchReport {
	r := exampleReport()
	r.Facts = []ReportFact{}
	r.Questions[0].FactIDs = []string{}
	r.Questions[0].StatementIDs = []string{"s1"}
	r.Questions[0].Missing = []string{}
	r.Questions[0].Findings = []ReportFinding{{
		Kind:          "limitation",
		Text:          "只能确认该周报的转载正文这样说明，原始样本定义仍未披露。",
		FactIDs:       []string{},
		StatementIDs:  []string{"s1"},
		ComparisonIDs: []string{},
	}}
	r.Statements = []ReportStatement{{
		ID:          "s1",
		Text:        "该段库存报数来自转载正文，原始样本明细未披露，不能确认样本前后一致。",
		DocumentID:  "doc-1",
		Quote:       "本文转载自行业协会周报，样本明细未披露。",
		ContentKind: "publisher_text",
		Limitations: []string{},
	}}
	return r
}

func TestQualitativeStatementSupportsFindingWithoutNumericFact(t *testing.T) {
	r := qualitativeReport()
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if c.Report == nil || len(c.Report.Facts) != 0 || len(c.Statements) != 1 {
		t.Fatalf("%+v", c)
	}
	s := c.Statements[0]
	if !s.QuoteMatched || !s.Consistent || s.QuoteOffset != 42 || s.ContentKind != "publisher_text" {
		t.Fatalf("statement citation not bound: %+v", s)
	}
	if issue(c, "finding_fields") || issue(c, "finding_reference") || issue(c, "statement_fields") || issue(c, "empty") {
		t.Fatalf("statement-only support rejected: %+v", c.Issues)
	}
	if !issue(c, "finding_review") || !issue(c, "statement_review") || c.Status == "consistent" {
		t.Fatalf("semantic review warning was lost: %+v", c)
	}
	// Structure-only readiness: source/semantic review remains required.
	feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	if feedback.Status != "source_review_required" || len(feedback.Revise) != 0 {
		t.Fatalf("%+v", feedback)
	}
	if ready := draftMCPResult(feedback)["structuredContent"].(map[string]any)["ready"]; ready != true {
		t.Fatalf("qualitative draft should be structurally deliverable: %+v", feedback)
	}
}

func TestStatementsCannotBypassNumericFactChecks(t *testing.T) {
	r := qualitativeReport()
	r.Statements[0].Text = "本周大豆到港1018.55万吨，样本明细未披露。"
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "statement_quantity") || len(reviewDraft(context.Background(), encodeReport(t, r), r.AsOf).Revise) == 0 {
		t.Fatalf("numeric claim hidden in a statement was accepted: %+v", c)
	}
	// A bare number without a unit is still an unchecked observation.
	r = qualitativeReport()
	r.Statements[0].Text = "本周库存为100，样本明细未披露。"
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "statement_quantity") {
		t.Fatalf("bare quantity hidden in a statement was accepted: %+v", c)
	}
	// Full-width digits are still digits, not prose.
	r = qualitativeReport()
	r.Statements[0].Text = "本周库存为１００，样本明细未披露。"
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "statement_quantity") {
		t.Fatalf("full-width quantity hidden in a statement was accepted: %+v", c)
	}
	// A multiplier is a numeric claim even without a digit.
	r = qualitativeReport()
	r.Statements[0].Text = "库存翻倍，样本明细未披露。"
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "statement_quantity") {
		t.Fatalf("multiplier hidden in a statement was accepted: %+v", c)
	}
	// Limitations must not become a second place to put an unchecked quantity.
	r = qualitativeReport()
	r.Statements[0].Limitations = []string{"正文提到库存100，未取得完整表格。"}
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "statement_quantity") {
		t.Fatalf("quantity hidden in a statement limitation was accepted: %+v", c)
	}
	r = qualitativeReport()
	r.Statements[0].Limitations = []string{"正文只覆盖前3页，样本明细未披露。"}
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); issue(c, "statement_quantity") {
		t.Fatalf("coverage page count in a limitation was treated as a quantity: %+v", c)
	}
	// A statement may name dates, page ranges, periods and sample counts.
	for _, allowed := range []string{
		"该说明适用于2026年10-12月，样本明细未披露。",
		"正文只覆盖前3页，样本明细未披露。",
		"样本从5家变为3家，正文仅覆盖2026/10/09后的页面。",
		"2026.10.09 与 2026-10 的版本差异未披露。",
		"第3期周报只覆盖2026年Q4，样本明细未披露。",
		"样本仅覆盖两家油厂，正文只到第3页。",
	} {
		r := qualitativeReport()
		r.Statements[0].Text = allowed
		if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); issue(c, "statement_quantity") {
			t.Fatalf("legitimate non-market counter was treated as a quantity: %q %+v", allowed, c)
		}
	}
	// A finding supported only by statements must not carry a quantity either.
	r = qualitativeReport()
	r.Questions[0].Findings[0].Text = "到港1018.55万吨，样本明细未披露。"
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "finding_quantity") || len(reviewDraft(context.Background(), encodeReport(t, r), r.AsOf).Revise) == 0 {
		t.Fatalf("numeric claim hidden in a statement-only finding was accepted: %+v", c)
	}
	// Attaching an unrelated fact ID must not unlock a market quantity in a
	// finding that still relies on qualitative statements.
	r = qualitativeReport()
	r.Facts = exampleReport().Facts
	r.Questions[0].FactIDs = []string{"f1"}
	r.Questions[0].Findings[0].FactIDs = []string{"f1"}
	r.Questions[0].Findings[0].Text = "样本覆盖达到1018.55家。"
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "finding_quantity") {
		t.Fatalf("borrowed fact ID unlocked a numeric statement finding: %+v", c)
	}
	// The same market quantity is left to the fact checks when the finding
	// references facts only (split the qualitative and numeric claims).
	r = exampleReport()
	r.Statements = qualitativeReport().Statements
	r.Questions[0].StatementIDs = []string{"s1"}
	r.Questions[0].Findings = []ReportFinding{{Kind: "observation", Text: "已披露到港1018.55万吨。", FactIDs: []string{"f1"}, ComparisonIDs: []string{}}}
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if issue(c, "finding_quantity") || issue(c, "finding_fields") {
		t.Fatalf("fact-only quantity was rejected as a statement bypass: %+v", c)
	}
}

func TestStatementReferenceOwnershipAndDuplicatesAreBounded(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ResearchReport)
		code string
	}{
		{"duplicate question reference", func(r *ResearchReport) { r.Questions[0].StatementIDs = []string{"s1", "s1"} }, "statement_reference"},
		{"unknown question reference", func(r *ResearchReport) { r.Questions[0].StatementIDs = []string{"missing"} }, "statement_reference"},
		{"unowned finding reference", func(r *ResearchReport) {
			r.Questions[0].StatementIDs = []string{}
		}, "finding_reference"},
		{"duplicate finding reference", func(r *ResearchReport) {
			r.Questions[0].Findings[0].StatementIDs = []string{"s1", "s1"}
		}, "finding_reference"},
		{"unknown finding reference", func(r *ResearchReport) {
			r.Questions[0].Findings[0].StatementIDs = []string{"missing"}
		}, "finding_reference"},
		{"duplicate statement id", func(r *ResearchReport) { r.Statements = append(r.Statements, r.Statements[0]) }, "identity"},
		{"statement id collides with fact", func(r *ResearchReport) {
			r.Facts = exampleReport().Facts
			r.Questions[0].FactIDs = []string{"f1"}
			r.Statements[0].ID = "f1"
			r.Questions[0].StatementIDs = []string{"f1"}
			r.Questions[0].Findings[0].StatementIDs = []string{"f1"}
		}, "identity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := qualitativeReport()
			tc.edit(&r)
			c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
			if !issue(c, tc.code) {
				t.Fatalf("missing %s: %+v", tc.code, c.Issues)
			}
		})
	}
	// A statement-only finding still needs comparison_ids as an explicit array.
	r := qualitativeReport()
	r.Questions[0].Findings[0].ComparisonIDs = nil
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "finding_fields") {
		t.Fatalf("nil comparison_ids accepted: %+v", c)
	}
}

func TestStatementFieldsNatureAndRetainedQuoteStayReviewable(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ResearchReport)
		code string
	}{
		{"ai summary", func(r *ResearchReport) { r.Statements[0].ContentKind = "ai_summary" }, "statement_ai_summary"},
		{"unverified", func(r *ResearchReport) { r.Statements[0].ContentKind = "unverified" }, "statement_unverified"},
		{"unknown nature", func(r *ResearchReport) { r.Statements[0].ContentKind = "verified" }, "statement_fields"},
		{"missing limitations", func(r *ResearchReport) { r.Statements[0].Limitations = nil }, "statement_fields"},
		{"bounded text", func(r *ResearchReport) { r.Statements[0].Text = strings.Repeat("字", 801) }, "statement_fields"},
		{"bounded quote", func(r *ResearchReport) { r.Statements[0].Quote = strings.Repeat("字", 2401) }, "statement_source"},
		{"invalid document", func(r *ResearchReport) { r.Statements[0].DocumentID = "../other" }, "statement_source"},
		{"bounded limitation", func(r *ResearchReport) { r.Statements[0].Limitations = []string{strings.Repeat("字", 501)} }, "statement_fields"},
		{"declared limitation", func(r *ResearchReport) { r.Statements[0].Limitations = []string{"正文样本定义未披露。"} }, "statement_limitations"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := qualitativeReport()
			tc.edit(&r)
			c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
			if !issue(c, tc.code) {
				t.Fatalf("missing %s: %+v", tc.code, c.Issues)
			}
		})
	}
	// AI and unverified statements remain source-review warnings, not structural
	// blockers, so an honest draft can still be finalized for human review.
	r := qualitativeReport()
	r.Statements[0].ContentKind = "ai_summary"
	if f := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf); f.Status != "source_review_required" || len(f.Revise) != 0 {
		t.Fatalf("%+v", f)
	}
	// An unmatched statement quote must not be reported as matched.
	r = qualitativeReport()
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, func(context.Context, ReportSource) (quoteProof, error) {
		return quoteProof{}, errors.New("not retained")
	})
	if c.Statements[0].QuoteMatched || !issue(c, "retained_statement_quote") || c.Statements[0].Consistent {
		t.Fatalf("%+v", c)
	}
}

func TestStatementProofKeepsReceiptIdentityAndSegments(t *testing.T) {
	r := qualitativeReport()
	seen := 0
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, func(_ context.Context, source ReportSource) (quoteProof, error) {
		seen++
		if source.DocumentID != "doc-1" || source.Quote != r.Statements[0].Quote {
			t.Fatal(source)
		}
		return quoteProof{Operation: "task/read", Version: strings.Repeat("b", 64), Offset: 9, SourceURL: "https://source.invalid/page", Published: "2026-09-24", Captured: "2026-10-09T12:00:00+08:00",
			OriginalHash: strings.Repeat("c", 64), Segments: []QuoteSegment{{OperationID: "task/read", Version: strings.Repeat("b", 64), ReadOffset: 0, ReadEnd: 30, QuoteOffset: 9, QuoteEnd: 29}}}, nil
	})
	if seen != 1 || !c.Statements[0].QuoteMatched || c.Statements[0].OperationID != "task/read" || c.Statements[0].QuoteOffset != 9 || len(c.Statements[0].Segments) != 1 || c.Statements[0].SourceURL != "https://source.invalid/page" || c.Statements[0].PublishedLabel != "2026-09-24" {
		t.Fatalf("lost receipt proof: %+v", c.Statements[0])
	}
}

func TestLegacyReportsWithoutStatementsAndNewCheckerVersion(t *testing.T) {
	r := exampleReport()
	raw := encodeReport(t, r)
	if strings.Contains(raw, "statement") {
		t.Fatal("legacy report unexpectedly gained a statement field")
	}
	c := checkReport(context.Background(), raw, r.AsOf, acceptedQuote)
	if c.Validator != "futures-report-check-v8" || c.Report == nil || c.Status != "consistent" || c.Statements == nil || len(c.Statements) != 0 {
		t.Fatalf("%+v", c)
	}
	feedback := reviewDraft(context.Background(), raw, r.AsOf)
	if feedback.Validator != "futures-report-check-v8" || feedback.Status != "source_review_required" || len(feedback.Revise) != 0 {
		t.Fatalf("%+v", feedback)
	}
	// New fields are optional on questions and findings as well as the report.
	r = qualitativeReport()
	r.Questions[0].StatementIDs = nil
	r.Questions[0].Findings[0].StatementIDs = nil
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); c.Report == nil {
		t.Fatalf("optional statement fields became required: %+v", c)
	}
}

func TestReportInstructionsDescribeQualitativeEvidence(t *testing.T) {
	text := reportInstructions("2026-10-09")
	for _, want := range []string{`"statements"`, `"statement_ids"`, "非数值论据", "不证明解释", "20说明"} {
		if !strings.Contains(text, want) {
			t.Fatalf("instructions missing %q", want)
		}
	}
}

func TestStatementJSONRoundTripIsStrict(t *testing.T) {
	r := qualitativeReport()
	raw := encodeReport(t, r)
	var decoded ResearchReport
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil || len(decoded.Statements) != 1 || decoded.Statements[0].ID != "s1" {
		t.Fatalf("%v %+v", err, decoded)
	}
	// Unknown fields inside a statement are rejected by the shared shape check.
	c := checkReport(context.Background(), strings.Replace(raw, `"content_kind":"publisher_text"`, `"content_kind":"publisher_text","value":"1018.55"`, 1), r.AsOf, acceptedQuote)
	if c.Status != "unstructured" || !issue(c, "format") {
		t.Fatalf("%+v", c)
	}
}

func TestStatementHTTPUsesOnlyTaskScopedRetainedReceipts(t *testing.T) {
	r := qualitativeReport()
	quote := r.Statements[0].Quote
	text := "前言\n" + quote + "\n结束"
	token := strings.Repeat("t", 32)
	version := strings.Repeat("a", 64)
	seen := []string{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing service token")
		}
		seen = append(seen, req.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/v1/apps/futures/runs/task-2":
			json.NewEncoder(w).Encode(map[string]any{"id": "task-2", "status": "completed", "result": encodeReport(t, r), "metadata": map[string]string{"as_of": r.AsOf, "report_format": reportSchema}})
		case "/v1/apps/futures/runs/task-2/evidence":
			io.WriteString(w, `{"items":[{"operation_id":"task-2/read","document_id":"doc-1"}],"next_cursor":""}`)
		case "/v1/apps/futures/runs/task-2/evidence/record":
			json.NewEncoder(w).Encode(retainedPart{OperationID: "task-2/read", DocumentID: "doc-1", Text: text, Offset: 0, Version: version, ReadOffset: 100, ReadEnd: 100 + utf8.RuneCountInString(text)})
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
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/task-2/report-check", nil))
	var out ReportCheck
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err, w.Body.String())
	}
	if w.Code != 200 || len(out.Statements) != 1 || !out.Statements[0].QuoteMatched || out.Statements[0].OperationID != "task-2/read" || issue(out, "retained_statement_quote") {
		t.Fatalf("%+v", out)
	}
	// One task read, one receipt list for the document, one record page.
	if len(seen) != 3 {
		t.Fatalf("unexpected receipt requests: %v", seen)
	}
}

func TestComparisonsRemainNumericFactOnly(t *testing.T) {
	r := qualitativeReport()
	r.Comparisons = []ReportComparison{{ID: "c1", Current: "s1", Baseline: "s1"}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "operands") || c.Comparisons[0].Consistent || c.Comparisons[0].Difference != "" {
		t.Fatalf("qualitative statement entered comparison arithmetic: %+v", c)
	}
}

func TestStatementCountIsBounded(t *testing.T) {
	r := exampleReport()
	for i := 0; i < 21; i++ {
		r.Statements = append(r.Statements, ReportStatement{ID: fmt.Sprintf("s%d", i), Text: "来源说明，不含报数。", DocumentID: "doc-1", Quote: "原文说明", ContentKind: "publisher_text", Limitations: []string{}})
	}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if c.Status != "unstructured" || !issue(c, "format") {
		t.Fatalf("statement count cap not enforced: %+v", c)
	}
}
