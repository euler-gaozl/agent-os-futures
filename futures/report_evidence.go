package futures

import "fmt"

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
	StatementIDs  []string `json:"statement_ids,omitempty"`
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
		if f.Nature == "reported" && e.PeriodBasis != "unknown" {
			add(path, "ai_summary_as_actual", "AI 摘要不能标为已确认实绩或参与跨月/跨样本比较；请取得正文，或改为待核说明并保留缺口。")
		}
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

func checkFindings(q ReportQuestion, path string, facts map[string]ReportFact, comparisons map[string]bool, statements map[string]ReportStatement, add func(string, string, string)) {
	if len(q.Findings) > 8 {
		add(path, "finding_fields", "每个问题至多八项资料解读。")
	}
	questionFacts, questionComparisons, questionStatements := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, id := range q.FactIDs {
		questionFacts[id] = true
	}
	for _, id := range q.ComparisonIDs {
		questionComparisons[id] = true
	}
	for _, id := range q.StatementIDs {
		questionStatements[id] = true
	}
	for findingIndex, f := range q.Findings {
		findingPath := fmt.Sprintf("%s.findings[%d]", path, findingIndex)
		if (f.Kind != "observation" && f.Kind != "conflict" && f.Kind != "limitation") || !bounded(f.Text, 1200) || (len(f.FactIDs) == 0 && len(f.StatementIDs) == 0) || len(f.FactIDs) > 40 || len(f.StatementIDs) > 20 || f.ComparisonIDs == nil || len(f.ComparisonIDs) > 30 {
			add(findingPath, "finding_fields", "资料解读需有 observation/conflict/limitation 类型、有界文字，以及本问题的事实或非数值说明引用；comparison_ids 必须为数组。")
		}
		seen := map[string]bool{}
		for refIndex, id := range f.FactIDs {
			refPath := fmt.Sprintf("%s.fact_ids[%d]", findingPath, refIndex)
			switch {
			case facts[id].ID == "":
				add(refPath, "finding_reference", "该位置的事实 ID 不存在于 facts；请核对实际支持这条解读的事实，不能编造或借用无关事实通过检查。")
			case seen[id]:
				add(refPath, "finding_reference", "该位置重复引用了本条解读中的同一事实；每条解读的 fact_ids 不得重复。")
			case !questionFacts[id]:
				add(refPath, "finding_reference", "该位置的事实存在于 facts，但未列入所属问题的 fact_ids；核对支持关系后修正本问题的引用归属，不能仅删除相关证据换取通过。")
			}
			seen[id] = true
		}
		if f.Kind == "observation" && regionalClaim.MatchString(f.Text) {
			regional, borrowed := false, false
			for _, id := range f.FactIDs {
				if fact := facts[id]; fact.ID != "" {
					if regionalClaim.MatchString(fact.Scope) {
						regional = true
					} else {
						borrowed = true
					}
				}
			}
			if borrowed && !regional {
				add(findingPath, "scope_borrow", "地区数量或占比不能借用全国/其他样本的事实；请使用对应地区、样本和统计期的事实与原文。")
			}
		}
		seen = map[string]bool{}
		for refIndex, id := range f.StatementIDs {
			refPath := fmt.Sprintf("%s.statement_ids[%d]", findingPath, refIndex)
			switch {
			case statements[id].ID == "":
				add(refPath, "finding_reference", "该位置的非数值说明 ID 不存在于 statements；请核对实际支持这条解读的原文说明。")
			case seen[id]:
				add(refPath, "finding_reference", "该位置重复引用了本条解读中的同一说明；每条解读的 statement_ids 不得重复。")
			case !questionStatements[id]:
				add(refPath, "finding_reference", "该位置的说明存在于 statements，但未列入所属问题的 statement_ids；核对支持关系后修正本问题的引用归属。")
			}
			seen[id] = true
		}
		// A finding that leans on qualitative statements must not also carry a
		// market quantity, even when an unrelated fact ID is attached. Numeric
		// observations belong in a finding backed by facts alone, where the
		// existing value/unit/period checks apply.
		if len(f.StatementIDs) > 0 && numericClaim(f.Text) {
			add(findingPath, "finding_quantity", "引用非数值说明的解读不能夹带市场数量（日期、页码、期数等非报数计数除外）；数值报数须在只引用 facts 的解读中表达并接受检查。")
		}
		seen = map[string]bool{}
		for refIndex, id := range f.ComparisonIDs {
			refPath := fmt.Sprintf("%s.comparison_ids[%d]", findingPath, refIndex)
			switch {
			case !comparisons[id]:
				add(refPath, "finding_reference", "该位置的比较 ID 不存在于 comparisons；请核对实际支持这条解读的比较。")
			case seen[id]:
				add(refPath, "finding_reference", "该位置重复引用了本条解读中的同一比较；每条解读的 comparison_ids 不得重复。")
			case !questionComparisons[id]:
				add(refPath, "finding_reference", "该位置的比较存在于 comparisons，但未列入所属问题的 comparison_ids；核对支持关系后修正本问题的引用归属。")
			}
			seen[id] = true
		}
	}
	if len(q.Findings) > 0 {
		add(path, "finding_review", "资料解读已关联报数，但尚未自动验证语义、冲突解释或因果关系；须结合引用和来源限制复核。")
	}
}
