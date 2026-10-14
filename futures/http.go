package futures

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/euler-gaozl/agent-os-futures/internal/webproxy"
	"github.com/euler-gaozl/agent-os-sdk/contracts"
	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Server struct {
	HTTP                 servicehttp.Config
	Client               serviceclient.Client
	KnowledgeURL, WebDir string
}

func (s *Server) Handler() http.Handler { return servicehttp.New(s.HTTP, nil, s.Routes) }
func (s *Server) Routes(mux *http.ServeMux) {
	s.reportRoute(mux)
	mux.HandleFunc("GET /api/agents", func(w http.ResponseWriter, r *http.Request) {
		webproxy.Proxy(w, r, s.Client.RegistryURL+"/v1/published/futures", s.Client.Token)
	})
	mux.HandleFunc("POST /api/tasks", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Agent     contracts.Ref `json:"agent"`
			Commodity string        `json:"commodity"`
			Period    string        `json:"period"`
			Question  string        `json:"question"`
			AsOf      string        `json:"as_of"`
		}
		if e := servicehttp.Decode(w, r, &in); e != nil {
			return e
		}
		if strings.TrimSpace(in.Commodity) == "" || len(in.Commodity) > 100 || len(in.Period) > 300 || len(in.Question) > 20000 {
			return errors.New("commodity and bounded research question required")
		}
		if r.Header.Get("Idempotency-Key") == "" {
			return errors.New("idempotency key required")
		}
		task := fmt.Sprintf("品种：%s\n资料时间范围：%s\n用户问题：%s", in.Commodity, in.Period, in.Question)
		metadata := map[string]string{"commodity": in.Commodity, "period": in.Period}
		if in.AsOf != "" {
			if _, ok := validDate(in.AsOf); !ok {
				return &servicehttp.StatusError{Status: 400, Message: "valid classification date required"}
			}
			metadata["as_of"] = in.AsOf
			metadata["report_format"] = reportSchema
			task += reportInstructions(in.AsOf)
		}
		body := contracts.JSON(map[string]any{"agent": in.Agent, "task": task, "metadata": metadata})
		copy := r.Clone(r.Context())
		copy.Body = io.NopCloser(bytes.NewReader(body))
		copy.ContentLength = int64(len(body))
		webproxy.Proxy(w, copy, s.Client.RuntimeURL+"/v1/apps/futures/runs", s.Client.Token)
		return nil
	}))
	mux.HandleFunc("GET /api/tasks", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		var rows []map[string]any
		if e := s.Client.Request(r.Context(), "GET", s.Client.RuntimeURL+"/v1/apps/futures/runs", nil, &rows); e != nil {
			return e
		}
		for _, row := range rows {
			project(row)
		}
		servicehttp.JSONResponse(w, 200, rows)
		return nil
	}))
	mux.HandleFunc("GET /api/tasks/{id}", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		var row map[string]any
		if e := s.Client.Request(r.Context(), "GET", s.Client.RuntimeURL+"/v1/apps/futures/runs/"+url.PathEscape(r.PathValue("id")), nil, &row); e != nil {
			return e
		}
		project(row)
		servicehttp.JSONResponse(w, 200, row)
		return nil
	}))
	mux.HandleFunc("POST /api/tasks/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		webproxy.Proxy(w, r, s.Client.RuntimeURL+"/v1/apps/futures/runs/"+url.PathEscape(r.PathValue("id"))+"/cancel", s.Client.Token)
	})
	for _, suffix := range []string{"sources", "sources/record"} {
		upstream := strings.Replace(suffix, "sources", "evidence", 1)
		mux.HandleFunc("GET /api/tasks/{id}/"+suffix, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			webproxy.Proxy(w, r, s.Client.RuntimeURL+"/v1/apps/futures/runs/"+url.PathEscape(r.PathValue("id"))+"/"+upstream, s.Client.Token)
		})
	}
	mux.HandleFunc("GET /api/readiness", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		var credentials, knowledge map[string]any
		e1 := s.Client.Request(r.Context(), "GET", s.Client.RegistryURL+"/v1/status", nil, &credentials)
		e2 := s.Client.Request(r.Context(), "GET", s.KnowledgeURL+"/v1/status", nil, &knowledge)
		servicehttp.JSONResponse(w, 200, map[string]any{"model_configured": e1 == nil && credentials["deepseek_configured"] == true, "indexed_documents": knowledge["indexed_documents"], "knowledge_available": e2 == nil})
		return nil
	}))
	if s.WebDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(s.WebDir)))
	}
}
func project(row map[string]any) {
	if m, ok := row["metadata"].(map[string]any); ok {
		row["commodity"] = m["commodity"]
		row["as_of"] = m["as_of"]
		row["report_format"] = m["report_format"]
	}
	delete(row, "metadata")
}
