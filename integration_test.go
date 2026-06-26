//go:build integration

package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func liveSearXNGURL(t *testing.T) string {
	t.Helper()
	url := "http://localhost:8080"
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != 200 {
		t.Skipf("SearXNG not reachable at %s: %v", url, err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	return url
}

func TestIntegration_Search_RealSearXNG(t *testing.T) {
	sxURL := liveSearXNGURL(t)

	cfg := DefaultConfig()
	cfg.SearXNG.BaseURL = sxURL
	mcp := NewMCPServer(cfg)
	ts := httptest.NewServer(mcp.recoveryMiddleware(mcp))
	defer ts.Close()

	sseResp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	scanner := bufio.NewScanner(sseResp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if endpoint == "" {
		t.Fatal("no endpoint event")
	}

	done := make(chan string, 1)
	go func() {
		body := `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"searxng_search","arguments":{"query":"hello world","max_results":3}}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()

	timeout := time.After(10 * time.Second)
	var resultText string
outer:
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for search results")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line[6:]), &raw) != nil {
			continue
		}
		if raw["id"] != float64(10) {
			continue
		}
		resultMap, ok := raw["result"].(map[string]any)
		if !ok {
			continue
		}
		content, ok := resultMap["content"].([]any)
		if !ok || len(content) == 0 {
			continue
		}
		item, ok := content[0].(map[string]any)
		if !ok {
			continue
		}
		resultText, _ = item["text"].(string)
		break outer
	}
	done <- resultText

	if resultText == "" {
		t.Fatal("no search result text received")
	}

	var searchResp map[string]any
	if err := json.Unmarshal([]byte(resultText), &searchResp); err != nil {
		t.Fatalf("bad JSON in result: %v", err)
	}

	t.Logf("query: %v", searchResp["query"])
	t.Logf("results count: %v", len(searchResp["results"].([]any)))
	t.Logf("unresponsive_engines count: %v", len(searchResp["unresponsive_engines"].([]any)))

	if _, ok := searchResp["results"]; !ok {
		t.Error("missing results field")
	}
	if _, ok := searchResp["query"]; !ok {
		t.Error("missing query field")
	}
}

func TestIntegration_Fetch_RealURL(t *testing.T) {
	cfg := DefaultConfig()
	mcp := NewMCPServer(cfg)
	ts := httptest.NewServer(mcp.recoveryMiddleware(mcp))
	defer ts.Close()

	sseResp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	scanner := bufio.NewScanner(sseResp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	go func() {
		body := `{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"searxng_fetch","arguments":{"url":"https://example.com","max_length":5000}}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
	}()

	timeout := time.After(10 * time.Second)
	var resultText string
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for fetch result")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line[6:]), &raw) != nil {
			continue
		}
		if raw["id"] != float64(20) {
			continue
		}
		resultMap, ok := raw["result"].(map[string]any)
		if !ok {
			continue
		}
		content, ok := resultMap["content"].([]any)
		if !ok || len(content) == 0 {
			continue
		}
		item, ok := content[0].(map[string]any)
		if !ok {
			continue
		}
		resultText, _ = item["text"].(string)
		break
	}

	if resultText == "" {
		t.Fatal("no fetch result text received")
	}

	var fetchResp map[string]any
	if err := json.Unmarshal([]byte(resultText), &fetchResp); err != nil {
		t.Fatalf("bad JSON in result: %v", err)
	}

	t.Logf("url: %v", fetchResp["url"])
	t.Logf("status_code: %v", fetchResp["status_code"])
	t.Logf("content_length: %v", fetchResp["content_length"])

	if fetchResp["status_code"] != float64(200) {
		t.Errorf("status_code = %v, want 200", fetchResp["status_code"])
	}
	body, _ := fetchResp["body"].(string)
	if !strings.Contains(body, "Example Domain") {
		t.Error("body should mention Example Domain")
	}
}

func TestIntegration_Fetch_FakeURL(t *testing.T) {
	cfg := DefaultConfig()
	mcp := NewMCPServer(cfg)
	ts := httptest.NewServer(mcp.recoveryMiddleware(mcp))
	defer ts.Close()

	sseResp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	scanner := bufio.NewScanner(sseResp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	go func() {
		body := `{"jsonrpc":"2.0","id":30,"method":"tools/call","params":{"name":"searxng_fetch","arguments":{"url":"http://nonexistent.example.test:9999/"}}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
	}()

	timeout := time.After(5 * time.Second)
	var resultText string
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for fetch error")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line[6:]), &raw) != nil {
			continue
		}
		if raw["id"] != float64(30) {
			continue
		}
		resultMap, ok := raw["result"].(map[string]any)
		if !ok {
			continue
		}
		isError, _ := resultMap["isError"].(bool)
		if !isError {
			t.Error("expected isError=true for fake URL")
		}
		content, ok := resultMap["content"].([]any)
		if !ok || len(content) == 0 {
			continue
		}
		item, ok := content[0].(map[string]any)
		if !ok {
			continue
		}
		resultText, _ = item["text"].(string)
		break
	}

	if resultText == "" {
		t.Fatal("no fetch error text received")
	}
	t.Logf("Fetch error text: %s", resultText)
}
