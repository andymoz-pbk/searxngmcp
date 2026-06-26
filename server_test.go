package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := DefaultConfig()
	cfg.SearXNG.BaseURL = "http://127.0.0.1:1"
	mcp := NewMCPServer(cfg)
	return httptest.NewServer(mcp.recoveryMiddleware(mcp))
}

func TestServer_CORS_Headers(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if v := resp.Header.Get("Access-Control-Allow-Origin"); v != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", v)
	}
	if v := resp.Header.Get("Access-Control-Allow-Methods"); v == "" {
		t.Error("missing Access-Control-Allow-Methods")
	}
}

func TestServer_CORS_Preflight(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/sse", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin on OPTIONS")
	}
}

func TestServer_NotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_MethodNotAllowed(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/sse", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /sse status = %d, want 405", resp.StatusCode)
	}

	resp2, err := http.Get(ts.URL + "/messages/foo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /messages/foo status = %d, want 405", resp2.StatusCode)
	}
}

func TestServer_SessionNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/messages/nonexistent", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_SSE_EndpointEvent(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if endpoint == "" {
		t.Fatal("no endpoint event received")
	}
	if !strings.HasPrefix(endpoint, "http") {
		t.Errorf("endpoint = %q, want absolute URL", endpoint)
	}
}

func TestServer_ToolsList(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if endpoint == "" {
		t.Fatal("no endpoint event received")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		postResp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("POST failed: %v", err)
			return
		}
		postResp.Body.Close()
	}()

	var found bool
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for tools/list response")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var resp JSONRPCResponse
			if err := json.Unmarshal([]byte(line[6:]), &resp); err != nil {
				continue
			}
			if resp.ID == float64(1) && resp.Result != nil {
				found = true
				result, ok := resp.Result.(map[string]any)
				if !ok {
					t.Fatal("result is not a map")
				}
				tools, ok := result["tools"].([]any)
				if !ok {
					t.Fatal("tools is not an array")
				}
				if len(tools) != 11 {
					t.Errorf("got %d tools, want 11", len(tools))
				}
				break
			}
		}
	}
	wg.Wait()
	if !found {
		t.Fatal("did not receive tools/list response")
	}
}

func TestServer_ToolsCall_Search_Error(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if endpoint == "" {
		t.Fatal("no endpoint event received")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		body := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"searxng_search","arguments":{"query":"hello"}}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		postResp, _ := http.DefaultClient.Do(req)
		if postResp != nil {
			postResp.Body.Close()
		}
	}()

	var found bool
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for tools/call response")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var raw map[string]any
			if err := json.Unmarshal([]byte(line[6:]), &raw); err != nil {
				continue
			}
			if raw["id"] == float64(2) && raw["result"] != nil {
				found = true
				break
			}
		}
	}
	wg.Wait()
	if !found {
		t.Fatal("did not receive tools/call response")
	}
}

func TestServer_SSE_ContentType(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
}

func TestServer_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`not json`))
		req.Header.Set("Content-Type", "application/json")
		postResp, _ := http.DefaultClient.Do(req)
		if postResp != nil {
			postResp.Body.Close()
		}
	}()

	var found bool
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for parse error response")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.Contains(line, "-32700") {
			found = true
			break
		}
	}
	wg.Wait()
	if !found {
		t.Fatal("did not receive parse error")
	}
}

func TestServer_Initialize(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var endpoint string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: http") {
			endpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		postResp, _ := http.DefaultClient.Do(req)
		if postResp != nil {
			postResp.Body.Close()
		}
	}()

	var found bool
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for initialize response")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var raw map[string]any
			if err := json.Unmarshal([]byte(line[6:]), &raw); err != nil {
				continue
			}
			if raw["id"] == float64(1) {
				result, ok := raw["result"].(map[string]any)
				if !ok {
					continue
				}
			if result["protocolVersion"] != "2025-03-26" {
					t.Errorf("protocolVersion = %v, want 2025-03-26", result["protocolVersion"])
				}
				found = true
				break
			}
		}
	}
	wg.Wait()
	if !found {
		t.Fatal("did not receive initialize response")
	}
}

func TestStreamableHTTP_Post_Initialize(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v", raw["jsonrpc"])
	}
	if raw["id"] != float64(1) {
		t.Errorf("id = %v", raw["id"])
	}
	result, ok := raw["result"].(map[string]any)
	if !ok {
		t.Fatal("result missing or not a map")
	}
	if result["protocolVersion"] != "2025-03-26" {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}
	cap, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("capabilities missing")
	}
	if _, ok := cap["tools"]; !ok {
		t.Error("capabilities.tools missing")
	}
}

func TestStreamableHTTP_Post_ToolsList(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	result, ok := raw["result"].(map[string]any)
	if !ok {
		t.Fatal("result missing")
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("tools missing or not array")
	}
	if len(tools) != 11 {
		t.Errorf("got %d tools, want 11", len(tools))
	}
}

func TestStreamableHTTP_Post_Ping(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":3,"method":"ping"}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["id"] != float64(3) {
		t.Errorf("id = %v", raw["id"])
	}
	if _, ok := raw["result"]; !ok {
		t.Error("result missing")
	}
	if raw["error"] != nil {
		t.Errorf("unexpected error: %v", raw["error"])
	}
}

func TestStreamableHTTP_Post_Notification(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202 (Accepted) for notification", resp.StatusCode)
	}
}

func TestStreamableHTTP_Post_UnknownMethod(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":4,"method":"bogus","params":{}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["error"] == nil {
		t.Fatal("expected error for unknown method")
	}
	errObj := raw["error"].(map[string]any)
	if errObj["code"] != float64(-32601) {
		t.Errorf("error code = %v, want -32601", errObj["code"])
	}
}

func TestStreamableHTTP_Post_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["error"] == nil {
		t.Fatal("expected error for bad JSON")
	}
	errObj := raw["error"].(map[string]any)
	if errObj["code"] != float64(-32700) {
		t.Errorf("error code = %v, want -32700", errObj["code"])
	}
}

func TestStreamableHTTP_Post_ToolCall_Search(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_datetime","arguments":{"timezone":"UTC"}}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	result, ok := raw["result"].(map[string]any)
	if !ok {
		t.Fatalf("result missing or not a map: %v", raw)
	}
	content, ok := result["content"].([]any)
	if !ok {
		t.Fatalf("content missing or not array: %v", result)
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] not a map: %v", content)
	}
	text, ok := item["text"].(string)
	if !ok {
		t.Fatalf("text missing: %v", item)
	}
	var dt map[string]any
	if err := json.Unmarshal([]byte(text), &dt); err != nil {
		t.Fatalf("datetime JSON: %v", err)
	}
	if dt["timezone"] != "UTC" {
		t.Errorf("timezone = %q", dt["timezone"])
	}
}

func TestStreamableHTTP_Get_ReturnsSSE(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestStreamableHTTP_Delete_Returns204(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestStreamableHTTP_Post_NotificationWithoutID(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","method":"ping"}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202 (Accepted) for notification", resp.StatusCode)
	}
}

func TestStreamableHTTP_Post_MissingContentType(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["result"]; !ok {
		t.Error("expected result for ping")
	}
}

func TestStreamableHTTP_Post_ToolCall_Error(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"searxng_search","arguments":{}}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	result, ok := raw["result"].(map[string]any)
	if !ok {
		t.Fatalf("result missing: %v", raw)
	}
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError=true for missing query")
	}
}

func TestStreamableHTTP_RootPath(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
