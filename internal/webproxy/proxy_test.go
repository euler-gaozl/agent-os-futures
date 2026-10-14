package webproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyNeverCommitsTruncatedSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, request, response string
		partial                 bool
		status, want            int
	}{
		{"complete", "12345678", "1234567890123456", false, 200, 200},
		{"request capacity", "123456789", "ok", false, 200, 413},
		{"response capacity", "ok", "12345678901234567", false, 200, 502},
		{"partial response", "ok", "short", true, 200, 502},
		{"upstream rejection", "ok", "rejected", false, 409, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer service" || r.Header.Get("Idempotency-Key") != "stable" || r.URL.RawQuery != "offset=2" {
					t.Error("request identity changed")
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != tc.request {
					t.Error("request truncated")
				}
				if tc.partial {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.response)
			}))
			defer peer.Close()
			r := httptest.NewRequest("POST", "/proxy?offset=2", strings.NewReader(tc.request))
			r.Header.Set("Authorization", "Bearer browser")
			r.Header.Set("Idempotency-Key", "stable")
			w := httptest.NewRecorder()
			ProxyLimits(w, r, peer.URL, "service", 8, 16)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.want == 413 && calls != 0 {
				t.Fatal("oversized request forwarded")
			}
			if tc.want == 200 && w.Body.String() != tc.response {
				t.Fatal("complete response changed")
			}
		})
	}
}
