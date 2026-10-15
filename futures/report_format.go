package futures

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// Format diagnostics never copy input values or decoder error strings. The
// outer format code and report schema stay fixed; current readers get a path
// and a bounded repair hint, not a repaired or partially accepted report.
type reportFormatError struct {
	path    string
	message string
}

func (e *reportFormatError) Error() string { return e.message }

func formatError(path, message string) error {
	if path == "" {
		path = "report"
	}
	return &reportFormatError{path, message}
}

func jsonFieldPath(parent, field string) string {
	// Normal schema names are useful for repair. Arbitrary names may contain
	// pasted source text, controls or credentials and must not enter feedback.
	if len(field) == 0 || len(field) > 64 {
		field = "[字段名省略]"
	} else {
		for _, c := range field {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
				field = "[字段名省略]"
				break
			}
		}
	}
	if parent == "" {
		return field
	}
	return parent + "." + field
}

func reportFormatIssue(raw string, err error) (string, string) {
	var diagnostic *reportFormatError
	if errors.As(err, &diagnostic) {
		return diagnostic.path, diagnostic.message
	}
	var syntax *json.SyntaxError
	isSyntax := errors.As(err, &syntax)
	// Token and Decode can report truncation differently. Recognize the
	// standard library's EOF SyntaxError without exposing decoder text.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || isSyntax && syntax.Error() == "unexpected end of JSON input" {
		return "report", "JSON 未结束：" + jsonByteLocation(raw, int64(len(raw)+1)) + "（输入末尾）；请核对字符串、数组及对象是否完整闭合。原文未被修改。"
	}
	if isSyntax {
		return "report", "JSON 语法错误：" + jsonByteLocation(raw, syntax.Offset) + "；请检查该位置的引号、分隔符或闭合符号。原文未被修改。"
	}
	return "report", "原始回答未符合结构化资料格式，不能作为已核验报告。"
}

func jsonByteLocation(raw string, offset int64) string {
	// json.SyntaxError offsets and these displayed columns count UTF-8 bytes,
	// not characters. EOF points one byte after the supplied input.
	if offset < 1 {
		offset = 1
	}
	if offset > int64(len(raw)+1) {
		offset = int64(len(raw) + 1)
	}
	prefix := raw[:int(offset)-1]
	line := strings.Count(prefix, "\n") + 1
	column := len(prefix) - strings.LastIndex(prefix, "\n")
	return fmt.Sprintf("UTF-8 第 %d 字节（第 %d 行，第 %d 字节列）", offset, line, column)
}

// Locate unknown fields and type mismatches before the strict struct decoder.
// Reflection derives the allowed shape from the existing JSON contract rather
// than maintaining a second schema. Counts and required arrays are checked by
// parseReport; null keeps encoding/json's historical zero-value semantics.
func checkReportJSONShape(raw []byte, expected reflect.Type, path string) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) && path != "" {
		return nil
	}
	for expected.Kind() == reflect.Pointer {
		expected = expected.Elem()
	}
	switch expected.Kind() {
	case reflect.Struct:
		if len(raw) == 0 || raw[0] != '{' {
			return formatError(path, "此处必须为对象；不能以字符串、数组或其他类型代替。")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		types := map[string]reflect.Type{}
		for i := 0; i < expected.NumField(); i++ {
			field := expected.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				types[name] = field.Type
			}
		}
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sort.Strings(names) // Stable feedback even when multiple fields are wrong.
		for _, name := range names {
			t, known := types[name]
			if !known {
				// Match the existing encoding/json case-insensitive behavior.
				for canonical, typ := range types {
					if strings.EqualFold(canonical, name) {
						t, known = typ, true
						break
					}
				}
			}
			child := jsonFieldPath(path, name)
			if !known {
				return formatError(child, "不支持的字段；请使用 futures-evidence-v1 已声明的字段，保留事实及原文，不自行扩展对象。")
			}
			if err := checkReportJSONShape(fields[name], t, child); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if len(raw) == 0 || raw[0] != '[' {
			return formatError(path, "此处必须为数组；没有项目时使用 []，不能以对象或字符串代替。")
		}
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		for i, value := range values {
			if err := checkReportJSONShape(value, expected.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.String:
		if len(raw) == 0 || raw[0] != '"' {
			return formatError(path, "此处必须为字符串；数值字段 value 也使用十进制字符串，不以 JSON 数字、数组或对象代替。")
		}
	}
	return nil
}
