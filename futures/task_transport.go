package futures

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
)

// Match Runtime's task transport. These are byte/resource capacities, not
// generation token limits. Registry/Runtime own model capability validation.
const taskRequestBytes int64 = 32 << 20
const taskTextBytes = 16 << 20
const taskResponseBytes int64 = 64 << 20

func decodeTaskRequest(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, taskRequestBytes)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return taskDecodeError(err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return taskDecodeError(err)
	}
	return nil
}

func taskDecodeError(err error) error {
	var capacity *http.MaxBytesError
	if errors.As(err, &capacity) {
		return &servicehttp.StatusError{Status: 413, Message: "research request exceeds transport capacity"}
	}
	return &servicehttp.StatusError{Status: 400, Message: "expected one valid research request JSON object"}
}

// Fetch a single app-scoped task with capacity for the complete result. Other
// lists and evidence pages keep their own smaller, independently bounded APIs.
func (s *Server) readTask(ctx context.Context, id string, out any) error {
	endpoint := s.Client.RuntimeURL + "/v1/apps/futures/runs/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.Client.Token)
	resp, err := servicehttp.Client.Do(req)
	if err != nil {
		return &servicehttp.StatusError{Status: 503, Message: "task service unavailable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &servicehttp.StatusError{Status: resp.StatusCode, Message: "task service rejected request"}
	}
	if resp.StatusCode != http.StatusOK {
		return &servicehttp.StatusError{Status: 502, Message: "unexpected task service response"}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, taskResponseBytes+1))
	if err != nil || int64(len(raw)) > taskResponseBytes {
		return &servicehttp.StatusError{Status: 502, Message: "task response is incomplete or exceeds transport capacity"}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &servicehttp.StatusError{Status: 502, Message: "invalid task service JSON response"}
	}
	return nil
}
