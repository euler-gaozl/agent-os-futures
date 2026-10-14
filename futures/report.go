package futures

// A read-only business projection. Runtime owns the original result and receipts.
// These checks establish bounded structural/quotation consistency, not truth.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
)

const reportSchema = "futures-evidence-v1"

type ResearchReport struct {
	Schema      string             `json:"schema"`
	AsOf        string             `json:"as_of"`
	Questions   []ReportQuestion   `json:"questions"`
	Facts       []ReportFact       `json:"facts"`
	Comparisons []ReportComparison `json:"comparisons"`
}
type ReportQuestion struct {
	ID            string   `json:"id"`
	Question      string   `json:"question"`
	FactIDs       []string `json:"fact_ids"`
	ComparisonIDs []string `json:"comparison_ids"`
	Missing       []string `json:"missing"`
}
type ReportFact struct {
	ID      string       `json:"id"`
	Metric  string       `json:"metric"`
	Value   string       `json:"value"`
	Unit    string       `json:"unit"`
	Scope   string       `json:"scope"`
	Measure string       `json:"measure"`
	Nature  string       `json:"nature"`
	Period  ReportPeriod `json:"period"`
	Source  ReportSource `json:"source"`
}
type ReportPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
type ReportSource struct {
	DocumentID string `json:"document_id"`
	Quote      string `json:"quote"`
	PeriodText string `json:"period_text"`
}
type ReportComparison struct {
	ID       string `json:"id"`
	Current  string `json:"current"`
	Baseline string `json:"baseline"`
}
type ReportIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type FactCheck struct {
	ID             string `json:"id"`
	Group          string `json:"group"`
	QuoteMatched   bool   `json:"quote_matched"`
	OperationID    string `json:"operation_id,omitempty"`
	Version        string `json:"version,omitempty"`
	QuoteOffset    int    `json:"quote_offset"`
	SourceURL      string `json:"source_url,omitempty"`
	PublishedLabel string `json:"published_label,omitempty"`
	CapturedLabel  string `json:"captured_label,omitempty"`
	Consistent     bool   `json:"consistent"`
}
type ComparisonCheck struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Difference string `json:"difference"`
	Unit       string `json:"unit"`
	Consistent bool   `json:"consistent"`
}
type ReportCheck struct {
	Validator          string            `json:"validator"`
	ResultSHA256       string            `json:"result_sha256"`
	ClassificationDate string            `json:"classification_date"`
	Status             string            `json:"status"`
	Notice             string            `json:"notice"`
	Report             *ResearchReport   `json:"report,omitempty"`
	Facts              []FactCheck       `json:"facts"`
	Comparisons        []ComparisonCheck `json:"comparisons"`
	Issues             []ReportIssue     `json:"issues"`
}
type retainedPart struct {
	OperationID     string `json:"operation_id"`
	DocumentID      string `json:"document_id"`
	Text            string `json:"text"`
	Offset          int    `json:"offset"`
	NextOffset      *int   `json:"next_offset"`
	Version         string `json:"version"`
	ReadOffset      int    `json:"read_offset"`
	ReadEnd         int    `json:"read_end"`
	Source          string `json:"source"`
	SourceTruncated bool   `json:"source_truncated"`
}
type quoteProof struct {
	Operation, Version             string
	Offset                         int
	SourceURL, Published, Captured string
}
type quoteReader func(context.Context, ReportSource) (quoteProof, error)

var reportID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,119}$`)
var receiptHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var decimal = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,14})(\.[0-9]{1,6})?$`)
var numberToken = regexp.MustCompile(`-?[0-9]+(?:\.[0-9]+)?`)
var uncertainWords = regexp.MustCompile(`预估|预计|估计|预测|预期|计划|拟建|拟于`)
var estimateWords = regexp.MustCompile(`预估|预计|估计|预测|预期`)
var planWords = regexp.MustCompile(`计划|拟建|拟于`)
var unsupportedAssertion = regexp.MustCompile(`主要发生|主要集中|导致|必然|全库|不存在|没有任何|均无|全部缺失|已经兑现|已全部兑现`)
var sameBasisStatement = regexp.MustCompile(`(?:^|[。；\n])同口径[，：]`)
var temporalLabel = regexp.MustCompile(`[0-9]{1,4}(?:年|月|日|周|季度)`)
var proseQuantity = regexp.MustCompile(`(?:[0-9]+(?:\.[0-9]+)?|[零一二三四五六七八九十两][零一二三四五六七八九十百千万亿两点]*)\s*(?:万吨|吨|千克|公斤|亿|万|元|%|％|‰|倍|桶|立方米|蒲式耳|个百分点)|[0-9]+手|百分之[0-9零一二三四五六七八九十百千万亿两点]+`)

func validDate(value string) (time.Time, bool) {
	t, e := time.Parse("2006-01-02", value)
	return t, e == nil && t.Format("2006-01-02") == value
}
func compact(value string) string { return strings.Join(strings.Fields(value), "") }
func bounded(value string, n int) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= n
}
func issuePath(section string, index int, id string) string {
	if reportID.MatchString(id) {
		return section + "." + id
	}
	return fmt.Sprintf("%s[%d]", section, index)
}

// Reject duplicate keys as well as unknown fields. A last-key-wins parser would
// permit two readers to disagree about the authoritative value in one report.
func uniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 12 {
			return errors.New("report nesting limit")
		}
		t, e := d.Token()
		if e != nil {
			return e
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			keys := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := key.(string)
				if !ok || keys[s] {
					return errors.New("duplicate report field")
				}
				keys[s] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
		} else if delim == '[' {
			for d.More() {
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
		} else {
			return errors.New("unexpected delimiter")
		}
		_, e = d.Token()
		return e
	}
	if e := walk(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return errors.New("extra report content")
	}
	return nil
}
func parseReport(raw string) (*ResearchReport, error) {
	if len(raw) > 96<<10 {
		return nil, errors.New("report size limit")
	}
	if e := uniqueJSON([]byte(raw)); e != nil {
		return nil, e
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	var out ResearchReport
	if e := d.Decode(&out); e != nil {
		return nil, e
	}
	if out.Schema != reportSchema || out.Facts == nil || out.Comparisons == nil || len(out.Facts) > 40 || len(out.Comparisons) > 30 || len(out.Questions) < 1 || len(out.Questions) > 8 {
		return nil, errors.New("unsupported report schema or count")
	}
	for _, q := range out.Questions {
		if q.FactIDs == nil || q.ComparisonIDs == nil || q.Missing == nil {
			return nil, errors.New("question arrays required")
		}
	}
	return &out, nil
}

// Conservative v1 period syntax. Ambiguous yearless dates are not guessed.
// The matched phrase must be a literal part of the retained quote.
func sourcePeriod(value string) (ReportPeriod, bool) {
	v := compact(value)
	if _, ok := validDate(v); ok {
		return ReportPeriod{v, v}, true
	}
	patterns := []struct {
		re   string
		kind string
	}{
		{`^([0-9]{4})年([0-9]{1,2})月([0-9]{1,2})日$`, "day"},
		{`^([0-9]{4})年([0-9]{1,2})月份?$`, "month"},
		{`^([0-9]{4})年$`, "year"},
		{`^([0-9]{4})年([0-9]{1,2})月底$`, "month_end"},
	}
	for _, p := range patterns {
		m := regexp.MustCompile(p.re).FindStringSubmatch(v)
		if m == nil {
			continue
		}
		y, _ := strconv.Atoi(m[1])
		mo, day := 1, 1
		if len(m) > 2 {
			mo, _ = strconv.Atoi(m[2])
		}
		if len(m) > 3 {
			day, _ = strconv.Atoi(m[3])
		}
		if mo < 1 || mo > 12 {
			return ReportPeriod{}, false
		}
		start := time.Date(y, time.Month(mo), day, 0, 0, 0, 0, time.UTC)
		if start.Day() != day {
			return ReportPeriod{}, false
		}
		end := start
		switch p.kind {
		case "month":
			end = start.AddDate(0, 1, -1)
		case "year":
			end = start.AddDate(1, 0, -1)
		case "month_end":
			start = start.AddDate(0, 1, -1)
			end = start
		}
		return ReportPeriod{start.Format("2006-01-02"), end.Format("2006-01-02")}, true
	}
	// Both range endpoints must name their year, so Dec→Jan is unambiguous.
	for _, separator := range []string{"至", "到", "~", "～", "—"} {
		if parts := strings.Split(v, separator); len(parts) == 2 {
			a, ok := sourcePeriod(parts[0])
			b, ok2 := sourcePeriod(parts[1])
			if ok && ok2 && a.Start == a.End && b.Start == b.End && a.Start <= b.Start {
				return ReportPeriod{a.Start, b.End}, true
			}
		}
	}
	return ReportPeriod{}, false
}
func dateGroup(f ReportFact, asOf string) string {
	suffix := map[string]string{"reported": "来源报数", "estimate": "第三方估计", "plan": "披露计划"}[f.Nature]
	if f.Period.End < asOf {
		return "已结束区间 · " + suffix
	}
	if f.Period.Start > asOf {
		return "未来区间 · " + suffix
	}
	return "覆盖基准日 · " + suffix
}
func amountPresent(value, quote string) bool {
	for _, token := range numberToken.FindAllString(quote, -1) {
		if token == value {
			return true
		}
	}
	return false
}
func factExpression(f ReportFact) string {
	// A deliberately small grammar, NOT a general-purpose semantic verifier.
	// Date and subject must be adjacent to this metric/amount within one clause.
	// Do not carry either across another amount, omitted year, or new subject.
	period, scope := regexp.QuoteMeta(compact(f.Source.PeriodText)), regexp.QuoteMeta(compact(f.Scope))
	join := `[，,：:]?`
	context := `(?:` + period + join + scope + `|` + scope + join + `(?:截至)?` + period + `)`
	modality := `(?:预估|预计|估计|预测|预期|计划|拟建|拟于)?`
	metric := regexp.QuoteMeta(compact(f.Metric))
	amount := regexp.QuoteMeta(f.Value) + regexp.QuoteMeta(compact(f.Unit))
	pattern := `(?:^|[。；;！？!?])(?:(?:同口径|不同口径)[，,:：])?(?:截至|在)?` + context + join + `(?:的)?` + modality + metric + modality + `(?:为|是|达到|达|约为|为约|量为|[:：])?` + amount + `(?:[，,。；;！？!?]|$)`
	return regexp.MustCompile(pattern).FindString(compact(f.Source.Quote))
}

func comparableExpressions(current, baseline ReportFact) bool {
	if current.Source.DocumentID != baseline.Source.DocumentID || current.Source.Quote != baseline.Source.Quote {
		return false
	}
	quote := compact(current.Source.Quote)
	if strings.Contains(quote, "不同口径") {
		return false
	}
	first, second := factExpression(baseline), factExpression(current)
	// Bind the declaration to this adjacent pair, not another observation.
	return first != "" && second != "" && sameBasisStatement.MatchString(first) &&
		strings.Contains(quote, first+strings.TrimLeft(second, "。；;！？!?"))
}
func checkReport(ctx context.Context, raw, asOf string, read quoteReader) ReportCheck {
	digest := sha256.Sum256([]byte(raw))
	out := ReportCheck{Validator: "futures-report-check-v2", ResultSHA256: hex.EncodeToString(digest[:]), ClassificationDate: asOf, Status: "needs_review", Notice: "基准日用于过去/未来分类，不代表当日已知信息。检查仅覆盖所支持的结构、时间表达、算术及留存引用；不证明来源真实性、完整性、样本稳定或独立交叉核验。", Facts: []FactCheck{}, Comparisons: []ComparisonCheck{}, Issues: []ReportIssue{}}
	add := func(path, code, msg string) { out.Issues = append(out.Issues, ReportIssue{path, code, msg}) }
	report, e := parseReport(raw)
	if e != nil {
		out.Status = "unstructured"
		add("report", "format", "原始回答未符合结构化资料格式，不能作为已核验报告。")
		return out
	}
	out.Report = report
	if _, ok := validDate(asOf); !ok || report.AsOf != asOf {
		add("as_of", "cutoff", "报告时间分类基准日与提交时固定日期不一致或缺少固定日期。")
		return out
	}
	facts := map[string]ReportFact{}
	consistent := map[string]bool{}
	for index, f := range report.Facts {
		startIssues := len(out.Issues)
		path := issuePath("facts", index, f.ID)
		check := FactCheck{ID: f.ID, QuoteOffset: -1}
		if !reportID.MatchString(f.ID) || facts[f.ID].ID != "" {
			add(path, "identity", "事实编号无效或重复。")
		} else {
			facts[f.ID] = f
		}
		if !bounded(f.Metric, 100) || !bounded(f.Scope, 200) || !bounded(f.Unit, 30) || !decimal.MatchString(f.Value) {
			add(path, "fields", "指标、样本、单位和十进制数值必须明确且有界。")
		}
		if f.Measure != "stock" && f.Measure != "flow" && f.Measure != "cumulative_flow" && f.Measure != "capacity" && f.Measure != "ratio" && f.Measure != "commitment" {
			add(path, "measure", "需区分实物库存、区间流量、累计流量、产能、比率和未执行合同。")
		}
		if temporalLabel.MatchString(f.Metric) || temporalLabel.MatchString(f.Scope) {
			add(path, "period_in_label", "指标和样本名称应稳定；具体统计日期放在 period，避免同一指标因月份不同而无法比较。")
		}
		ratioUnit := f.Unit == "%" || f.Unit == "％" || f.Unit == "‰" || f.Unit == "倍"
		if ratioUnit && f.Measure != "ratio" || f.Measure == "ratio" && !ratioUnit || (strings.Contains(f.Metric, "开机率") || strings.Contains(f.Metric, "产能利用率")) && f.Measure != "ratio" {
			add(path, "ratio_measure", "比率与产能总量不同；开机率、利用率及百分比应标为 ratio，并保留原文的比率单位。")
		}
		if (strings.Contains(f.Metric, "未执行合同") || strings.Contains(f.Metric, "未履约合同")) && f.Measure != "commitment" {
			add(path, "commitment_measure", "未执行或未履约合同不是实物库存，应标为 commitment。")
		}
		if f.Nature != "reported" && f.Nature != "estimate" && f.Nature != "plan" {
			add(path, "nature", "资料属性无效。")
		}
		_, startOK := validDate(f.Period.Start)
		_, endOK := validDate(f.Period.End)
		if !startOK || !endOK || f.Period.Start > f.Period.End {
			add(path, "period", "统计日历区间无效。")
		}
		if f.Measure == "stock" && f.Period.Start != f.Period.End {
			add(path, "stock_period", "库存需使用明确时点，不能以区间总量表示。")
		}
		if f.Measure == "commitment" && f.Period.Start != f.Period.End {
			add(path, "commitment_period", "未执行合同余额需对应明确时点；区间签约量属于另一指标。")
		}
		if f.Nature == "reported" && f.Period.End > asOf {
			add(path, "future_actual", "基准日之后尚未结束的区间不能作为该日期前已发生实绩。")
		}
		if !reportID.MatchString(f.Source.DocumentID) || !bounded(f.Source.Quote, 2400) || !bounded(f.Source.PeriodText, 100) {
			add(path, "source", "必须提供本任务读过的文档与有界原文。")
		} else {
			q := compact(f.Source.Quote)
			expression := factExpression(f)
			if expression == "" || !amountPresent(f.Value, expression) {
				add(path, "anchor", "尚未匹配同一明确句式中的日期、样本、指标、数值和单位；需回查原文，不凭整段共现确认关系。")
			}
			p, ok := sourcePeriod(f.Source.PeriodText)
			if !ok || p != f.Period || !strings.Contains(q, compact(f.Source.PeriodText)) {
				add(path, "period_anchor", "现有日期规则尚未确认所填区间与引用一致；需回查年份和统计窗口。")
			}
			if f.Nature == "reported" && uncertainWords.MatchString(expression) {
				add(path, "estimated_as_actual", "引用包含估计或计划表述，不能直接标为实绩。")
			}
			if f.Nature == "estimate" && (!estimateWords.MatchString(expression) || planWords.MatchString(expression)) {
				add(path, "estimate_anchor", "现有规则尚未确认估计表述与这项报数的对应关系；需结合原文核对。")
			}
			if f.Nature == "plan" && (!planWords.MatchString(expression) || estimateWords.MatchString(expression)) {
				add(path, "plan_anchor", "现有规则尚未确认计划表述与这项报数的对应关系；需结合原文核对。")
			}
			// A wrong interpretation should not hide its genuine original quote.
			// Matching a receipt remains independent of checking the fact fields.
			if read != nil {
				proof, err := read(ctx, f.Source)
				if err != nil {
					add(path, "retained_quote", "未能在本任务有界留存资料中逐字确认该引用；可能是原文不符、未读取、超限或服务不可用。")
				} else {
					check.QuoteMatched = true
					check.OperationID = proof.Operation
					check.Version = proof.Version
					check.QuoteOffset = proof.Offset
					check.SourceURL, check.PublishedLabel, check.CapturedLabel = proof.SourceURL, proof.Published, proof.Captured
				}
			}
		}
		check.Group = dateGroup(f, asOf)
		check.Consistent = read != nil && len(out.Issues) == startIssues
		consistent[f.ID] = check.Consistent
		out.Facts = append(out.Facts, check)
	}
	comparisons := map[string]bool{}
	for index, c := range report.Comparisons {
		path := issuePath("comparisons", index, c.ID)
		check := ComparisonCheck{ID: c.ID}
		a, aOK := facts[c.Current]
		b, bOK := facts[c.Baseline]
		if !reportID.MatchString(c.ID) || comparisons[c.ID] {
			add(path, "identity", "比较编号无效或重复。")
		} else if !aOK || !bOK || c.Current == c.Baseline {
			add(path, "operands", "比较只能引用两项存在且不同的事实。")
		} else if a.Metric != b.Metric || a.Unit != b.Unit || a.Scope != b.Scope || a.Measure != b.Measure || a.Nature != b.Nature {
			add(path, "basis", "指标、单位、样本、数量类型或资料属性不一致，禁止直接相减。")
		} else if b.Period.End >= a.Period.Start {
			add(path, "overlap", "比较区间重叠或前后顺序不成立。")
		} else if read != nil && (!consistent[c.Current] || !consistent[c.Baseline]) {
			add(path, "operands", "比较只能引用两项检查一致的不同事实。")
		} else if !comparableExpressions(a, b) {
			add(path, "comparability", "未取得同份原文中的明确同口径对照；相同样本文字不证明两期可比。")
		} else if read != nil && decimal.MatchString(a.Value) && decimal.MatchString(b.Value) {
			av, _ := new(big.Rat).SetString(a.Value)
			bv, _ := new(big.Rat).SetString(b.Value)
			check.Difference = strings.TrimRight(strings.TrimRight(new(big.Rat).Sub(av, bv).FloatString(6), "0"), ".")
			if check.Difference == "-0" {
				check.Difference = "0"
			}
			check.From = b.Period.Start + " — " + b.Period.End
			check.To = a.Period.Start + " — " + a.Period.End
			check.Unit = a.Unit
			if a.Measure == "ratio" && (a.Unit == "%" || a.Unit == "％") {
				check.Unit = "个百分点"
			}
			check.Consistent = true
		}
		comparisons[c.ID] = true
		out.Comparisons = append(out.Comparisons, check)
	}
	questions := map[string]bool{}
	for index, q := range report.Questions {
		path := issuePath("questions", index, q.ID)
		if proseQuantity.MatchString(q.Question) {
			add(path, "prose_quantity", "问题文字中的数量没有逐项来源绑定；请将报数保留为 facts 并关联 fact_ids，问题本身使用中性问句。")
		}
		if unsupportedAssertion.MatchString(q.Question) {
			add(path, "unsupported_assertion", "问题文本含未经验证的归因或整体判断，请改为中性的待查问题。")
		}
		if !reportID.MatchString(q.ID) || questions[q.ID] || !bounded(q.Question, 400) || len(q.FactIDs) > 40 || len(q.ComparisonIDs) > 30 || len(q.Missing) > 12 {
			add(path, "question", "问题或引用数量超限、内容为空或编号重复。")
		}
		questions[q.ID] = true
		for _, id := range q.FactIDs {
			if _, ok := facts[id]; !ok {
				add(path, "fact_reference", "问题引用了不存在的事实。")
			}
		}
		// Catch explicit relabelling, not general prose entailment. A question
		// about missing plans remains possible in Missing; findings belong in facts.
		if planWords.MatchString(q.Question) && len(q.FactIDs) > 0 {
			hasPlan := false
			for _, id := range q.FactIDs {
				hasPlan = hasPlan || facts[id].Nature == "plan"
			}
			if !hasPlan {
				add(path, "prose_nature", "问题提及计划，但关联事实没有计划属性；不要把预估或实绩改称计划，需核对表述和事实关联。")
			}
		}
		for _, id := range q.ComparisonIDs {
			if !comparisons[id] {
				add(path, "comparison_reference", "问题引用了不存在的比较。")
			}
		}
		for _, v := range q.Missing {
			if proseQuantity.MatchString(v) {
				add(path, "prose_quantity", "待查描述不能夹带未逐项核对的报数；已有数值放入 facts，不能为消除提示删掉关键数据。")
			}
			if unsupportedAssertion.MatchString(v) {
				add(path, "unsupported_assertion", "待查项不能声称全库、外部世界不存在数据或已有因果结论。")
			}
			if !bounded(v, 400) {
				add(path, "missing", "待查资料描述为空或超限。")
			}
		}
		if len(q.FactIDs) == 0 && len(q.Missing) == 0 {
			add(path, "empty", "问题必须列出对应证据或具体待查资料。")
		}
	}
	if read == nil {
		add("report", "retained_unchecked", "此为草稿检查，未读取任务回执。原文存在性、真实性及报告完整性尚未验证。")
	}
	if len(out.Issues) == 0 && read != nil {
		out.Status = "consistent"
	}
	return out
}

// Per-request limits bound HTTP count and memory; nothing is persisted here.
type reportEvidenceReader struct {
	server          *Server
	base            string
	requests, bytes int
	cache           map[string][]retainedPart
}

func (r *reportEvidenceReader) request(ctx context.Context, uri string, out any) error {
	r.requests++
	if r.requests > 48 {
		return errors.New("receipt lookup limit")
	}
	return r.server.Client.Request(ctx, "GET", r.base+uri, nil, out)
}
func (r *reportEvidenceReader) quote(ctx context.Context, source ReportSource) (quoteProof, error) {
	parts, known := r.cache[source.DocumentID]
	if !known {
		var list struct {
			Items []retainedPart `json:"items"`
			Next  string         `json:"next_cursor"`
		}
		if e := r.request(ctx, "/evidence?document_id="+url.QueryEscape(source.DocumentID)+"&limit=8", &list); e != nil {
			return quoteProof{}, e
		}
		for _, item := range list.Items {
			if item.DocumentID != source.DocumentID || item.OperationID == "" || len(item.OperationID) > 512 {
				return quoteProof{}, errors.New("receipt identity mismatch")
			}
			var joined retainedPart
			offset, version := 0, ""
			for {
				var part retainedPart
				q := "/evidence/record?operation_id=" + url.QueryEscape(item.OperationID) + "&limit=8192&offset=" + strconv.Itoa(offset)
				if version != "" {
					q += "&version=" + version
				}
				if e := r.request(ctx, q, &part); e != nil {
					return quoteProof{}, e
				}
				if part.OperationID != item.OperationID || part.DocumentID != source.DocumentID || part.Offset != offset || !receiptHash.MatchString(part.Version) || (version != "" && version != part.Version) || utf8.RuneCountInString(part.Text) > 8192 || part.ReadOffset < 0 || part.ReadEnd < part.ReadOffset || (offset > 0 && (part.ReadOffset != joined.ReadOffset || part.ReadEnd != joined.ReadEnd)) {
					return quoteProof{}, errors.New("receipt segment mismatch")
				}
				r.bytes += len(part.Text)
				if r.bytes > 256<<10 {
					return quoteProof{}, errors.New("receipt byte limit")
				}
				if offset == 0 {
					joined = part
				} else {
					joined.Text += part.Text
				}
				version = part.Version
				if part.NextOffset == nil {
					if utf8.RuneCountInString(joined.Text) != part.ReadEnd-part.ReadOffset {
						return quoteProof{}, errors.New("receipt full range mismatch")
					}
					break
				}
				if *part.NextOffset != offset+utf8.RuneCountInString(part.Text) || *part.NextOffset <= offset {
					return quoteProof{}, errors.New("receipt cursor mismatch")
				}
				offset = *part.NextOffset
			}
			parts = append(parts, joined)
		}
		r.cache[source.DocumentID] = parts
	}
	for _, p := range parts {
		if offset := strings.Index(p.Text, source.Quote); offset >= 0 {
			proof := quoteProof{Operation: p.OperationID, Version: p.Version, Offset: p.ReadOffset + utf8.RuneCountInString(p.Text[:offset])}
			// Labels are copied from the same retained read, never guessed from
			// today's Knowledge state or promoted to independently verified dates.
			if p.ReadOffset == 0 {
				lines := strings.Split(p.Text, "\n")
				if !strings.HasSuffix(p.Text, "\n") {
					lines = lines[:len(lines)-1]
				}
				if len(lines) > 16 {
					lines = lines[:16]
				}
				for _, line := range lines {
					for prefix, target := range map[string]*string{"来源：": &proof.SourceURL, "发布日期：": &proof.Published, "原始采集时间：": &proof.Captured} {
						if strings.HasPrefix(line, prefix) {
							value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
							if len(value) <= 2048 {
								*target = value
							}
						}
					}
				}
			}
			if proof.SourceURL == "" && !p.SourceTruncated {
				proof.SourceURL = p.Source
			}
			if u, e := url.Parse(proof.SourceURL); e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(proof.SourceURL) > 2048 {
				proof.SourceURL = ""
			}
			return proof, nil
		}
	}
	return quoteProof{}, errors.New("quote not found within loaded retained receipts")
}
func (s *Server) reportRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks/{id}/report-check", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		if !reportID.MatchString(r.PathValue("id")) || r.URL.RawQuery != "" {
			return &servicehttp.StatusError{Status: 400, Message: "invalid task report request"}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		base := s.Client.RuntimeURL + "/v1/apps/futures/runs/" + url.PathEscape(r.PathValue("id"))
		var task struct {
			ID       string            `json:"id"`
			Status   string            `json:"status"`
			Result   string            `json:"result"`
			Metadata map[string]string `json:"metadata"`
		}
		if e := s.Client.Request(ctx, "GET", base, nil, &task); e != nil {
			return e
		}
		if task.ID != r.PathValue("id") {
			return errors.New("task identity mismatch")
		}
		reader := &reportEvidenceReader{server: s, base: base, cache: map[string][]retainedPart{}}
		asOf := task.Metadata["as_of"]
		if task.Metadata["report_format"] != reportSchema {
			asOf = ""
		}
		result := checkReport(ctx, task.Result, asOf, reader.quote)
		w.Header().Set("Cache-Control", "no-store")
		servicehttp.JSONResponse(w, 200, result)
		return nil
	}))
}

func reportInstructions(asOf string) string {
	return fmt.Sprintf(`
时间分类基准日固定为 %s，用于过去/未来分类，不代表历史时点已知信息回测。最后回答只输出一个 JSON 对象（不加代码围栏）：
{"schema":"futures-evidence-v1","as_of":"%s","questions":[{"id":"q1","question":"待核核心问题","fact_ids":["f1"],"comparison_ids":[],"missing":["仍需调查的具体资料"]}],"facts":[{"id":"f1","metric":"原文指标名称","value":"十进制数值","unit":"原文单位","scope":"原文样本范围","measure":"stock 或 flow 或 cumulative_flow 或 capacity 或 ratio 或 commitment","nature":"reported 或 estimate 或 plan","period":{"start":"YYYY-MM-DD","end":"YYYY-MM-DD"},"source":{"document_id":"实际document_read的ID","quote":"逐字复制已读取原文，不添加或改写任何字符","period_text":"quote内明确带年份的原文日期"}}],"comparisons":[{"id":"c1","current":"后一期fact编号","baseline":"前一期fact编号"}]}
quote 必须包含支持该报数的连续原文，最多2400字符，不能跨日期或样本借用其他数字。period_text使用明确带年份的原文日期；含糊日期不要猜测。最多40事实/8问题/30比较。自动检查只支持有限明确句式，未识别会标待核：不能为凑格式改写原文或删掉关键事实。估计、计划必须单列其nature，过去估计也不是实绩；库存是时点，压榨量是区间流量，累计流量另标cumulative_flow。开机率等比率使用ratio；时点未执行合同使用commitment，不能当实物库存。metric/scope使用稳定指标/样本名，日期只放period。百分比的前后差额是百分点。比较只有同一来源明确声明同口径且保留完整支持原文时才可能自动计算；没有声明则列待核口径，不能补写“同口径”。问题与缺口文字保持中性，不夹带报数或已证实结论：所有报数留在facts并关联fact_ids，不能把预估改称计划，不能为消除提示删掉关键数据。不要输出自由总结、方向预测、因果结论或全库缺失断言；未证实的解释保持问题与待查资料。保留问题与证据关系，先查关键资料，缺口按已授权公开来源继续调查，不能为过格式校验而删除问题。若绑定了check_research_draft_v2工具，最终输出前提交完整草稿和本任务as_of，依据revise修订并回查source_review；最多两轮草稿检查。无法确认的项保留待核，不改原文、不删核心问题、不声称检查工具证明事实真实。`, asOf, asOf)
}
