package main

import (
	"context"
	"github.com/euler-gaozl/agent-os-futures/futures"
	"github.com/euler-gaozl/agent-os-sdk/servicehost"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

func main() {
	servicehost.Main(servicehost.Options{Role: "futures", Port: "8098", Database: false, Concurrency: 16, UserFacing: true, PublicPath: futures.PublicPath, AdminTokenEnv: "BUSINESS_TOKEN"}, func(ctx context.Context, p *pgxpool.Pool, c servicehttp.Config) (http.Handler, func(context.Context) error, error) {
		if err := futures.ValidateUserToken(c); err != nil {
			return nil, nil, err
		}
		if err := applyLocalDefaults(); err != nil {
			return nil, nil, err
		}
		return (&futures.Server{HTTP: c, Client: servicehost.Client(), KnowledgeURL: servicehost.Env("KNOWLEDGE_URL", "http://127.0.0.1:8095"), WebDir: servicehost.Env("FUTURES_WEB_DIR", "web/dist")}).Handler(), nil, nil
	})
}
