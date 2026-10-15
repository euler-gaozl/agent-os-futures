package futures

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestReportFormatFeedbackLocatesRepairWithoutEchoingInput(t *testing.T) {
	raw := encodeReport(t, exampleReport())
	comparison := encodeReport(t, comparisonReport())
	for _, tc := range []struct {
		name, raw, path, message string
	}{
		{"unfinished object", raw[:len(raw)-1], "report", "JSON 未结束"},
		{"unknown comparison field", strings.Replace(comparison, `"baseline":"old"`, `"baseline":"old","basis":"PRIVATE_FIXTURE_VALUE"`, 1), "comparisons[0].basis", "不支持的字段"},
		{"number instead of string", strings.Replace(raw, `"value":"1018.55"`, `"value":1018.55`, 1), "facts[0].value", "必须为字符串"},
		{"object instead of array", strings.Replace(raw, `"comparisons":[]`, `"comparisons":{}`, 1), "comparisons", "必须为数组"},
		{"non-object fact", strings.Replace(raw, `"facts":[{`, `"facts":["PRIVATE_FIXTURE_VALUE",{`, 1), "facts[0]", "必须为对象"},
		{"duplicate key", strings.Replace(raw, `"schema":`, `"schema":"PRIVATE_FIXTURE_VALUE","schema":`, 1), "schema", "重复字段"},
		{"escaped duplicate key", strings.Replace(raw, `"schema":`, `"`+string(rune(92))+`u0073chema":"PRIVATE_FIXTURE_VALUE","schema":`, 1), "schema", "重复字段"},
		{"wrong schema", strings.Replace(raw, reportSchema, "PRIVATE_FIXTURE_VALUE", 1), "schema", reportSchema},
		{"null facts", `{"schema":"futures-evidence-v1","questions":[{"id":"q1","question":"?","fact_ids":[],"comparison_ids":[],"missing":[]}],"facts":null,"comparisons":[]}`, "facts", "必须提供数组"},
		{"missing question array", strings.Replace(raw, `"fact_ids":["f1"],`, ``, 1), "questions[0].fact_ids", "必须提供数组"},
		{"null question array", strings.Replace(raw, `"missing":["各月实际到港量及统计口径"]`, `"missing":null`, 1), "questions[0].missing", "必须提供数组"},
		{"too few questions", `{"schema":"futures-evidence-v1","questions":[],"facts":[],"comparisons":[]}`, "questions", "1–8"},
		{"extra JSON value", raw + ` {"PRIVATE_FIXTURE_VALUE":true}`, "report", "多余内容"},
		{"root scalar", `"PRIVATE_FIXTURE_VALUE"`, "report", "必须为对象"},
		{"depth limit", strings.Repeat("[", 14) + "0" + strings.Repeat("]", 14), "report", "嵌套深度"},
		{"size limit", strings.Repeat(" ", (96<<10)+1), "report", "98304"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := checkReport(context.Background(), tc.raw, "2026-10-09", nil)
			if c.Status != "unstructured" || c.Report != nil || len(c.Issues) != 1 || c.Issues[0].Code != "format" || c.Issues[0].Path != tc.path || !strings.Contains(c.Issues[0].Message, tc.message) {
				t.Fatalf("missing precise format feedback: %+v", c.Issues)
			}
			if strings.Contains(c.Issues[0].Message, "PRIVATE_FIXTURE_VALUE") || strings.Contains(c.Issues[0].Path, "PRIVATE_FIXTURE_VALUE") {
				t.Fatal("format diagnostic echoed an input value")
			}
			f := reviewDraft(context.Background(), tc.raw, "2026-10-09")
			if f.Status != "revision_required" || len(f.Revise) != 1 || f.Revise[0].Code != "format" || f.Revise[0].Path != tc.path || f.Revise[0].Message != c.Issues[0].Message {
				t.Fatalf("MCP lost actionable feedback: %+v", f)
			}
		})
	}
}

func TestReportSyntaxFeedbackUsesOneBasedUTF8ByteLocations(t *testing.T) {
	raw := "{\n  \"说明\": \"合成\",\n  \"facts\": !\n}"
	position := strings.Index(raw, "!") + 1
	c := checkReport(context.Background(), raw, "2026-10-09", nil)
	if len(c.Issues) != 1 || !strings.Contains(c.Issues[0].Message, "JSON 语法错误") || !strings.Contains(c.Issues[0].Message, fmt.Sprintf("第 %d 字节", position)) || !strings.Contains(c.Issues[0].Message, "第 3 行，第 12 字节列") {
		t.Fatalf("syntax location is not actionable: %+v", c.Issues)
	}
	truncated := "{\n  \"schema\": \"futures-evidence-v1\"\n"
	c = checkReport(context.Background(), truncated, "2026-10-09", nil)
	if !strings.Contains(c.Issues[0].Message, fmt.Sprintf("第 %d 字节", len(truncated)+1)) || !strings.Contains(c.Issues[0].Message, "第 3 行，第 1 字节列") {
		t.Fatalf("EOF location did not point after all UTF-8 bytes: %+v", c.Issues)
	}
}

func TestReportFormatFeedbackDoesNotEchoUnboundedFieldNames(t *testing.T) {
	key := strings.Repeat("PRIVATE_FIELD_", 300)
	raw := strings.Replace(encodeReport(t, exampleReport()), `"schema":`, `"`+key+`":"PRIVATE_FIXTURE_VALUE","schema":`, 1)
	c := checkReport(context.Background(), raw, "2026-10-09", nil)
	if c.Status != "unstructured" || len(c.Issues) != 1 || !strings.Contains(c.Issues[0].Message, "不支持的字段") || len(c.Issues[0].Message)+len(c.Issues[0].Path) > 1000 || strings.Contains(c.Issues[0].Path, "PRIVATE_FIELD_") || strings.Contains(c.Issues[0].Message, "PRIVATE_FIXTURE_VALUE") {
		t.Fatalf("unbounded diagnostic: %+v", c.Issues)
	}
}

func TestReportSyntaxFeedbackLocatesDelimiterAndSeparatorErrors(t *testing.T) {
	for _, tc := range []struct {
		name, raw, offending string
		line, column         int
	}{
		{"wrong closing delimiter", "{\n  \"说明\": \"合成\",\n  \"facts\": [{\"id\":\"f1\"]\n}", "]", 3, 23},
		{"missing comma", "{\n  \"说明\": \"合成\"\n  \"facts\": []\n}", "\"facts\"", 3, 3},
		{"missing colon", "{\n  \"说明\": \"合成\",\n  \"facts\" []\n}", "[", 3, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			position := strings.Index(tc.raw, tc.offending) + 1
			c := checkReport(context.Background(), tc.raw, "2026-10-09", nil)
			want := fmt.Sprintf("UTF-8 第 %d 字节（第 %d 行，第 %d 字节列）", position, tc.line, tc.column)
			if c.Status != "unstructured" || c.Report != nil || len(c.Issues) != 1 || c.Issues[0].Code != "format" || !strings.Contains(c.Issues[0].Message, want) {
				t.Fatalf("token syntax location must identify the offending byte %q: %+v", want, c.Issues)
			}
		})
	}
}
