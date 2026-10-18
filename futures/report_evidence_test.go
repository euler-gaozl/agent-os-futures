package futures

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUncertainEvidenceCanBeRetainedWithoutInventedDates(t *testing.T) {
	r := exampleReport()
	f := &r.Facts[0]
	f.Period = ReportPeriod{}
	f.Source.Quote = "智能摘要：本周大豆到港预估1018.55万吨。内容由AI生成。"
	f.Source.PeriodText = "本周"
	f.Evidence = &ReportEvidence{ContentKind: "ai_summary", PeriodBasis: "unknown", PeriodNote: "摘要只写本周，未取得统计区间正文。", Limitations: []string{"正文需要订阅，摘要不能替代原发布者调查。"}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if c.Report == nil || !c.Facts[0].QuoteMatched || c.Facts[0].Consistent || c.Facts[0].Group != "统计期未确认 · 待核报数" {
		t.Fatalf("lost candidate or fabricated validation: %+v", c)
	}
	for _, code := range []string{"ai_summary", "period_unknown", "source_limitations"} {
		if !issue(c, code) {
			t.Fatalf("missing uncertainty: %s", code)
		}
	}
	if issue(c, "period") || issue(c, "evidence_fields") || issue(c, "period_anchor") {
		t.Fatal("unknown date was treated as an instruction to invent one", c.Issues)
	}
	feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	if len(feedback.Revise) != 0 || feedback.Status != "source_review_required" {
		t.Fatal("honest uncertainty incorrectly requires structural revision", feedback)
	}
	// Explicit unknown cannot conceal a supplied date, even a valid one.
	f.Period = ReportPeriod{"2026-09-01", "2026-09-30"}
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "evidence_fields") || c.Facts[0].Consistent || c.Facts[0].Group != "统计期未确认 · 待核报数" {
		t.Fatal(c)
	}
}

func TestInferredDatesDoNotBecomeFutureActualRevisionRequirements(t *testing.T) {
	r := exampleReport()
	f := &r.Facts[0]
	f.Nature = "reported"
	f.Period = ReportPeriod{"2027-09-01", "2027-09-30"}
	f.Source.Quote = "9月份，全国全样本油厂大豆到港1018.55万吨。"
	f.Source.PeriodText = "9月份"
	f.Evidence = &ReportEvidence{ContentKind: "publisher_text", PeriodBasis: "inferred", PeriodNote: "原文没有年份，暂填的年份含推断，仍须回查原发布者确认。", Limitations: []string{}}
	r.Questions[0].Question = "到港报数的日期依据是否明确？"
	raw := encodeReport(t, r)
	c := checkReport(context.Background(), raw, r.AsOf, acceptedQuote)
	if !issue(c, "period_inferred") || !issue(c, "period_anchor") || issue(c, "future_actual") || c.Facts[0].Consistent || !c.Facts[0].QuoteMatched || c.Facts[0].Group != "日期含推断 · 暂不归类" {
		t.Fatalf("inferred year became an established future date: %+v", c)
	}
	if c.Report.Facts[0].Source.Quote != f.Source.Quote || c.Report.Facts[0].Period != f.Period {
		t.Fatal("review rewrote the candidate evidence")
	}
	feedback := reviewDraft(context.Background(), raw, r.AsOf)
	if len(feedback.Revise) != 0 || feedback.Status != "source_review_required" {
		t.Fatalf("honest date uncertainty incorrectly requires structural revision: %+v", feedback)
	}

	// Declaring uncertainty does not excuse malformed fields or erase the
	// existing rule once the source explicitly establishes a future period.
	f.Period.Start = "2027-09-31"
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "period") || len(reviewDraft(context.Background(), encodeReport(t, r), r.AsOf).Revise) == 0 {
		t.Fatal("invalid inferred calendar interval accepted", c)
	}
	f.Period.Start = "2027-09-01"
	f.Source.Quote = "2027年9月份，全国全样本油厂大豆到港1018.55万吨。"
	f.Source.PeriodText = "2027年9月份"
	f.Evidence.PeriodBasis = "explicit"
	f.Evidence.PeriodNote = ""
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "future_actual") || c.Facts[0].Consistent || len(reviewDraft(context.Background(), encodeReport(t, r), r.AsOf).Revise) == 0 {
		t.Fatal("explicit future actual no longer requires revision", c)
	}
}

func TestDeclaredEvidenceNeverOverridesQuoteOrComparisonChecks(t *testing.T) {
	for _, tc := range []struct{ kind, basis, code string }{
		{"ai_summary", "explicit", "ai_summary"},
		{"unverified", "explicit", "evidence_unverified"},
		{"publisher_text", "inferred", "period_inferred"},
	} {
		r := comparisonReport()
		r.Facts[0].Evidence = &ReportEvidence{ContentKind: tc.kind, PeriodBasis: tc.basis, PeriodNote: "年份依据页面日期推断，未获明确正文确认。", Limitations: []string{}}
		c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
		if !issue(c, tc.code) || c.Facts[0].Consistent || c.Comparisons[0].Consistent || c.Comparisons[0].Difference != "" {
			t.Fatalf("uncertainty upgraded into a comparison: %+v", c)
		}
	}
	r := exampleReport()
	r.Facts[0].Evidence = &ReportEvidence{ContentKind: "publisher_text", PeriodBasis: "explicit", Limitations: []string{}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, nil)
	if c.Facts[0].QuoteMatched || c.Facts[0].Consistent || !issue(c, "retained_unchecked") {
		t.Fatal("self-declaration created a receipt proof")
	}
	r.Facts[0].Evidence.Limitations = []string{"监测样本可能变动，尚未取得样本明细。"}
	c = checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "source_limitations") || c.Facts[0].Consistent || !c.Facts[0].QuoteMatched {
		t.Fatal(c)
	}
}

func TestEvidenceDeclarationsValidateButLegacyReportsStayReadable(t *testing.T) {
	for _, e := range []*ReportEvidence{
		{},
		{ContentKind: "verified", PeriodBasis: "explicit", Limitations: []string{}},
		{ContentKind: "publisher_text", PeriodBasis: "inferred", Limitations: []string{}},
		{ContentKind: "publisher_text", PeriodBasis: "explicit", Limitations: []string{strings.Repeat("字", 501)}},
	} {
		r := exampleReport()
		r.Facts[0].Evidence = e
		if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "evidence_fields") {
			t.Fatal(c)
		}
	}
	r := exampleReport()
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); c.Report == nil || c.Status != "consistent" {
		t.Fatal("legacy optional-field compatibility changed", c)
	}
}

func TestFindingsBindToQuestionWithoutClaimingSemanticVerification(t *testing.T) {
	r := exampleReport()
	r.Questions[0].Findings = []ReportFinding{{Kind: "limitation", Text: "目前取得的是原发布者到港估计，实际到港仍需补充；不能据此确认实绩。", FactIDs: []string{"f1"}, ComparisonIDs: []string{}}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "finding_review") || issue(c, "finding_fields") || issue(c, "finding_reference") || c.Status == "consistent" || !c.Facts[0].Consistent {
		t.Fatal("finding and evidence checks were conflated", c)
	}
	feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	if len(feedback.Revise) != 0 {
		t.Fatal("bounded evidence-linked answer rejected", feedback)
	}
	for _, ids := range [][]string{{"missing"}, {"f1", "f1"}, {}} {
		r.Questions[0].Findings[0].FactIDs = ids
		c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
		if !issue(c, "finding_fields") && !issue(c, "finding_reference") {
			t.Fatal("unbound finding accepted", c)
		}
	}
	r.Questions[0].Findings[0].FactIDs = []string{"f1"}
	r.Questions[0].Findings[0].Kind = "prediction"
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "finding_fields") {
		t.Fatal(c)
	}
}

func TestAISummaryCannotBeReportedFact(t *testing.T) {
	r := exampleReport()
	f := &r.Facts[0]
	f.Nature = "reported"
	f.Source.Quote = "智能摘要：2026年第39周大豆库存865.43万吨。内容由AI生成。"
	f.Source.PeriodText = "2026年第39周"
	f.Evidence = &ReportEvidence{ContentKind: "ai_summary", PeriodBasis: "inferred", PeriodNote: "时点由标题日期推定。", Limitations: []string{"AI生成摘要，未取得正文。"}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "ai_summary_as_actual") {
		t.Fatalf("AI summary was accepted as a reported fact: %+v", c.Issues)
	}
	if feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf); feedback.Status != "revision_required" {
		t.Fatalf("AI summary did not require revision: %+v", feedback)
	}
}

func TestMultiPeriodEstimateCanShareContinuousQuote(t *testing.T) {
	r := exampleReport()
	first := r.Facts[0]
	first.Nature = "estimate"
	first.Period = ReportPeriod{"2026-10-01", "2026-10-31"}
	first.Source.PeriodText = "2026年10月份"
	first.Source.Quote = "2026年10月份国内油厂大豆到港约854.1万吨，11月预计870万吨。"
	second := first
	second.ID = "f2"
	second.Value = "870"
	second.Period = ReportPeriod{"2026-11-01", "2026-11-30"}
	second.Source.PeriodText = "11月"
	r.Facts = []ReportFact{first, second}
	r.Questions[0].FactIDs = []string{"f1", "f2"}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if issue(c, "multi_period_estimate") {
		t.Fatalf("split estimates sharing one continuous quote were rejected: %+v", c.Issues)
	}
}

func TestStatementClaimRequiresSupportAndAcceptsExplicitSourceLine(t *testing.T) {
	r := exampleReport()
	r.Statements = []ReportStatement{{ID: "s1", Text: "新华财经快讯为海关数据转引，页面暂无详文。", DocumentID: "doc-1", Quote: "新华财经 | 2026年09月08日", ContentKind: "publisher_text", Limitations: []string{}}}
	r.Questions[0].StatementIDs = []string{"s1"}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "statement_support") {
		t.Fatalf("unsupported statement claim accepted: %+v", c.Issues)
	}
	if feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf); feedback.Status != "revision_required" {
		t.Fatalf("statement support did not require revision: %+v", feedback)
	}
	r.Statements[0].Quote = "本文转载自新华财经，页面暂无详文。"
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); issue(c, "statement_support") {
		t.Fatalf("explicit source line was not accepted: %+v", c.Issues)
	}
}

func TestWhitespaceOnlyQuoteDifferenceIsExplicitNotSilent(t *testing.T) {
	r := exampleReport()
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, func(context.Context, ReportSource) (quoteProof, error) {
		return quoteProof{Operation: "task/read", Version: strings.Repeat("a", 64), MatchMode: "whitespace"}, nil
	})
	if !issue(c, "quote_normalized") || !c.Facts[0].QuoteMatched || c.Facts[0].Consistent {
		t.Fatalf("whitespace match was not surfaced explicitly: %+v", c)
	}
	if feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf); feedback.Status != "source_review_required" || len(feedback.Revise) != 0 {
		t.Fatalf("normalized quote should remain source review: %+v", feedback)
	}
}

func TestReceiptBudgetIsNotReportedAsQuoteMismatch(t *testing.T) {
	r := exampleReport()
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, func(context.Context, ReportSource) (quoteProof, error) {
		return quoteProof{}, errReceiptBudget
	})
	if !issue(c, "receipt_budget") || issue(c, "retained_quote") {
		t.Fatalf("budget exhaustion was misreported as a bad quote: %+v", c.Issues)
	}
}

func TestNormalizedQuoteIndexPreservesOriginalOffset(t *testing.T) {
	text := "来源：监测显示，9月18日，进口 大豆 库存869万吨。"
	off, ok := normalizedQuoteIndex(text, "进口大豆库存869万吨")
	if !ok || string([]rune(text)[off:off+utf8.RuneCountInString("进口 大豆 库存869万吨")]) != "进口 大豆 库存869万吨" {
		t.Fatalf("normalized offset lost mapping: %d %v", off, ok)
	}
}

func TestStatementClaimCounterexamples(t *testing.T) {
	for _, tc := range []struct{ name, text, quote string }{
		{"second claim", "页面暂无详文，两机构调查方法不同", "页面暂无详文"},
		{"partial no-detail", "页面暂无详文", "暂无库存数据"},
		{"database-is-not-source", "此数字是海关数据转引", "市场数据库"},
	} {
		r := exampleReport()
		r.Statements = []ReportStatement{{ID: "s1", Text: tc.text, DocumentID: "doc-1", Quote: tc.quote, ContentKind: "publisher_text", Limitations: []string{}}}
		r.Questions[0].StatementIDs = []string{"s1"}
		if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); !issue(c, "statement_support") {
			t.Fatalf("%s: unsupported claim was accepted: %+v", tc.name, c.Issues)
		}
	}
}

func TestFindingMonthMustBeCoveredByCitedFact(t *testing.T) {
	r := exampleReport()
	r.Questions[0].Findings = []ReportFinding{{Kind: "observation", Text: "2025年6月末库存为74.29万吨，9月18日为117.32万吨。", FactIDs: []string{"f1"}, ComparisonIDs: []string{}}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "finding_period_borrow") {
		t.Fatalf("finding borrowed a month outside the cited fact: %+v", c.Issues)
	}
	r.Facts[0].Period = ReportPeriod{"2026-09-01", "2026-09-30"}
	r.Questions[0].Findings[0].Text = "9月18日库存为117.32万吨。"
	if c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote); issue(c, "finding_period_borrow") {
		t.Fatalf("covered month was rejected: %+v", c.Issues)
	}
}

func TestFindingMonthMustRespectYear(t *testing.T) {
	r := exampleReport()
	r.Facts[0].Period = ReportPeriod{Start: "2026-09-01", End: "2026-09-30"}
	r.Questions[0].Findings = []ReportFinding{{Kind: "observation", Text: "2025年9月库存情况。", FactIDs: []string{"f1"}, ComparisonIDs: []string{}}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if !issue(c, "finding_period_borrow") {
		t.Fatal("accepted a 2025 month backed by a 2026 fact")
	}
}

func TestJointQuoteWhitespaceAcrossSegments(t *testing.T) {
	hash := strings.Repeat("a", 64)
	parts := []retainedPart{
		{OperationID: "task/read/1", DocumentID: "doc-1", OriginalHash: hash, Version: hash, ReadOffset: 0, ReadEnd: 6, Text: "进口 大豆 "},
		{OperationID: "task/read/2", DocumentID: "doc-1", OriginalHash: hash, Version: hash, ReadOffset: 6, ReadEnd: 13, Text: "库存869万吨"},
	}
	proof, ok := joinedQuote(parts, ReportSource{DocumentID: "doc-1", Quote: "进口大豆库存869万吨"})
	if !ok || proof.MatchMode != "whitespace" || proof.Offset != 0 || len(proof.Segments) != 2 {
		t.Fatalf("cross-segment whitespace match was not explicit: %+v %v", proof, ok)
	}
}

func TestPublicationMonthDoesNotBorrowStatisticalPeriod(t *testing.T) {
	r := exampleReport()
	r.Facts[0].Period = ReportPeriod{Start: "2026-09-01", End: "2026-09-30"}
	r.Questions[0].Findings = []ReportFinding{{Kind: "observation", Text: "2026年10月8日发布的报告披露：2026年9月大豆压榨991.04万吨。", FactIDs: []string{"f1"}, ComparisonIDs: []string{}}}
	c := checkReport(context.Background(), encodeReport(t, r), r.AsOf, acceptedQuote)
	if issue(c, "finding_period_borrow") || issue(c, "finding_period_review") {
		t.Fatalf("publication month was treated as a statistical month: %+v", c.Issues)
	}
}
