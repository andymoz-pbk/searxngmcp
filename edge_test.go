package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mockSearXNG(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearXNGResponse{
			Query:           q,
			NumberOfResults: 1,
			Results: []SearXNGResult{
				{Title: "Result", URL: "https://x", Content: q, Engine: "test", Score: 1, Category: "general"},
			},
		})
	})
	return httptest.NewServer(mux)
}

func TestEdge_SearXNG_VeryLongQuery(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	longQuery := strings.Repeat("a", 10000)
	client := NewSearXNGClient(ts.URL, 5)
	resp, _, err := client.Search(SearchParams{Query: longQuery})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Query != longQuery {
		t.Errorf("query round-trip failed: got %d chars, want %d", len(resp.Query), len(longQuery))
	}
}

func TestEdge_SearXNG_UnicodeQuery(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	queries := []string{
		"Hello 世界",
		"日本語検索",
		"مرحبا بالعالم",
		"Привет мир",
		"Γειά σου Κόσμε",
		"안녕 세상아",
		"ñóüçê",
		"\u00e9\u00e0\u00fc\u00f1",   // accented latin
		"\u4e2d\u56fd",               // chinese
		"\u0928\u092e\u0938\u094d\u0924\u0947", // devanagari
	}

	for _, q := range queries {
		client := NewSearXNGClient(ts.URL, 5)
		resp, _, err := client.Search(SearchParams{Query: q})
		if err != nil {
			t.Errorf("query %q failed: %v", q, err)
			continue
		}
		if resp.Query != q {
			t.Errorf("query round-trip: got %q, want %q", resp.Query, q)
		}
	}
}

func TestEdge_SearXNG_EmojiQuery(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	emojis := []string{
		"🔥🔥🔥",
		"🌟 star",
		"test🔍search",
		"\U0001F600\U0001F601\U0001F602",  // emoticons
		"a\u0300b\u0301c\u0302",          // combining diacritics
	}

	for _, q := range emojis {
		client := NewSearXNGClient(ts.URL, 5)
		resp, _, err := client.Search(SearchParams{Query: q})
		if err != nil {
			t.Errorf("emoji query %q failed: %v", q, err)
			continue
		}
		if resp.Query != q {
			t.Errorf("query round-trip: got %q, want %q", resp.Query, q)
		}
	}
}

func TestEdge_SearXNG_ControlChars(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	controlQueries := []string{
		"line1\nline2",
		"tab\tseparated",
		"carriage\rreturn",
		"bell\aalert",
		"null\x00byte",
		"escape\x1bsequence",
	}

	for _, q := range controlQueries {
		client := NewSearXNGClient(ts.URL, 5)
		_, _, err := client.Search(SearchParams{Query: q})
		if err != nil {
			t.Errorf("control-char query %q: unexpected error: %v", q, err)
		}
	}
}

func TestEdge_SearXNG_XSSInjection(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	payloads := []string{
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert(1)>",
		"javascript:alert(1)",
		"'><script>alert(1)</script>",
		"<svg/onload=alert(1)>",
		"`'\"<script>",
		"{{constructor.constructor('alert(1)')()}}",
	}

	for _, p := range payloads {
		client := NewSearXNGClient(ts.URL, 5)
		resp, _, err := client.Search(SearchParams{Query: p})
		if err != nil {
			t.Errorf("xss payload %q: unexpected error: %v", p, err)
			continue
		}
		if !strings.Contains(resp.Query, "<script>") && strings.Contains(p, "<script>") {
			t.Errorf("xss payload %q was sanitized by client, should be passed through", p)
		}
	}
}

func TestEdge_SearXNG_SQLInjection(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	payloads := []string{
		"' OR '1'='1",
		"'; DROP TABLE users; --",
		"1' OR '1'='1' /*",
		"' UNION SELECT * FROM users --",
		"admin'--",
	}

	for _, p := range payloads {
		client := NewSearXNGClient(ts.URL, 5)
		resp, _, err := client.Search(SearchParams{Query: p})
		if err != nil {
			t.Errorf("sql injection %q: unexpected error: %v", p, err)
			continue
		}
		if resp.Query != p {
			t.Errorf("query round-trip: got %q, want %q", resp.Query, p)
		}
	}
}

func TestEdge_SearXNG_CommandInjection(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	payloads := []string{
		"; rm -rf /",
		"| cat /etc/passwd",
		"`id`",
		"$(cat /etc/shadow)",
		"& whoami &",
		"; shutdown -h now",
	}

	for _, p := range payloads {
		client := NewSearXNGClient(ts.URL, 5)
		resp, _, err := client.Search(SearchParams{Query: p})
		if err != nil {
			t.Errorf("cmd injection %q: unexpected error: %v", p, err)
			continue
		}
		if resp.Query != p {
			t.Errorf("query round-trip: got %q, want %q", resp.Query, p)
		}
	}
}

func TestEdge_SearXNG_WhitespaceOnlyQuery(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	_, _, err := client.Search(SearchParams{Query: "   "})
	if err != nil {
		t.Logf("whitespace-only query error (expected, depends on SearXNG): %v", err)
	}
}

func TestEdge_SearXNG_NegativeLimits(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	_, _, err := client.Search(SearchParams{Query: "test", MaxResults: -5})
	if err != nil {
		t.Errorf("negative max_results: unexpected error: %v", err)
	}
}

func TestEdge_SearXNG_AllParamsEmpty(t *testing.T) {
	ts := mockSearXNG(t)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	_, _, err := client.Search(SearchParams{
		Query:      "test",
		Categories: "",
		Language:   "",
		PageNo:     0,
		TimeRange:  "",
		Safesearch: 0,
	})
	if err != nil {
		t.Errorf("empty optional params: unexpected error: %v", err)
	}
}

func fastCfg() *Config {
	cfg := DefaultConfig()
	cfg.Fetch.Timeout = 1
	return cfg
}

func TestEdge_Fetch_BinaryContent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		raw := make([]byte, 256)
		for i := range raw {
			raw[i] = byte(i)
		}
		w.Write(raw)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if result.IsError {
		t.Fatalf("binary content fetch: unexpected error: %s", result.Content[0].Text)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &resp); err != nil {
		t.Fatalf("bad JSON in result: %v", err)
	}
	body, _ := resp["body"].(string)
	if len(body) == 0 {
		t.Error("binary content fetch returned empty body")
	}
}

func TestEdge_Fetch_NonUTF8Content(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
		w.Write([]byte{0xc0, 0xe8, 0xec, 0xf2, 0xf9})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if result.IsError {
		t.Fatal("non-UTF8 content: should not error")
	}
}

func TestEdge_Fetch_RedirectChain(t *testing.T) {
	redirectCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount <= 5 {
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusFound)
		} else {
			w.Write([]byte("final"))
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := fastCfg()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if result.IsError {
		t.Fatalf("redirect chain: unexpected error: %s", result.Content[0].Text)
	}
}

func TestEdge_Fetch_RedirectExhaustion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/")
		w.WriteHeader(http.StatusFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := fastCfg()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if !result.IsError {
		t.Log("redirect chain exhausted — got response instead of error")
	}
}

func TestEdge_Fetch_InternalIP(t *testing.T) {
	cfg := fastCfg()
	urls := []string{
		"http://127.0.0.1:22/",
		"http://10.0.0.1:80/",
		"http://172.16.0.1:80/",
		"http://192.168.1.1:80/",
		"http://169.254.169.254:80/",
		"http://0.0.0.0:80/",
		"http://[::1]:80/",
	}

	for _, url := range urls {
		result := handleToolCall("searxng_fetch", map[string]any{"url": url}, cfg, nil)
		if !result.IsError {
			t.Logf("internal URL %q returned success (expected if service is reachable)", url)
		}
	}
}

func TestEdge_Fetch_InvalidURLScheme(t *testing.T) {
	cfg := DefaultConfig()
	urls := []string{
		"ftp://example.com/",
		"file:///etc/passwd",
		"gopher://localhost:70/",
		"data:text/html,<script>alert(1)</script>",
		"javascript:alert(1)",
	}

	for _, url := range urls {
		result := handleToolCall("searxng_fetch", map[string]any{"url": url}, cfg, nil)
		if !result.IsError {
			t.Errorf("insecure scheme %q should error, but succeeded", url)
		}
	}
}

func TestEdge_Fetch_VeryLargeResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("x", 2*1024*1024)))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Fetch.MaxContentLength = 100
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL, "max_length": float64(100)}, cfg, nil)
	if result.IsError {
		t.Fatalf("large response with limit: unexpected error: %s", result.Content[0].Text)
	}
	var resp map[string]any
	json.Unmarshal([]byte(result.Content[0].Text), &resp)
	body, _ := resp["body"].(string)
	if len(body) > 100 {
		t.Errorf("body truncated to %d bytes, expected <= 100", len(body))
	}
}

func TestEdge_Fetch_URLWithSpecialChars(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL + "/path with spaces"}, cfg, nil)
	if !result.IsError {
		t.Logf("URL with spaces succeeded (server-specific)")
	}
}

func TestEdge_Fetch_SSRFViaRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:22/")
		w.WriteHeader(http.StatusFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := fastCfg()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	t.Logf("SSRF redirect result: isError=%v", result.IsError)
}

func TestEdge_Fetch_CRLFInHeader(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := fastCfg()
	crlfURL := ts.URL + "/%0d%0aX-Injected:%20true"
	result := handleToolCall("searxng_fetch", map[string]any{"url": crlfURL}, cfg, nil)
	t.Logf("CRLF injection attempt: isError=%v", result.IsError)
}

func TestEdge_Fetch_MaxLengthZero(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("content"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL, "max_length": float64(0)}, cfg, nil)
	if result.IsError {
		t.Fatalf("zero max_length: unexpected error: %s", result.Content[0].Text)
	}
}

func TestEdge_HtmlToText_MalformedHTML(t *testing.T) {
	tests := []struct {
		name string
		html string
	}{
		{"unclosed tag", "<p>text"},
		{"no angle bracket", "<p"},
		{"nested script", "<script><script>nested</script>"},
		{"script in attribute", "<div data-x=\"<script>\">content</div>"},
		{"unclosed script", "<script>no end tag"},
		{"empty", ""},
		{"only tags", "<><><>"},
		{"deeply nested", "<div><div><div><div><div><div><div><div><div><div>x</div></div></div></div></div></div></div></div></div></div>"},
		{"mixed case tag", "<SCRIPT>alert(1)</SCRIPT>"},
		{"newlines between tags", "<div\n>text</div\n>"},
		{"zero-width chars", "<p>ab\u200bc</p>"},
		{"bidi override", "<p>\u202ERLO\u202Dtest</p>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := htmlToText(tc.html, "smart")
			_ = result // should not panic, result can be anything
		})
	}
}

func TestEdge_HtmlToText_LongInput(t *testing.T) {
	longHTML := "<p>" + strings.Repeat("hello world ", 10000) + "</p>"
	result := htmlToText(longHTML, "smart")
	if len(result) == 0 {
		t.Error("long HTML produced empty result")
	}
}

func TestEdge_HtmlToText_ScriptWithAttributes(t *testing.T) {
	html := `<script type="text/javascript" src="https://evil.example/x.js">alert(1)</script><p>Content</p>`
	result := htmlToText(html, "smart")
	if result != "Content" {
		t.Errorf("got %q, want %q", result, "Content")
	}
}

func TestEdge_HtmlToText_StyleWithAttributes(t *testing.T) {
	html := `<style type="text/css" media="all">body{color:red}</style><p>Visible</p>`
	result := htmlToText(html, "smart")
	if result != "Visible" {
		t.Errorf("got %q, want %q", result, "Visible")
	}
}

func TestEdge_HtmlToText_MixedCaseScript(t *testing.T) {
	html := `<ScRiPt>alert(1)</ScRiPt><p>Content</p>`
	result := htmlToText(html, "smart")
	if result != "Content" {
		t.Errorf("got %q, want %q", result, "Content")
	}
}

func TestEdge_HtmlToText_HtmlEncodedXSS(t *testing.T) {
	html := `&lt;script&gt;alert(1)&lt;/script&gt;`
	result := htmlToText(html, "smart")
	if !strings.Contains(result, "<script>") {
		t.Errorf("HTML entities not decoded: got %q", result)
	}
}

func TestEdge_Server_VeryLongSessionID(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	longID := strings.Repeat("a", 100000)
	resp, err := http.Post(ts.URL+"/messages/"+longID, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("long session ID: expected 404, got %d", resp.StatusCode)
	}
}

func TestEdge_Server_VeryLongJSONRPCBody(t *testing.T) {
	ts := newTestServer(t)
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

	done := make(chan bool, 1)
	go func() {
		body := `{"jsonrpc":"2.0","id":99,"method":"tools/call","params":{"name":"searxng_search","arguments":{"query":"` + strings.Repeat("x", 100000) + `"}}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		postResp, _ := http.DefaultClient.Do(req)
		if postResp != nil {
			postResp.Body.Close()
		}
		done <- true
	}()

	<-done
}

func TestEdge_Server_InvalidMethod(t *testing.T) {
	ts := newTestServer(t)
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
		body := `{"jsonrpc":"2.0","id":1,"method":"i_do_not_exist","params":{}}`
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		postResp, _ := http.DefaultClient.Do(req)
		if postResp != nil {
			postResp.Body.Close()
		}
	}()

	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for unknown method error")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var raw map[string]any
			json.Unmarshal([]byte(line[6:]), &raw)
			if err, ok := raw["error"].(map[string]any); ok {
				if err["code"] == float64(-32601) {
					return
				}
			}
		}
	}
}

func TestEdge_Server_NullCharacterInPath(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	url := ts.URL + "/messages/test\x00null"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{}`))
	if err != nil {
		t.Logf("null byte in URL: %v (expected)", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("null byte request: %v (expected)", err)
		return
	}
	if resp != nil {
		resp.Body.Close()
	}
}

func TestEdge_Server_ManyConcurrentSessions(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	n := 50
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			resp, err := http.Get(ts.URL + "/sse")
			if err != nil {
				errs <- err
				return
			}
			resp.Body.Close()
			errs <- nil
		}()
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent session %d: %v", i, err)
		}
	}
}

func TestEdge_ToolArgs_NilArgs(t *testing.T) {
	cfg := DefaultConfig()
	searxng := NewSearXNGClient("http://127.0.0.1:1", 1)
	result := handleToolCall("searxng_search", nil, cfg, searxng)
	if !result.IsError {
		t.Error("nil args should produce error for missing query")
	}
}

func TestEdge_ToolArgs_WrongTypes(t *testing.T) {
	cfg := DefaultConfig()
	searxng := NewSearXNGClient("http://127.0.0.1:1", 1)

	args := map[string]any{
		"query":       123,
		"max_results": "not a number",
		"pageno":      true,
	}
	result := handleToolCall("searxng_search", args, cfg, searxng)
	if !result.IsError {
		t.Log("wrong types handled without error (query becomes \"\")")
	}
}

func TestEdge_SearXNGResponse_UnicodeResults(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"query": "test",
			"results": []map[string]any{
				{
					"title":   "日本語タイトル",
					"url":     "https://example.com/日本語",
					"content": "アラビア語: مرحبا بالعالم",
					"engine":  "test",
					"score":   0.9,
					"category": "general",
				},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	resp, _, err := client.Search(SearchParams{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "日本語タイトル" {
		t.Errorf("title round-trip: got %q", resp.Results[0].Title)
	}
}

func TestEdge_SearXNGResponse_SurrogatePair(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"query":   "🔥",
			"results": []map[string]any{},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	resp, _, err := client.Search(SearchParams{Query: "🔥"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Query != "🔥" {
		t.Errorf("emoji round-trip: got %q", resp.Query)
	}
}

func TestEdge_Fetch_ResponseHeadersPassthrough(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "should-not-appear")
		w.Header().Set("Server", "test-edge-server")
		w.Write([]byte("ok"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	var resp map[string]any
	json.Unmarshal([]byte(result.Content[0].Text), &resp)
	headers, _ := resp["headers"].(map[string]any)
	if headers == nil {
		t.Fatal("missing headers in fetch response")
	}
	if headers["server"] != "test-edge-server" {
		t.Errorf("expected server header, got %v", headers["server"])
	}
	if _, ok := headers["x-custom-header"]; ok {
		t.Error("custom header should not be in passthrough list")
	}
}

func TestEdge_Server_MalformedJSONRPC(t *testing.T) {
	ts := newTestServer(t)
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

	malformedBodies := []string{
		"",                                    // empty
		"   ",                                 // whitespace only
		"null",                                // JSON null
		"[]",                                  // JSON array instead of object
		`{"jsonrpc":"2.0"}`,                   // missing method
		`{"jsonrpc":"2.0","method":123}`,      // method is number
		`{"jsonrpc":"3.0","method":"ping"}`,   // wrong jsonrpc version
		`{"jsonrpc":"2.0","id":"str","method":"tools/call","params":"not-an-object"}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"%s"}`, strings.Repeat("x", 10000)),
	}

	for i, body := range malformedBodies {
		body := body
		t.Run(fmt.Sprintf("malformed_%d", i), func(t *testing.T) {
			done := make(chan bool, 1)
			go func() {
				req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				postResp, _ := http.DefaultClient.Do(req)
				if postResp != nil {
					postResp.Body.Close()
				}
				done <- true
			}()
			<-done
		})
	}
}

func TestEdge_HtmlToText_ZeroWidthChars(t *testing.T) {
	input := "a\u200bb\u200cc\u200dd\u200ee"
	result := htmlToText(input, "smart")
	if !strings.Contains(result, "a") {
		t.Error("zero-width chars broke htmlToText")
	}
}

func TestEdge_HtmlToText_NullByteInHTML(t *testing.T) {
	input := "<p>Hello\x00World</p>"
	result := htmlToText(input, "smart")
	if !strings.Contains(result, "Hello") {
		t.Error("null byte broke htmlToText")
	}
}
