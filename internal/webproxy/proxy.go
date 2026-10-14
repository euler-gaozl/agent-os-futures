package webproxy

import (
	"bytes"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"io"
	"net/http"
)

func Proxy(w http.ResponseWriter, r *http.Request, target, token string) {
	ProxyLimits(w, r, target, token, 2<<20, 4<<20)
}

// Routes select transport capacity; callers cannot increase it. Read the full
// bounded reply before sending a success status, so truncation is never hidden.
func ProxyLimits(w http.ResponseWriter, r *http.Request, target, token string, requestLimit, responseLimit int64) {
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, requestLimit))
	if e != nil {
		servicehttp.JSONResponse(w, 413, map[string]string{"error": "request exceeds transport capacity or could not be read"})
		return
	}
	req, e := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if e != nil {
		servicehttp.JSONResponse(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", r.Header.Get("Idempotency-Key"))
	resp, e := servicehttp.Client.Do(req)
	if e != nil {
		servicehttp.JSONResponse(w, 503, map[string]string{"error": "upstream service unavailable"})
		return
	}
	defer resp.Body.Close()
	result, e := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if e != nil || int64(len(result)) > responseLimit {
		servicehttp.JSONResponse(w, 502, map[string]string{"error": "upstream response is incomplete or exceeds transport capacity"})
		return
	}
	for _, h := range []string{"Content-Type", "Content-Disposition", "X-Artifact-Media-Type", "Retry-After"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(result)
}
