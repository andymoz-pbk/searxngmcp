package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHandleSearch_MissingQuery(t *testing.T) {
	cfg := DefaultConfig()
	searxng := NewSearXNGClient("http://127.0.0.1:1", 1)
	result := handleToolCall("searxng_search", map[string]any{}, cfg, searxng)
	if !result.IsError {
		t.Fatal("expected error for missing query")
	}
}

func TestHandleSearch_UnreachableSearXNG(t *testing.T) {
	cfg := DefaultConfig()
	searxng := NewSearXNGClient("http://127.0.0.1:1", 1)
	result := handleToolCall("searxng_search", map[string]any{"query": "hello"}, cfg, searxng)
	if !result.IsError {
		t.Fatal("expected error for unreachable searxng")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected error content")
	}
}

func TestHandleSearch_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"query":"hello","results":[{"title":"T1","url":"https://x","content":"snippet","engine":"google","score":0.9,"category":"general"}]}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	searxng := NewSearXNGClient(ts.URL, 5)
	result := handleToolCall("searxng_search", map[string]any{"query": "hello"}, cfg, searxng)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if len(result.Content) == 0 {
		t.Fatal("no content returned")
	}
}

func TestHandleFetch_MissingURL(t *testing.T) {
	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{}, cfg, nil)
	if !result.IsError {
		t.Fatal("expected error for missing url")
	}
}

func TestHandleFetch_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent header")
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Server", "test-server")
		w.Write([]byte(`<html><head><title>Test</title></head><body><p>Hello world</p></body></html>`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "Hello world") {
		t.Errorf("expected body text, got: %s", result.Content[0].Text)
	}
}

func TestHandleFetch_NonHTMLResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
}

func TestHandleFetch_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": ts.URL}, cfg, nil)
	if result.IsError {
		t.Fatalf("unexpected error for 404: %s", result.Content[0].Text)
	}
}

func TestHandleFetch_BadURL(t *testing.T) {
	cfg := DefaultConfig()
	result := handleToolCall("searxng_fetch", map[string]any{"url": "://bad"}, cfg, nil)
	if !result.IsError {
		t.Fatal("expected error for bad url")
	}
}

func TestHandlesearch_WithParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("categories") != "news" {
			t.Errorf("categories = %q", q.Get("categories"))
		}
		if q.Get("language") != "fr" {
			t.Errorf("language = %q", q.Get("language"))
		}
		if q.Get("pageno") != "3" {
			t.Errorf("pageno = %q", q.Get("pageno"))
		}
		if q.Get("time_range") != "month" {
			t.Errorf("time_range = %q", q.Get("time_range"))
		}
		w.Write([]byte(`{"results":[]}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := DefaultConfig()
	searxng := NewSearXNGClient(ts.URL, 5)
	result := handleToolCall("searxng_search", map[string]any{
		"query":       "test",
		"categories":  "news",
		"language":    "fr",
		"pageno":      float64(3),
		"time_range":  "month",
		"safesearch":  float64(1),
		"max_results": float64(5),
	}, cfg, searxng)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
}

func TestHtmlToText(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "simple text",
			html: "<p>Hello world</p>",
			want: "Hello world",
		},
		{
			name: "strip script",
			html: "<script>alert('x')</script><p>Content</p>",
			want: "Content",
		},
		{
			name: "strip style",
			html: "<style>body{color:red}</style><p>Text</p>",
			want: "Text",
		},
		{
			name: "block tags to newlines",
			html: "<div>A</div><p>B</p>",
			want: "A B",
		},
		{
			name: "nested tags",
			html: "<div><p>Hello <b>world</b></p></div>",
			want: "Hello world",
		},
		{
			name: "html entities",
			html: "<p>AT&amp;T &amp; Co</p>",
			want: "AT&T & Co",
		},
		{
			name: "multiple whitespace",
			html: "<p>  Hello    world  </p>",
			want: "Hello world",
		},
		{
			name: "newlines from br",
			html: "<p>Line1<br>Line2<br/>Line3</p>",
			want: "Line1 Line2 Line3",
		},
		{
			name: "strip svg block",
			html: "<svg><path d=\"M10 20 L30 40\"/><circle cx=\"50\" cy=\"50\" r=\"10\"/></svg><p>Text</p>",
			want: "Text",
		},
		{
			name: "strip noscript",
			html: "<noscript>JS required</noscript><p>Content</p>",
			want: "Content",
		},
		{
			name: "strip iframe",
			html: "<iframe src=\"https://ads.example\"></iframe><p>Article text</p>",
			want: "Article text",
		},
		{
			name: "strip template",
			html: "<template><tr><td>template</td></tr></template><p>Real content</p>",
			want: "Real content",
		},
		{
			name: "content area extraction",
			html: "<html><head></head><body><nav>links</nav><main><article><p>This is a longer article body with enough text to pass the minimum content threshold for extraction. It contains multiple sentences that describe the content of the page in detail.</p></article></main><footer>copyright 2026</footer></body></html>",
			want: "links This is a longer article body with enough text to pass the minimum content threshold for extraction. It contains multiple sentences that describe the content of the page in detail. copyright 2026",
		},
		{
			name: "filter low-letter-ratio lines",
			html: "<p>Hello world</p><div>M10 20 L30 40 a5 5 0 0 1 0 10</div><div>#333 #fff #000</div><p>goodbye</p>",
			want: "Hello world M10 20 L30 40 a5 5 0 0 1 0 10 #333 #fff #000 goodbye",
		},
		{
			name: "fallback to meta description for SPAs",
			html: "<html><head><meta property=\"og:description\" content=\"This is the page description from meta\"></head><body><div id=\"app\"></div><script>loadApp()</script></body></html>",
			want: "This is the page description from meta",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := htmlToText(tc.html, "smart")
			if got != tc.want {
				t.Errorf("htmlToText(%q) = %q, want %q", tc.html, got, tc.want)
			}
		})
	}
}

func TestToolCaller_UnknownTool(t *testing.T) {
	cfg := DefaultConfig()
	result := handleToolCall("nonexistent", nil, cfg, nil)
	if !result.IsError {
		t.Fatal("expected error for unknown tool")
	}
}

func parseDateTimeResult(t *testing.T, result ToolCallResult) map[string]any {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &raw); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	return raw
}

func TestDateTime_DefaultUTC(t *testing.T) {
	before := time.Now().Unix()
	result := handleDateTime(nil)
	after := time.Now().Unix()

	r := parseDateTimeResult(t, result)

	if r["timezone"] != "UTC" {
		t.Errorf("timezone = %q, want UTC", r["timezone"])
	}

	ts, ok := r["unix_timestamp"].(float64)
	if !ok {
		t.Fatalf("unix_timestamp type = %T", r["unix_timestamp"])
	}
	if int64(ts) < before || int64(ts) > after {
		t.Errorf("unix_timestamp %d outside range [%d, %d]", int64(ts), before, after)
	}

	iso, _ := r["iso_8601"].(string)
	if !strings.HasSuffix(iso, "Z") && !strings.Contains(iso, "+00:00") {
		t.Errorf("iso_8601 = %q, expected UTC suffix", iso)
	}

	date, _ := r["date"].(string)
	timeStr, _ := r["time"].(string)
	if len(date) != 10 || date[4] != '-' || date[7] != '-' {
		t.Errorf("date format = %q, want YYYY-MM-DD", date)
	}
	if len(timeStr) != 8 || timeStr[2] != ':' || timeStr[5] != ':' {
		t.Errorf("time format = %q, want HH:MM:SS", timeStr)
	}

	utcIso, _ := r["utc_iso_8601"].(string)
	if iso != utcIso {
		t.Errorf("iso_8601 = %q, utc_iso_8601 = %q, should match in UTC", iso, utcIso)
	}
}

func TestDateTime_CustomTimezone(t *testing.T) {
	result := handleDateTime(map[string]any{"timezone": "America/New_York"})
	r := parseDateTimeResult(t, result)

	if r["timezone"] != "America/New_York" {
		t.Errorf("timezone = %q", r["timezone"])
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	wantNow := time.Now().In(loc)

	ts, _ := r["unix_timestamp"].(float64)
	gotUnix := time.Unix(int64(ts), 0)
	if gotUnix.Year() != wantNow.Year() || gotUnix.Month() != wantNow.Month() || gotUnix.Day() != wantNow.Day() {
		t.Errorf("date mismatch: got %s, system says %s", gotUnix.Format("2006-01-02"), wantNow.Format("2006-01-02"))
	}

	year, _ := r["year"].(float64)
	if int(year) != wantNow.Year() {
		t.Errorf("year = %.0f, want %d", year, wantNow.Year())
	}

	dow, _ := r["day_of_week"].(string)
	if dow != wantNow.Weekday().String() {
		t.Errorf("day_of_week = %q, want %q", dow, wantNow.Weekday().String())
	}

	dst, _ := r["is_dst"].(bool)
	if dst != wantNow.IsDST() {
		t.Errorf("is_dst = %v, want %v", dst, wantNow.IsDST())
	}
}

func TestDateTime_AllFieldsInternalConsistency(t *testing.T) {
	result := handleDateTime(map[string]any{"timezone": "Asia/Tokyo"})
	r := parseDateTimeResult(t, result)

	year := int(r["year"].(float64))
	monthStr, _ := r["month"].(string)
	dayStr, _ := r["day"].(string)
	date, _ := r["date"].(string)

	wantDate := fmt.Sprintf("%04d-%s-%s", year, monthStr, dayStr)
	if date != wantDate {
		t.Errorf("date = %q, but year+month+day = %q", date, wantDate)
	}

	timeStr, _ := r["time"].(string)
	hour24, _ := r["hour_24"].(string)
	minute, _ := r["minute"].(string)
	second, _ := r["second"].(string)

	wantTime := fmt.Sprintf("%s:%s:%s", hour24, minute, second)
	if timeStr != wantTime {
		t.Errorf("time = %q, but hour_24+minute+second = %q", timeStr, wantTime)
	}
}

func TestDateTime_InvalidTimezone(t *testing.T) {
	result := handleDateTime(map[string]any{"timezone": "Mars/Olympus"})
	if !result.IsError {
		t.Fatal("expected error for invalid timezone")
	}
}

func TestDateTime_EmptyTimezone(t *testing.T) {
	result := handleDateTime(map[string]any{"timezone": ""})
	r := parseDateTimeResult(t, result)
	if r["timezone"] != "UTC" {
		t.Errorf("timezone = %q, want UTC for empty input", r["timezone"])
	}
}

func TestDateTime_NilArgs(t *testing.T) {
	result := handleDateTime(nil)
	r := parseDateTimeResult(t, result)
	if r["timezone"] != "UTC" {
		t.Errorf("timezone = %q, want UTC for nil args", r["timezone"])
	}
}

func TestDateTime_AllNumericFieldsPresent(t *testing.T) {
	result := handleDateTime(nil)
	r := parseDateTimeResult(t, result)

	requiredNumeric := []string{"unix_timestamp", "unix_timestamp_ms", "year", "day_of_year", "week_number"}
	for _, key := range requiredNumeric {
		v, ok := r[key].(float64)
		if !ok {
			t.Errorf("%s missing or not numeric (type=%T)", key, r[key])
			continue
		}
		if v <= 0 {
			t.Errorf("%s = %v, expected positive", key, v)
		}
	}

	ms, _ := r["unix_timestamp_ms"].(float64)
	sec, _ := r["unix_timestamp"].(float64)
	if ms < sec*1000 || ms > (sec+1)*1000 {
		t.Errorf("unix_timestamp_ms=%.0f not consistent with unix_timestamp=%.0f", ms, sec)
	}
}

func TestDateTime_DayOfYearAndWeekNumber(t *testing.T) {
	result := handleDateTime(nil)
	r := parseDateTimeResult(t, result)

	doy := int(r["day_of_year"].(float64))
	if doy < 1 || doy > 366 {
		t.Errorf("day_of_year = %d, out of range [1,366]", doy)
	}

	wn := int(r["week_number"].(float64))
	if wn < 1 || wn > 53 {
		t.Errorf("week_number = %d, out of range [1,53]", wn)
	}
}

func TestDateTime_UTCFieldValues(t *testing.T) {
	result := handleDateTime(map[string]any{"timezone": "Europe/London"})
	r := parseDateTimeResult(t, result)

	utcTime, _ := r["utc_time"].(string)
	utcDate, _ := r["utc_date"].(string)

	if len(utcTime) != 8 || utcTime[2] != ':' {
		t.Errorf("utc_time format = %q, want HH:MM:SS", utcTime)
	}
	if len(utcDate) != 10 || utcDate[4] != '-' {
		t.Errorf("utc_date format = %q, want YYYY-MM-DD", utcDate)
	}
}

func TestServer_ToolsListIncludesDatetime(t *testing.T) {
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
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
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
			t.Fatal("timeout waiting for tools/list")
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
			if raw["id"] == float64(1) && raw["result"] != nil {
				result := raw["result"].(map[string]any)
				tools := result["tools"].([]any)
				var names []string
				for _, t := range tools {
					names = append(names, t.(map[string]any)["name"].(string))
				}
				found := false
				for _, n := range names {
					if n == "get_datetime" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("tools/list missing get_datetime. tools: %v", names)
				}
				return
			}
		}
	}
}

func TestServer_DatetimeToolCall(t *testing.T) {
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
		body := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_datetime","arguments":{"timezone":"UTC"}}}`
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
			t.Fatal("timeout waiting for datetime result")
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
			if raw["id"] == float64(5) {
				result := raw["result"].(map[string]any)
				if isErr, _ := result["isError"].(bool); isErr {
					t.Fatalf("datetime call returned error: %v", result["content"])
				}
				content := result["content"].([]any)
				item := content[0].(map[string]any)
				text := item["text"].(string)
				var dt map[string]any
				if err := json.Unmarshal([]byte(text), &dt); err != nil {
					t.Fatalf("bad datetime JSON: %v", err)
				}
				if dt["timezone"] != "UTC" {
					t.Errorf("timezone = %q", dt["timezone"])
				}
				if _, ok := dt["iso_8601"]; !ok {
					t.Error("missing iso_8601")
				}
				return
			}
		}
	}
}

func TestTools_GenerateUUID_Format(t *testing.T) {
	result := handleUUID(nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	uuid := result.Content[0].Text
	if len(uuid) != 36 {
		t.Errorf("UUID length = %d, want 36", len(uuid))
	}
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Errorf("UUID parts = %d, want 5", len(parts))
	}
	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Errorf("UUID part lengths: %d %d %d %d %d, want 8 4 4 4 12",
			len(parts[0]), len(parts[1]), len(parts[2]), len(parts[3]), len(parts[4]))
	}
	for _, p := range parts {
		for _, c := range p {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("UUID contains non-hex char %q in part %s", c, p)
			}
		}
	}
	if parts[2][0] != '4' {
		t.Errorf("UUID version = %c, want 4", parts[2][0])
	}
	variant := parts[3][0]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
		t.Errorf("UUID variant = %c, want 8/9/a/b", variant)
	}
}

func TestTools_GenerateUUID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		result := handleUUID(nil)
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.Content[0].Text)
		}
		uuid := result.Content[0].Text
		if seen[uuid] {
			t.Errorf("duplicate UUID generated: %s", uuid)
		}
		seen[uuid] = true
	}
}

func TestTools_Base64Encode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"hello", "hello", "aGVsbG8="},
		{"url", "https://example.com", "aHR0cHM6Ly9leGFtcGxlLmNvbQ=="},
		{"unicode", "héllo wörld", "aMOpbGxvIHfDtnJsZA=="},
		{"binary", "\x00\x01\x02\xff", "AAEC/w=="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"text": tt.input}
			result := handleBase64Encode(args)
			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
			got := result.Content[0].Text
			if got != tt.want {
				t.Errorf("encode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTools_Base64Encode_MissingArg(t *testing.T) {
	result := handleBase64Encode(map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error for empty input: %s", result.Content[0].Text)
	}
	if result.Content[0].Text != "" {
		t.Errorf("got %q, want empty", result.Content[0].Text)
	}
}

func TestTools_Base64Decode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"hello", "aGVsbG8=", "hello", false},
		{"url", "aHR0cHM6Ly9leGFtcGxlLmNvbQ==", "https://example.com", false},
		{"unicode", "aMOpbGxvIHfDtnJsZA==", "héllo wörld", false},
		{"unpadded", "aGVsbG8", "hello", false},
		{"invalid char", "!!!invalid!!!", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"encoded": tt.input}
			result := handleBase64Decode(args)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error but got none")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
			got := result.Content[0].Text
			if got != tt.want {
				t.Errorf("decode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTools_Base64Decode_MissingEncoded(t *testing.T) {
	result := handleBase64Decode(map[string]any{})
	if !result.IsError {
		t.Fatal("expected error for missing 'encoded' arg")
	}
}

func TestTools_Base64_RoundTrip(t *testing.T) {
	inputs := []string{"", "a", "hello world", "héllo wörld", "https://example.com/path?q=test&x=1"}
	for _, in := range inputs {
		encResult := handleBase64Encode(map[string]any{"text": in})
		if encResult.IsError {
			t.Fatalf("encode failed: %s", encResult.Content[0].Text)
		}
		encoded := encResult.Content[0].Text
		decResult := handleBase64Decode(map[string]any{"encoded": encoded})
		if decResult.IsError {
			t.Fatalf("decode failed: %s", decResult.Content[0].Text)
		}
		decoded := decResult.Content[0].Text
		if decoded != in {
			t.Errorf("round-trip mismatch: %q -> %q -> %q", in, encoded, decoded)
		}
	}
}

func TestTools_HashString_Known(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		algorithm string
		want      string
	}{
		{"sha256 empty", "", "sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"sha256 hello", "hello", "sha256", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{"sha512 empty", "", "sha512", "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"},
		{"md5 empty", "", "md5", "d41d8cd98f00b204e9800998ecf8427e"},
		{"md5 hello", "hello", "md5", "5d41402abc4b2a76b9719d911017c592"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"text": tt.text, "algorithm": tt.algorithm}
			result := handleHash(args)
			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
			got := result.Content[0].Text
			if got != tt.want {
				t.Errorf("hash(%q, %s) = %q, want %q", tt.text, tt.algorithm, got, tt.want)
			}
		})
	}
}

func TestTools_HashString_DefaultAlgorithm(t *testing.T) {
	args := map[string]any{"text": "hello"}
	result := handleHash(args)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	got := result.Content[0].Text
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("default hash = %q, want sha256 of 'hello' = %q", got, want)
	}
}

func TestTools_HashString_InvalidAlgorithm(t *testing.T) {
	args := map[string]any{"text": "hello", "algorithm": "sha1"}
	result := handleHash(args)
	if !result.IsError {
		t.Fatal("expected error for unsupported algorithm")
	}
	if !strings.Contains(result.Content[0].Text, "unsupported") {
		t.Errorf("error message = %q, want contains 'unsupported'", result.Content[0].Text)
	}
}

func TestTools_HashString_DifferentInputs(t *testing.T) {
	algo := "sha256"
	inputs := []string{"", "a", "b", "hello", "world", strings.Repeat("x", 10000)}
	seen := make(map[string]bool)
	for _, in := range inputs {
		args := map[string]any{"text": in, "algorithm": algo}
		result := handleHash(args)
		if result.IsError {
			t.Fatalf("hash(%q) failed: %s", in, result.Content[0].Text)
		}
		hash := result.Content[0].Text
		if seen[hash] {
			t.Errorf("hash collision for distinct inputs: %q", in)
		}
		seen[hash] = true
	}
}

func TestTools_RandomString_Length(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"zero (defaults to 16)", 0},
		{"one", 1},
		{"default length", 16},
		{"large", 1000},
		{"max", 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"length": float64(tt.length)}
			result := handleRandomString(args)
			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
			got := result.Content[0].Text
		wantLen := tt.length
		if wantLen <= 0 {
			wantLen = 16 // zero/negative defaults to 16
		}
		if wantLen > 4096 {
			wantLen = 4096
		}
			if len(got) != wantLen {
				t.Errorf("length = %d, want %d", len(got), wantLen)
			}
		})
	}
}

func TestTools_RandomString_Charset(t *testing.T) {
	tests := []struct {
		name    string
		charset string
		allowed string
	}{
		{"alphanumeric", "alphanumeric", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"},
		{"alphabetic", "alphabetic", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"},
		{"numeric", "numeric", "0123456789"},
		{"hex", "hex", "0123456789abcdef"},
		{"ascii", "ascii", " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"length": float64(100), "charset": tt.charset}
			result := handleRandomString(args)
			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
			got := result.Content[0].Text
			if len(got) != 100 {
				t.Errorf("length = %d, want 100", len(got))
			}
			for _, c := range got {
				if !strings.ContainsRune(tt.allowed, c) {
					t.Errorf("char %q not in allowed charset %s", c, tt.charset)
				}
			}
		})
	}
}

func TestTools_RandomString_DefaultCharset(t *testing.T) {
	args := map[string]any{"length": float64(50)}
	result := handleRandomString(args)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	got := result.Content[0].Text
	if len(got) != 50 {
		t.Errorf("length = %d, want 50", len(got))
	}
	alphaNum := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for _, c := range got {
		if !strings.ContainsRune(alphaNum, c) {
			t.Errorf("default charset char %q not in alphanumeric", c)
		}
	}
}

func TestTools_RandomString_InvalidCharset(t *testing.T) {
	args := map[string]any{"length": float64(10), "charset": "nonexistent"}
	result := handleRandomString(args)
	if !result.IsError {
		t.Fatal("expected error for invalid charset")
	}
	if !strings.Contains(result.Content[0].Text, "unsupported") {
		t.Errorf("error message = %q, want contains 'unsupported'", result.Content[0].Text)
	}
}

func TestTools_RandomString_MaxLength(t *testing.T) {
	args := map[string]any{"length": float64(5000)}
	result := handleRandomString(args)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "truncated to 4096 (requested 5000)") {
		t.Errorf("expected truncation warning, got: %q", text)
	}
	// The random string portion (after warning + newline) should be 4096
	parts := strings.SplitN(text, "\n", 2)
	if len(parts) != 2 || len(parts[1]) != 4096 {
		t.Errorf("random string length = %d, want 4096", len(parts[1]))
	}
}

func TestTools_RandomString_Uniqueness(t *testing.T) {
	args := map[string]any{"length": float64(100), "charset": "numeric"}
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		result := handleRandomString(args)
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.Content[0].Text)
		}
		s := result.Content[0].Text
		if seen[s] {
			t.Errorf("duplicate random string generated")
		}
		seen[s] = true
		for _, c := range s {
			if c < '0' || c > '9' {
				t.Errorf("non-digit char %q in numeric random string", c)
			}
		}
	}
}

func TestTools_RandomString_EmptyArgs(t *testing.T) {
	result := handleRandomString(map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error for empty args: %s", result.Content[0].Text)
	}
	if len(result.Content[0].Text) != 16 {
		t.Errorf("default length = %d, want 16", len(result.Content[0].Text))
	}
}

func TestTools_UtilityToolsViaHTTP(t *testing.T) {
	cfg := DefaultConfig()
	mcp := NewMCPServer(cfg)
	ts := httptest.NewServer(mcp)
	defer ts.Close()

	toolTests := []struct {
		name    string
		method  string
		params  map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name:   "generate_uuid",
			method: "generate_uuid",
			params: map[string]any{},
			check: func(t *testing.T, text string) {
				if len(text) != 36 {
					t.Errorf("UUID length = %d, want 36", len(text))
				}
			},
		},
		{
			name:   "base64_encode",
			method: "base64_encode",
			params: map[string]any{"text": "hello"},
			check: func(t *testing.T, text string) {
				if text != "aGVsbG8=" {
					t.Errorf("got %q, want aGVsbG8=", text)
				}
			},
		},
		{
			name:   "base64_decode",
			method: "base64_decode",
			params: map[string]any{"encoded": "aGVsbG8="},
			check: func(t *testing.T, text string) {
				if text != "hello" {
					t.Errorf("got %q, want hello", text)
				}
			},
		},
		{
			name:    "base64_decode invalid",
			method:  "base64_decode",
			params:  map[string]any{"encoded": "!!!invalid!!!"},
			wantErr: true,
		},
		{
			name:   "hash_string sha256",
			method: "hash_string",
			params: map[string]any{"text": "hello", "algorithm": "sha256"},
			check: func(t *testing.T, text string) {
				want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
				if text != want {
					t.Errorf("got %q, want %q", text, want)
				}
			},
		},
		{
			name:    "hash_string invalid algo",
			method:  "hash_string",
			params:  map[string]any{"text": "hello", "algorithm": "sha1"},
			wantErr: true,
		},
		{
			name:   "generate_random_string",
			method: "generate_random_string",
			params: map[string]any{"length": float64(20), "charset": "numeric"},
			check: func(t *testing.T, text string) {
				if len(text) != 20 {
					t.Errorf("length = %d, want 20", len(text))
				}
				for _, c := range text {
					if c < '0' || c > '9' {
						t.Errorf("non-digit char %q", c)
					}
				}
			},
		},
		{
			name:    "generate_random_string invalid charset",
			method:  "generate_random_string",
			params:  map[string]any{"charset": "nonexistent"},
			wantErr: true,
		},
	}

	for _, tt := range toolTests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/call",
				"params": map[string]any{
					"name":      tt.method,
					"arguments": tt.params,
				},
			}
			rawBody, _ := json.Marshal(body)
			resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(string(rawBody)))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			var raw map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				t.Fatal(err)
			}

			if tt.wantErr {
				if _, ok := raw["error"]; !ok {
					result, _ := raw["result"].(map[string]any)
					if result != nil {
						if isErr, _ := result["isError"].(bool); isErr {
							return
						}
					}
					t.Fatalf("expected error for %s, got success: %v", tt.method, raw)
				}
				return
			}

			if errVal, ok := raw["error"]; ok {
				t.Fatalf("%s returned error: %v", tt.method, errVal)
			}

			result, ok := raw["result"].(map[string]any)
			if !ok {
				t.Fatalf("missing result: %v", raw)
			}
			if isErr, _ := result["isError"].(bool); isErr {
				t.Fatalf("%s returned isError: %v", tt.method, result["content"])
			}
			content, _ := result["content"].([]any)
			if len(content) == 0 {
				t.Fatal("missing content")
			}
			item, _ := content[0].(map[string]any)
			text, _ := item["text"].(string)
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

func TestTools_UtilityToolList(t *testing.T) {
	cfg := DefaultConfig()
	mcp := NewMCPServer(cfg)
	ts := httptest.NewServer(mcp)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var raw map[string]any
	json.NewDecoder(resp.Body).Decode(&raw)

	result, _ := raw["result"].(map[string]any)
	tools, _ := result["tools"].([]any)

	expected := map[string]bool{
		"generate_uuid":          false,
		"base64_encode":          false,
		"base64_decode":          false,
		"hash_string":            false,
		"generate_random_string": false,
	}
	for _, tRaw := range tools {
		tool, _ := tRaw.(map[string]any)
		name, _ := tool["name"].(string)
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("utility tool %q not found in tools/list", name)
		}
	}
}
