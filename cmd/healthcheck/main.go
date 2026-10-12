package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

func main() {
	file := os.Getenv("AGENTOS_CONFIG")
	if file == "" {
		file = "/run/service.json"
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		os.Exit(1)
	}
	var c map[string]string
	if json.Unmarshal(raw, &c) != nil {
		os.Exit(1)
	}
	token := c["BUSINESS_TOKEN"]
	if token == "" {
		token = c["ADMIN_TOKEN"]
	}
	if key := os.Getenv("HEALTH_TOKEN_KEY"); key != "" {
		token = c[key]
	}
	if len(token) < 32 || token == c["SERVICE_TOKEN"] {
		os.Exit(1)
	}
	target := os.Getenv("HEALTH_URL")
	if target == "" {
		os.Exit(1)
	}
	req, e := http.NewRequest("GET", target, nil)
	if e != nil {
		os.Exit(1)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		os.Exit(1)
	}
}
