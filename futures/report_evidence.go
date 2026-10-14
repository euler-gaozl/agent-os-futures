package futures

// Evidence declarations describe uncertainty; they never upgrade a quotation
// into truth. They are optional so historical v1 reports remain readable.
type ReportEvidence struct {
	ContentKind string   `json:"content_kind"`
	PeriodBasis string   `json:"period_basis"`
	PeriodNote  string   `json:"period_note"`
	Limitations []string `json:"limitations"`
}

type ReportFinding struct {
	Kind          string   `json:"kind"`
	Text          string   `json:"text"`
	FactIDs       []string `json:"fact_ids"`
	ComparisonIDs []string `json:"comparison_ids"`
}

func checkEvidence(f ReportFact, path string, add func(string, string, string)) {
	e := f.Evidence
	if e == nil {
		return // Historical declarations cannot be invented by the reader.
	}
	switch e.ContentKind {
	case "publisher_text":
	case "ai_summary":
		add(path, "ai_summary", "该报数来自 AI 摘要，只保留为调查线索，尚不能作为原发布者实绩或比较依据；须取得支持它的正文。")
	case "unverified":
		add(path, "evidence_unverified", "来源性质尚未确认，报数只保留待核，不能直接用于比较。")
	default:
		add(path, "evidence_fields", "content_kind 必须为 publisher_text、ai_summary 或 unverified。")
	}
	switch e.PeriodBasis {
	case "explicit":
		if e.PeriodNote != "" && !bounded(e.PeriodNote, 500) {
			add(path, "evidence_fields", "统计期说明超限。")
		}
	case "inferred":
		if !bounded(e.PeriodNote, 500) {
			add(path, "evidence_fields", "推断日期必须说明所依据的原文及仍未确认之处。")
		}
		add(path, "period_inferred", "统计日期含推断，尚未由明确原文确认；不能把发布日期直接当统计日，也不直接用于比较。")
	case "unknown":
		if f.Period.Start != "" || f.Period.End != "" || !bounded(e.PeriodNote, 500) {
			add(path, "evidence_fields", "未知统计期的 start/end 必须同时留空，并说明缺少的日期依据。")
		}
		add(path, "period_unknown", "统计期尚未确认；保留原报数与日期文字，暂不归入近期或远期，也不参与比较。")
	default:
		add(path, "evidence_fields", "period_basis 必须为 explicit、inferred 或 unknown。")
	}
	if e.Limitations == nil || len(e.Limitations) > 8 {
		add(path, "evidence_fields", "来源限制必须为数组，至多八项。")
	}
	for _, note := range e.Limitations {
		if !bounded(note, 500) {
			add(path, "evidence_fields", "来源限制说明为空或超限。")
		}
	}
	if len(e.Limitations) > 0 {
		add(path, "source_limitations", "这项资料标注了来源限制，需回读核对；引用匹配不能解除正文截断、样本、转载或版本冲突等限制。")
	}
}

func checkFindings(q ReportQuestion, path string, facts map[string]ReportFact, comparisons map[string]bool, add func(string, string, string)) {
	if len(q.Findings) > 8 {
		add(path, "finding_fields", "每个问题至多八项资料解读。")
	}
	questionFacts, questionComparisons := map[string]bool{}, map[string]bool{}
	for _, id := range q.FactIDs {
		questionFacts[id] = true
	}
	for _, id := range q.ComparisonIDs {
		questionComparisons[id] = true
	}
	for _, f := range q.Findings {
		if (f.Kind != "observation" && f.Kind != "conflict" && f.Kind != "limitation") || !bounded(f.Text, 1200) || len(f.FactIDs) == 0 || len(f.FactIDs) > 40 || f.ComparisonIDs == nil || len(f.ComparisonIDs) > 30 {
			add(path, "finding_fields", "资料解读需有 observation/conflict/limitation 类型、有界文字和本问题的事实引用；comparison_ids 必须为数组。")
		}
		seen := map[string]bool{}
		for _, id := range f.FactIDs {
			if facts[id].ID == "" || !questionFacts[id] || seen[id] {
				add(path, "finding_reference", "资料解读的事实引用不存在、重复，或不属于本问题。")
			}
			seen[id] = true
		}
		seen = map[string]bool{}
		for _, id := range f.ComparisonIDs {
			if !comparisons[id] || !questionComparisons[id] || seen[id] {
				add(path, "finding_reference", "资料解读的比较引用不存在、重复，或不属于本问题。")
			}
			seen[id] = true
		}
	}
	if len(q.Findings) > 0 {
		add(path, "finding_review", "资料解读已关联报数，但尚未自动验证语义、冲突解释或因果关系；须结合引用和来源限制复核。")
	}
}
