package webproxy

import (
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"io"
	"net/http"
)

func Proxy(w http.ResponseWriter, r *http.Request, target, token string) {
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, e := http.NewRequestWithContext(r.Context(), r.Method, target, http.MaxBytesReader(w, r.Body, 2<<20))
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
	for _, h := range []string{"Content-Type", "Content-Disposition", "X-Artifact-Media-Type", "Retry-After"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 4<<20))
}
