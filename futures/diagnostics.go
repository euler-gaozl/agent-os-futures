package futures

// The BFF re-projects Runtime failure facts before they reach the browser.
// Runtime already returns bounded, fixed messages; this layer independently
// whitelists codes, stages, identifiers and sizes so a compromised, downgraded
// or buggy Runtime cannot turn diagnostics into a leak of raw exception text,
// provider URLs or model content. Diagnostics are read-only and use no-store.

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
)

const taskDiagnosticsProtocol = "app-run-diagnostics-v1"
const taskFailureProtocol = "app-run-failures-v1"

var diagnosticStages = map[string]bool{"tool": true, "model": true, "execution": true, "cancel": true, "unknown": true}
var diagnosticCodes = map[string]bool{
	"parameter_rejected": true, "configuration_or_auth": true, "downstream_unavailable": true,
	"effect_unknown": true, "protocol_error": true, "budget_exhausted": true,
	"persistence_or_lease": true, "cancelled": true, "unknown": true,
}
var taskStatuses = map[string]bool{"queued": true, "running": true, "waiting_children": true, "waiting_unknown": true, "completed": true, "failed": true, "cancelled": true}
var taskFailureStatuses = map[string]bool{"failed": true, "cancelled": true, "waiting_unknown": true}
var diagnosticActions = map[string]bool{
	"correct_input": true, "contact_operator": true, "retry_later": true, "operator_review": true,
	"narrow_scope": true, "none": true,
}
var diagnosticRetry = map[string]bool{
	"manual_after_correction": true, "operator_after_fix": true, "manual_retry_later": true,
	"operator_review": true, "do_not_retry": true,
}

// Fixed local copy. Runtime's safe_message is not echoed; only the stable code
// crosses this boundary and the browser owns the Chinese explanation.
var diagnosticMessages = map[string]string{
	"parameter_rejected":     "A tool request was rejected as invalid.",
	"configuration_or_auth":  "A required configuration or credential is missing or was rejected.",
	"downstream_unavailable": "A downstream service was temporarily unavailable.",
	"effect_unknown":         "An external effect could not be confirmed.",
	"protocol_error":         "An execution protocol response was incomplete or invalid.",
	"budget_exhausted":       "An execution budget or context limit was reached.",
	"persistence_or_lease":   "Execution state persistence or ownership could not be confirmed.",
	"cancelled":              "The task was cancelled.",
	"unknown":                "The recorded failure does not identify a confirmed cause.",
}

var failureVersions = regexp.MustCompile(`^fvd1:[a-f0-9]{64}$`)
var operationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)
var toolPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

type diagnosticUsage struct {
	Attempts             int   `json:"attempts"`
	UnknownUsageAttempts int   `json:"unknown_usage_attempts"`
	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	CachedPromptTokens   int64 `json:"cached_prompt_tokens"`
	AccountedTokens      int64 `json:"accounted_tokens"`
	MaxAttempts          int   `json:"max_attempts"`
	TokenBudget          int   `json:"token_budget"`
}

type diagnosticParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type taskDiagnostics struct {
	Available         bool                 `json:"available"`
	Reason            string               `json:"reason,omitempty"`
	Protocol          string               `json:"protocol,omitempty"`
	RunID             string               `json:"run_id"`
	Status            string               `json:"status,omitempty"`
	FailureVersion    string               `json:"failure_version,omitempty"`
	ReportedAt        string               `json:"reported_at,omitempty"`
	Stage             string               `json:"stage,omitempty"`
	Code              string               `json:"code,omitempty"`
	SafeMessage       string               `json:"safe_message,omitempty"`
	CauseConfirmed    bool                 `json:"cause_confirmed"`
	RelatedEventIDs   []int64              `json:"related_event_ids,omitempty"`
	OperationID       string               `json:"operation_id,omitempty"`
	RelatedRole       string               `json:"related_role,omitempty"`
	Tool              string               `json:"tool,omitempty"`
	SafeParameter     *diagnosticParameter `json:"safe_parameter,omitempty"`
	RecommendedAction string               `json:"recommended_action,omitempty"`
	RetryPolicy       string               `json:"retry_policy,omitempty"`
	Usage             *diagnosticUsage     `json:"usage,omitempty"`
}

type taskFailureItem struct {
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	Commodity      string `json:"commodity,omitempty"`
	Period         string `json:"period,omitempty"`
	Stage          string `json:"stage"`
	Code           string `json:"code"`
	CauseConfirmed bool   `json:"cause_confirmed"`
	OperationID    string `json:"operation_id,omitempty"`
	Tool           string `json:"tool,omitempty"`
	FailureVersion string `json:"failure_version"`
}

type taskFailureCoverage struct {
	Since     string `json:"since"`
	Until     string `json:"until"`
	Scanned   int    `json:"scanned"`
	ScanLimit int    `json:"scan_limit"`
	Truncated bool   `json:"truncated"`
	Complete  bool   `json:"complete"`
}

type taskFailurePage struct {
	Protocol   string              `json:"protocol"`
	App        string              `json:"app"`
	Items      []taskFailureItem   `json:"items"`
	NextCursor string              `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
	Coverage   taskFailureCoverage `json:"coverage"`
}

type rawDiagnostics struct {
	Protocol          string               `json:"protocol"`
	RunID             string               `json:"run_id"`
	Status            string               `json:"status"`
	FailureVersion    string               `json:"failure_version"`
	ReportedAt        string               `json:"reported_at"`
	Stage             string               `json:"stage"`
	Code              string               `json:"code"`
	SafeMessage       string               `json:"safe_message"`
	CauseConfirmed    bool                 `json:"cause_confirmed"`
	RelatedEventIDs   []int64              `json:"related_event_ids"`
	OperationID       string               `json:"operation_id"`
	RelatedRole       string               `json:"related_role"`
	Tool              string               `json:"tool"`
	SafeParameter     *diagnosticParameter `json:"safe_parameter"`
	RecommendedAction string               `json:"recommended_action"`
	RetryPolicy       string               `json:"retry_policy"`
	Usage             *diagnosticUsage     `json:"usage"`
}

type rawFailurePage struct {
	Protocol string `json:"protocol"`
	App      string `json:"app"`
	Items    []struct {
		RunID          string `json:"run_id"`
		Status         string `json:"status"`
		CreatedAt      string `json:"created_at"`
		Commodity      string `json:"commodity"`
		Period         string `json:"period"`
		Stage          string `json:"stage"`
		Code           string `json:"code"`
		CauseConfirmed bool   `json:"cause_confirmed"`
		OperationID    string `json:"operation_id"`
		Tool           string `json:"tool"`
		FailureVersion string `json:"failure_version"`
	} `json:"items"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
	Coverage   struct {
		Since     string `json:"since"`
		Until     string `json:"until"`
		Scanned   int    `json:"scanned"`
		ScanLimit int    `json:"scan_limit"`
		Truncated bool   `json:"truncated"`
		Complete  bool   `json:"complete"`
	} `json:"coverage"`
}

func boundedText(s string, runes int) string {
	if utf8.RuneCountInString(s) <= runes {
		return s
	}
	return string([]rune(s)[:runes])
}

func fixedDiagnosticCode(code string) string {
	if diagnosticCodes[code] {
		return code
	}
	return "unknown"
}

func fixedStage(stage string) string {
	if diagnosticStages[stage] {
		return stage
	}
	return "unknown"
}

func fixedParameter(p *diagnosticParameter) *diagnosticParameter {
	if p == nil || (p.Name != "query" && p.Name != "keyword" && p.Name != "symbol" && p.Name != "commodity") {
		return nil
	}
	value := p.Value
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 80 || strings.TrimSpace(value) != value {
		return nil
	}
	lower := strings.ToLower(value)
	for _, blocked := range []string{"://", "bearer ", "token", "secret", "password", "api_key", "apikey"} {
		if strings.Contains(lower, blocked) {
			return nil
		}
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return nil
		}
	}
	return &diagnosticParameter{Name: p.Name, Value: value}
}

func fixedOperation(run, op string) string {
	if !operationPattern.MatchString(op) || !strings.HasPrefix(op, run+"/") {
		return ""
	}
	return op
}

func fixedUsage(u *diagnosticUsage) *diagnosticUsage {
	if u == nil {
		return nil
	}
	nonNegative := func(v int64) int64 {
		if v < 0 {
			return 0
		}
		return v
	}
	nonNegativeInt := func(v int) int {
		if v < 0 {
			return 0
		}
		return v
	}
	return &diagnosticUsage{
		Attempts:             nonNegativeInt(u.Attempts),
		UnknownUsageAttempts: nonNegativeInt(u.UnknownUsageAttempts),
		PromptTokens:         nonNegative(u.PromptTokens),
		CompletionTokens:     nonNegative(u.CompletionTokens),
		CachedPromptTokens:   nonNegative(u.CachedPromptTokens),
		AccountedTokens:      nonNegative(u.AccountedTokens),
		MaxAttempts:          nonNegativeInt(u.MaxAttempts),
		TokenBudget:          nonNegativeInt(u.TokenBudget),
	}
}

func fixedTimestamp(v string) string {
	if v == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func sanitizeDiagnostics(id string, raw rawDiagnostics) taskDiagnostics {
	out := taskDiagnostics{
		Available:      true,
		Protocol:       taskDiagnosticsProtocol,
		RunID:          id,
		Status:         "unknown",
		Stage:          fixedStage(raw.Stage),
		Code:           fixedDiagnosticCode(raw.Code),
		CauseConfirmed: raw.CauseConfirmed,
		RelatedRole:    raw.RelatedRole,
		Usage:          fixedUsage(raw.Usage),
	}
	if taskStatuses[raw.Status] {
		out.Status = raw.Status
	}
	out.SafeMessage = diagnosticMessages[out.Code]
	if failureVersions.MatchString(raw.FailureVersion) {
		out.FailureVersion = raw.FailureVersion
	}
	out.ReportedAt = fixedTimestamp(raw.ReportedAt)
	out.OperationID = fixedOperation(id, raw.OperationID)
	if out.OperationID == "" {
		out.CauseConfirmed = false
	}
	if out.RelatedRole != "tool" && out.RelatedRole != "model" {
		out.RelatedRole = ""
	}
	if toolPattern.MatchString(raw.Tool) {
		out.Tool = raw.Tool
	}
	if out.Tool != "" && out.RelatedRole == "tool" {
		out.SafeParameter = fixedParameter(raw.SafeParameter)
	}
	for _, seq := range raw.RelatedEventIDs {
		if seq > 0 && len(out.RelatedEventIDs) < 8 {
			out.RelatedEventIDs = append(out.RelatedEventIDs, seq)
		}
	}
	if diagnosticActions[raw.RecommendedAction] {
		out.RecommendedAction = raw.RecommendedAction
	}
	if diagnosticRetry[raw.RetryPolicy] {
		out.RetryPolicy = raw.RetryPolicy
	}
	return out
}

func (s *Server) diagnosticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks/{id}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		id := r.PathValue("id")
		if !reportID.MatchString(id) {
			servicehttp.JSONResponse(w, 400, map[string]string{"error": "invalid task"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var raw rawDiagnostics
		err := s.Client.Request(ctx, "GET", s.Client.RuntimeURL+"/v1/apps/futures/runs/"+url.PathEscape(id)+"/diagnostics", nil, &raw)
		if err != nil {
			var status *servicehttp.StatusError
			if errors.As(err, &status) && status.Status == 404 {
				// An older Runtime has no diagnostics route. Keep the task page
				// usable and say only that details cannot be read yet.
				servicehttp.JSONResponse(w, 200, taskDiagnostics{Available: false, RunID: id, Reason: "diagnostics_not_supported"})
				return
			}
			servicehttp.JSONResponse(w, 503, map[string]string{"error": "diagnostics temporarily unavailable"})
			return
		}
		if raw.Protocol != taskDiagnosticsProtocol || raw.RunID != id {
			servicehttp.JSONResponse(w, 502, map[string]string{"error": "diagnostics response rejected"})
			return
		}
		servicehttp.JSONResponse(w, 200, sanitizeDiagnostics(id, raw))
	})
	mux.HandleFunc("GET /api/task-failures", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		q, err := failureListQuery(r)
		if err != nil {
			servicehttp.JSONResponse(w, 400, map[string]string{"error": err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		var raw rawFailurePage
		err = s.Client.Request(ctx, "GET", s.Client.RuntimeURL+"/v1/apps/futures/run-failures?"+q.Encode(), nil, &raw)
		if err != nil {
			servicehttp.JSONResponse(w, 503, map[string]string{"error": "failure records temporarily unavailable"})
			return
		}
		if raw.Protocol != taskFailureProtocol || raw.App != "futures" {
			servicehttp.JSONResponse(w, 502, map[string]string{"error": "failure response rejected"})
			return
		}
		out := taskFailurePage{Protocol: taskFailureProtocol, App: "futures", Items: []taskFailureItem{}}
		for _, item := range raw.Items {
			if len(out.Items) >= 50 || !reportID.MatchString(item.RunID) || !taskFailureStatuses[item.Status] {
				continue
			}
			fixed := taskFailureItem{
				RunID:          item.RunID,
				Status:         item.Status,
				CreatedAt:      fixedTimestamp(item.CreatedAt),
				Commodity:      boundedText(item.Commodity, 100),
				Period:         boundedText(item.Period, 300),
				Stage:          fixedStage(item.Stage),
				Code:           fixedDiagnosticCode(item.Code),
				CauseConfirmed: item.CauseConfirmed,
				OperationID:    fixedOperation(item.RunID, item.OperationID),
				FailureVersion: "",
			}
			if toolPattern.MatchString(item.Tool) {
				fixed.Tool = item.Tool
			}
			if failureVersions.MatchString(item.FailureVersion) {
				fixed.FailureVersion = item.FailureVersion
			}
			if fixed.CreatedAt == "" {
				continue
			}
			out.Items = append(out.Items, fixed)
		}
		if raw.NextCursor != "" && len(raw.NextCursor) <= 2048 {
			out.NextCursor = raw.NextCursor
		}
		out.HasMore = raw.HasMore && out.NextCursor != ""
		out.Coverage = taskFailureCoverage{
			Since: fixedTimestamp(raw.Coverage.Since), Until: fixedTimestamp(raw.Coverage.Until),
			Scanned: raw.Coverage.Scanned, ScanLimit: raw.Coverage.ScanLimit,
			Truncated: raw.Coverage.Truncated, Complete: raw.Coverage.Complete,
		}
		if out.Coverage.Scanned < 0 || out.Coverage.Scanned > 10000 {
			out.Coverage.Scanned = 0
		}
		if out.Coverage.ScanLimit < 0 || out.Coverage.ScanLimit > 10000 {
			out.Coverage.ScanLimit = 0
		}
		servicehttp.JSONResponse(w, 200, out)
	})
}

// failureListQuery validates every filter before it reaches Runtime. Unknown
// or duplicated query keys are rejected instead of being forwarded.
func failureListQuery(r *http.Request) (url.Values, error) {
	allowed := map[string]bool{"since": true, "until": true, "status": true, "category": true, "stage": true, "commodity": true, "limit": true, "after": true}
	raw := r.URL.Query()
	for key, values := range raw {
		if !allowed[key] || len(values) != 1 || values[0] == "" {
			return nil, errors.New("unknown, duplicate or empty failure parameter")
		}
	}
	parseTime := func(name string) error {
		if !raw.Has(name) {
			return nil
		}
		if _, err := time.Parse(time.RFC3339, raw.Get(name)); err != nil {
			return errors.New("invalid failure window")
		}
		return nil
	}
	if err := parseTime("since"); err != nil {
		return nil, err
	}
	if err := parseTime("until"); err != nil {
		return nil, err
	}
	if status := raw.Get("status"); status != "" && !taskFailureStatuses[status] {
		return nil, errors.New("invalid failure status")
	}
	if category := raw.Get("category"); category != "" && !diagnosticCodes[category] {
		return nil, errors.New("invalid failure category")
	}
	if stage := raw.Get("stage"); stage != "" && !diagnosticStages[stage] {
		return nil, errors.New("invalid failure stage")
	}
	if len(raw.Get("commodity")) > 100 {
		return nil, errors.New("commodity filter is too long")
	}
	if limit := raw.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 50 {
			return nil, errors.New("invalid failure page limit")
		}
	}
	if after := raw.Get("after"); len(after) > 2048 {
		return nil, errors.New("invalid failure cursor")
	}
	q := url.Values{}
	for key := range allowed {
		if raw.Has(key) {
			q.Set(key, raw.Get(key))
		}
	}
	return q, nil
}

func safeTaskError(raw string) string {
	if raw == "" {
		return ""
	}
	switch {
	case raw == "model HTTP 503":
		return "model HTTP 503"
	case strings.Contains(raw, "credential"):
		return "credential unavailable"
	case strings.Contains(raw, "model output is incomplete or truncated"):
		return "model output is incomplete or truncated"
	case strings.Contains(raw, "model token budget exhausted"):
		return "model token budget exhausted"
	case strings.Contains(raw, "context budget exceeded"):
		return "context budget exceeded"
	case strings.HasPrefix(raw, "service HTTP"):
		return "upstream request rejected"
	default:
		return "execution failed"
	}
}
