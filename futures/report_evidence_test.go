package futures

import (
	"context"
	"strings"
	"testing"
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
