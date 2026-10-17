package futures

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFindingDiagnosticsIdentifyTheExactBrokenReference(t *testing.T) {
	for _, tc := range []struct {
		name, field, want string
		ids               []string
	}{
		{"missing fact", "fact_ids", "不存在于 facts", []string{"a", "missing"}},
		{"unowned fact", "fact_ids", "未列入所属问题", []string{"a", "b"}},
		{"duplicate fact", "fact_ids", "重复引用", []string{"a", "a"}},
		{"missing comparison", "comparison_ids", "不存在于 comparisons", []string{"x", "missing"}},
		{"unowned comparison", "comparison_ids", "未列入所属问题", []string{"x", "y"}},
		{"duplicate comparison", "comparison_ids", "重复引用", []string{"x", "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valid := ReportFinding{Kind: "observation", Text: "合成测试说明", FactIDs: []string{"a"}, ComparisonIDs: []string{"x"}}
			q := ReportQuestion{ID: "q", FactIDs: []string{"a"}, ComparisonIDs: []string{"x"}, Findings: []ReportFinding{valid, valid, valid}}
			if tc.field == "fact_ids" {
				q.Findings[2].FactIDs = tc.ids
			} else {
				q.Findings[2].ComparisonIDs = tc.ids
			}
			before, _ := json.Marshal(q)
			var issues []ReportIssue
			checkFindings(q, "questions.q", map[string]ReportFact{"a": {ID: "a"}, "b": {ID: "b"}}, map[string]bool{"x": true, "y": true}, func(p, c, m string) {
				if c == "finding_reference" {
					issues = append(issues, ReportIssue{Path: p, Code: c, Message: m})
				}
			})
			after, _ := json.Marshal(q)
			if string(before) != string(after) || len(issues) != 1 || issues[0].Path != "questions.q.findings[2]."+tc.field+"[1]" || !strings.Contains(issues[0].Message, tc.want) {
				t.Fatalf("wrong location, explanation, or mutated input: %+v", issues)
			}
		})
	}
}

func TestDraftCorrectionMustRepairTheReportedFinding(t *testing.T) {
	r := exampleReport()
	other := r.Facts[0]
	other.ID = "f2"
	r.Facts = append(r.Facts, other)
	r.Questions[0].Findings = []ReportFinding{
		{Kind: "observation", Text: "合成测试第一条", FactIDs: []string{"f1"}, ComparisonIDs: []string{}},
		{Kind: "limitation", Text: "合成测试第二条", FactIDs: []string{"f2"}, ComparisonIDs: []string{}},
	}
	for pass := 0; pass < 2; pass++ {
		feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
		if feedback.Validator != "futures-report-check-v7" || feedback.Status != "revision_required" || len(feedback.Revise) != 1 || feedback.Revise[0].Path != "questions.q1.findings[1].fact_ids[0]" {
			t.Fatalf("wrong finding identified after unrelated edit: %+v", feedback)
		}
		// Editing an unrelated finding must not conceal the actual broken link.
		r.Questions[0].Findings[0].Text = "合成测试修改了第一条文字"
	}
	r.Questions[0].FactIDs = append(r.Questions[0].FactIDs, "f2")
	feedback := reviewDraft(context.Background(), encodeReport(t, r), r.AsOf)
	if feedback.Status != "source_review_required" || len(feedback.Revise) != 0 || len(feedback.SourceReview) == 0 {
		t.Fatalf("repaired ownership must still require source/semantic review: %+v", feedback)
	}
}
