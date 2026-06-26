package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]PropertyDef  `json:"properties"`
	Required   []string                `json:"required,omitempty"`
}

type PropertyDef struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ExternalResource struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

var searchTool = Tool{
	Name:        "searxng_search",
	Description: "Search the web using SearXNG metasearch engine. Returns JSON results with titles, URLs, snippets, engines, and scores.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"query": {
				Type:        "string",
				Description: "Search query (supports SearXNG syntax like site:example.com)",
			},
			"categories": {
				Type:        "string",
				Description: "Search categories: general, images, news, files, map, music, it, science, social media",
				Default:     "general",
			},
			"language": {
				Type:        "string",
				Description: "Language code (e.g. en, de, fr). Empty for auto-detect.",
				Default:     "",
			},
			"pageno": {
				Type:        "number",
				Description: "Page number of results (1-based)",
				Default:     1,
			},
			"time_range": {
				Type:        "string",
				Description: "Time range filter: day, week, month, year. Empty for no filter.",
				Default:     "",
			},
			"safesearch": {
				Type:        "number",
				Description: "SafeSearch level: 0 (off), 1 (moderate), 2 (strict)",
				Default:     0,
			},
			"max_results": {
				Type:        "number",
				Description: "Maximum number of results to return",
				Default:     10,
			},
			"engines": {
				Type:        "string",
				Description: "Comma-separated list of specific engines (e.g. google,bing,reddit,wikipedia)",
				Default:     "",
			},
		},
		Required: []string{"query"},
	},
}

var fetchTool = Tool{
	Name:        "searxng_fetch",
	Description: "Fetch the content of a web page. Returns page content with metadata (status code, content type, headers, connection info, external resources).",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"url": {
				Type:        "string",
				Description: "Full URL of the page to fetch",
			},
			"max_length": {
				Type:        "number",
				Description: "Maximum content length in bytes to return (0 = unlimited)",
				Default:     1_048_576,
			},
			"start_index": {
				Type:        "number",
				Description: "Character offset into the returned body to start reading from (for paginated/chunked reading of processed output)",
				Default:     0,
			},
			"format": {
				Type:        "string",
				Description: "Output format: text (cleaned plain text), markdown (HTML→Markdown), raw (unprocessed body), html (raw HTML), full (raw + unlimited + all metadata)",
				Default:     "text",
			},
			"mode": {
				Type:        "string",
				Description: "Content extraction mode: 'full' (default, strip only scripts/styles, keep all content) or 'smart' (Mozilla Readability: extract main article, strip navigation/sidebars/comments)",
				Default:     "full",
			},
			"check_llms_txt": {
				Type:        "boolean",
				Description: "Fetch {origin}/llms.txt and include in response",
				Default:     false,
			},
			"headers": {
				Type:        "object",
				Description: "Custom HTTP headers to include in the request (key-value pairs)",
				Default:     nil,
			},
			"cookies": {
				Type:        "string",
				Description: "Cookie header value to include (e.g. from browser devtools). Some sites require cookies from a real session.",
				Default:     "",
			},
			"proxy": {
				Type:        "string",
				Description: "Reader-mode proxy URL template (e.g. 'https://r.jina.ai/' or 'https://12ft.io/proxy?q='). The target URL is appended to this prefix. Useful for JS-heavy sites like Reuters.",
				Default:     "",
			},
		},
		Required: []string{"url"},
	},
}

var datetimeTool = Tool{
	Name:        "get_datetime",
	Description: "Get the current date, time, timezone, and timestamp information. Returns all common date/time representations in a single JSON response.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"timezone": {
				Type:        "string",
				Description: "IANA timezone name (e.g. 'America/New_York', 'Europe/London', 'Asia/Tokyo'). Defaults to 'UTC'.",
				Default:     "UTC",
			},
		},
	},
}

var uuidTool = Tool{
	Name:        "generate_uuid",
	Description: "Generate a random UUID v4 string. Uses cryptographic random number generator.",
	InputSchema: InputSchema{
		Type:       "object",
		Properties: map[string]PropertyDef{},
	},
}

var base64EncodeTool = Tool{
	Name:        "base64_encode",
	Description: "Encode a string to Base64.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"text": {Type: "string", Description: "Text to encode"},
		},
		Required: []string{"text"},
	},
}

var base64DecodeTool = Tool{
	Name:        "base64_decode",
	Description: "Decode a Base64 string back to text.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"encoded": {Type: "string", Description: "Base64 encoded string"},
		},
		Required: []string{"encoded"},
	},
}

var hashTool = Tool{
	Name:        "hash_string",
	Description: "Generate a cryptographic hash of a string. Supports sha256, sha512, and md5 algorithms.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"text":      {Type: "string", Description: "Text to hash"},
			"algorithm": {Type: "string", Description: "Hash algorithm: sha256, sha512, md5", Default: "sha256"},
		},
		Required: []string{"text"},
	},
}

var charsetMap = map[string]string{
	"alphanumeric": "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
	"alphabetic":   "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
	"numeric":      "0123456789",
	"hex":          "0123456789abcdef",
	"ascii":        " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~",
}

var dnsLookupTool = Tool{
	Name:        "dns_lookup",
	Description: "DNS lookup supporting all common record types (A, AAAA, MX, NS, CNAME, TXT, SRV, PTR, SOA, ALL). Supports custom DNS server and port. Reverse lookup via PTR type on an IP address. No caching: every call performs a fresh query.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"name": {
				Type:        "string",
				Description: "Hostname or IP address to look up. For PTR type, pass an IP address (auto-converted to in-addr.arpa / ip6.arpa).",
			},
			"type": {
				Type:        "string",
				Description: "Record type: A, AAAA, MX, NS, CNAME, TXT, SRV, PTR, SOA, or ALL (queries all types and merges results).",
				Default:     "A",
			},
			"server": {
				Type:        "string",
				Description: "DNS server IP address (e.g. '8.8.8.8'). Empty = use system nameservers (Linux: /etc/resolv.conf, Windows: falls back to 1.1.1.1).",
				Default:     "",
			},
			"port": {
				Type:        "number",
				Description: "DNS server port number (1-65535).",
				Default:     53,
			},
		},
		Required: []string{"name"},
	},
}

var randomStringTool = Tool{
	Name:        "generate_random_string",
	Description: "Generate a cryptographically secure random string with the specified length and character set.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"length": {
				Type:        "number",
				Description: "Length of the random string",
				Default:     16,
			},
			"charset": {
				Type:        "string",
				Description: "Character set: alphanumeric, alphabetic, numeric, hex, or ascii",
				Default:     "alphanumeric",
			},
		},
	},
}

var newsSearchTool = Tool{
	Name:        "searxng_search_news",
	Description: "Search news using SearXNG. Shortcut for search with category=news and time_range=week. Returns JSON results with titles, URLs, snippets, engines, and scores.",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"query": {
				Type:        "string",
				Description: "News search query",
			},
			"language": {
				Type:        "string",
				Description: "Language code (e.g. en, de, fr). Empty for auto-detect.",
				Default:     "",
			},
			"max_results": {
				Type:        "number",
				Description: "Maximum number of results to return",
				Default:     10,
			},
			"time_range": {
				Type:        "string",
				Description: "Time range filter: day, week, month, year. Empty for no filter.",
				Default:     "week",
			},
			"safesearch": {
				Type:        "number",
				Description: "SafeSearch level: 0 (off), 1 (moderate), 2 (strict)",
				Default:     0,
			},
			"engines": {
				Type:        "string",
				Description: "Comma-separated list of specific engines (e.g. google,bing,reddit)",
				Default:     "",
			},
		},
		Required: []string{"query"},
	},
}

var fetchManyTool = Tool{
	Name:        "searxng_fetch_many",
	Description: "Fetch multiple web pages in parallel. Returns an array of page contents with metadata (status code, content type, headers, connection info, external resources).",
	InputSchema: InputSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"urls": {
				Type:        "array",
				Description: "Array of URLs to fetch",
			},
			"max_length": {
				Type:        "number",
				Description: "Maximum content length in bytes per page (0 = unlimited)",
				Default:     1_048_576,
			},
			"start_index": {
				Type:        "number",
				Description: "Character offset into the returned body to start reading from (for paginated/chunked reading of processed output)",
				Default:     0,
			},
			"format": {
				Type:        "string",
				Description: "Output format: text (cleaned plain text), markdown (HTML→Markdown), raw (unprocessed body), html (raw HTML), full (raw + unlimited + all metadata)",
				Default:     "text",
			},
			"mode": {
				Type:        "string",
				Description: "Content extraction mode: 'full' (default, strip only scripts/styles, keep all content) or 'smart' (Mozilla Readability: extract main article, strip navigation/sidebars/comments)",
				Default:     "full",
			},
			"check_llms_txt": {
				Type:        "boolean",
				Description: "Fetch {origin}/llms.txt and include in response",
				Default:     false,
			},
			"cookies": {
				Type:        "string",
				Description: "Cookie header value (e.g. from browser devtools). Applies to all URLs.",
				Default:     "",
			},
			"proxy": {
				Type:        "string",
				Description: "Reader-mode proxy URL template (e.g. 'https://r.jina.ai/' or 'https://12ft.io/proxy?q='). Target URL appended.",
				Default:     "",
			},
		},
		Required: []string{"urls"},
	},
}

func handleToolCall(name string, args map[string]any, cfg *Config, searxng *SearXNGClient) ToolCallResult {
	switch name {
	case "searxng_search":
		return handleSearch(args, cfg, searxng)
	case "searxng_search_news":
		return handleNewsSearch(args, cfg, searxng)
	case "searxng_fetch":
		return handleFetch(args, cfg)
	case "searxng_fetch_many":
		return handleFetchMany(args, cfg)
	case "get_datetime":
		return handleDateTime(args)
	case "generate_uuid":
		return handleUUID(args)
	case "base64_encode":
		return handleBase64Encode(args)
	case "base64_decode":
		return handleBase64Decode(args)
	case "hash_string":
		return handleHash(args)
	case "generate_random_string":
		return handleRandomString(args)
	case "dns_lookup":
		return handleDNSLookup(args)
	case "url_encode":
		return handleURLEncode(args)
	case "url_decode":
		return handleURLDecode(args)
	case "hex_encode":
		return handleHexEncode(args)
	case "hex_decode":
		return handleHexDecode(args)
	case "jwt_decode":
		return handleJWTDecode(args)
	case "hash_identify":
		return handleHashIdentify(args)
	case "xor_cipher":
		return handleXorCipher(args)
	case "whois_lookup":
		return handleWhoisLookup(args)
	case "ssl_cert_info":
		return handleSSLCertInfo(args)
	default:
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("unknown tool: %s", name),
			}},
		}
	}
}

func handleSearch(args map[string]any, cfg *Config, searxng *SearXNGClient) ToolCallResult {
	query, _ := args["query"].(string)
	if query == "" {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "query is required"}},
		}
	}

	params := SearchParams{
		Query:      query,
		MaxResults: cfg.Search.DefaultMaxResults,
	}

	if v, ok := args["categories"].(string); ok && v != "" {
		params.Categories = v
	} else {
		params.Categories = cfg.Search.DefaultCategories
	}
	if v, ok := args["language"].(string); ok && v != "" {
		params.Language = v
	} else {
		params.Language = cfg.Search.DefaultLanguage
	}
	if v, ok := args["pageno"].(float64); ok && v > 0 {
		params.PageNo = int(v)
	}
	if v, ok := args["time_range"].(string); ok && v != "" {
		params.TimeRange = v
	}
	if v, ok := args["safesearch"].(float64); ok {
		params.Safesearch = int(v)
	} else {
		params.Safesearch = cfg.Search.DefaultSafesearch
	}
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		n := int(v)
		if n > cfg.Search.MaxMaxResults {
			n = cfg.Search.MaxMaxResults
		}
		params.MaxResults = n
	}
	if v, ok := args["engines"].(string); ok && v != "" {
		for _, e := range strings.Split(v, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				params.Engines = append(params.Engines, e)
			}
		}
	}

	resp, respHeaders, err := searxng.Search(params)
	if err != nil {
		errData := map[string]any{"error": err.Error()}
		for k, v := range respHeaders {
			errData[k] = v
		}
		raw, _ := json.Marshal(errData)
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: string(raw)}},
		}
	}

	envelope := map[string]any{
		"query":                resp.Query,
		"number_of_results":    resp.NumberOfResults,
		"results":              resp.Results,
		"answers":              resp.Answers,
		"corrections":          resp.Corrections,
		"suggestions":          resp.Suggestions,
		"infoboxes":            resp.Infoboxes,
		"unresponsive_engines": resp.UnresponsiveEngines,
	}
	for k, v := range respHeaders {
		envelope[k] = v
	}

	raw, _ := json.MarshalIndent(envelope, "", "  ")
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(raw)}},
	}
}

func handleFetch(args map[string]any, cfg *Config) ToolCallResult {
	urlStr, _ := args["url"].(string)
	if urlStr == "" {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "url is required"}},
		}
	}

	maxLength := cfg.Fetch.MaxContentLength
	if v, ok := args["max_length"].(float64); ok {
		if v > 0 {
			maxLength = int(v)
		} else if v == 0 {
			maxLength = math.MaxInt
		}
	}

	startIndex := 0
	if v, ok := args["start_index"].(float64); ok && v >= 0 {
		startIndex = int(v)
	}

	mode := "full"
	if v, ok := args["mode"].(string); ok && v != "" {
		mode = v
	}

	checkLLMSTxt := false
	if v, ok := args["check_llms_txt"].(bool); ok {
		checkLLMSTxt = v
	}

	fetchFormat, _ := args["format"].(string)
	if fetchFormat == "" {
		fetchFormat = "text"
	}

	customHeaders := map[string]string{}
	if h, ok := args["headers"].(map[string]any); ok {
		for k, val := range h {
			if s, ok := val.(string); ok {
				customHeaders[k] = s
			}
		}
	}

	cookies, _ := args["cookies"].(string)

	connInfo := map[string]any{}

	proxy, _ := args["proxy"].(string)
	if proxy != "" && !strings.HasPrefix(urlStr, proxy) {
		connInfo["original_url"] = urlStr
		connInfo["proxy"] = proxy
		urlStr = proxy + urlStr
	}

	var redirectChain []string

	client := &http.Client{
		Timeout: time.Duration(cfg.Fetch.Timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			redirectChain = append(redirectChain, req.URL.String())
			return nil
		},
	}

	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("invalid url: %s", err)}},
		}
	}
	req.Header.Set("User-Agent", cfg.Fetch.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	for k, v := range customHeaders {
		req.Header.Set(k, v)
	}

	trace := &httptrace.ClientTrace{
		GotConn: func(ci httptrace.GotConnInfo) {
			if ci.Conn != nil {
				connInfo["remote_addr"] = ci.Conn.RemoteAddr().String()
				connInfo["local_addr"] = ci.Conn.LocalAddr().String()
			}
		},
		DNSDone: func(di httptrace.DNSDoneInfo) {
			addrs := make([]string, len(di.Addrs))
			for i, a := range di.Addrs {
				addrs[i] = a.String()
			}
			connInfo["dns_results"] = addrs
		},
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			if err == nil {
				connInfo["tls_version"] = tlsVersionString(cs.Version)
				connInfo["tls_cipher"] = tls.CipherSuiteName(cs.CipherSuite)
				connInfo["tls_server_name"] = cs.ServerName
			}
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := client.Do(req)
	if err != nil {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("fetch failed: %s", err)}},
		}
	}
	defer resp.Body.Close()

	if fetchFormat == "full" {
		maxLength = math.MaxInt
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("read failed: %s", err)}},
		}
	}

	truncated := len(raw) > maxLength
	if truncated {
		raw = raw[:maxLength]
	}

	// Extract external resources from raw HTML before any processing
	resources := extractExternalResources(string(raw), urlStr)

	ct := resp.Header.Get("content-type")
	isHTML := strings.Contains(ct, "text/html") || strings.Contains(ct, "xhtml")
	skipProcessing := fetchFormat == "raw" || fetchFormat == "html" || fetchFormat == "full"

	bodyStr := string(raw)
	var body string
	if skipProcessing {
		body = bodyStr
	} else if fetchFormat == "markdown" {
		if isHTML {
			body = htmlToMarkdown(bodyStr, mode)
		} else {
			body = bodyStr
		}
	} else {
		// text format (default)
		if isHTML {
			body = htmlToText(bodyStr, mode)
		} else {
			body = bodyStr
		}
	}

	// Apply start_index on the final processed body (not the raw HTML)
	if startIndex > 0 {
		if startIndex < len(body) {
			body = body[startIndex:]
		} else {
			body = ""
		}
	}

	selectedHeaders := map[string]string{}
	for _, h := range []string{"content-type", "content-length", "server", "date", "x-robots-tag", "x-frame-options"} {
		if v := resp.Header.Get(h); v != "" {
			selectedHeaders[h] = v
		}
	}

	result := map[string]any{
		"url":                 urlStr,
		"status_code":         resp.StatusCode,
		"content_type":        ct,
		"content_length":      len(raw),
		"truncated":           truncated,
		"headers":             selectedHeaders,
		"body":                body,
		"external_resources":  resources,
	}

	if len(redirectChain) > 0 {
		result["redirect_chain"] = redirectChain
	}
	if len(connInfo) > 0 {
		if _, ok := connInfo["dns_results"]; !ok {
			connInfo["dns_results"] = []string{}
		}
		result["connection"] = connInfo
	}
	if checkLLMSTxt {
		if llms := fetchLLMSTXT(urlStr); llms != "" {
			result["llms_txt"] = llms
		} else {
			result["llms_txt"] = nil
		}
	}

	rawJSON, _ := json.MarshalIndent(result, "", "  ")
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(rawJSON)}},
	}
}

func handleNewsSearch(args map[string]any, cfg *Config, searxng *SearXNGClient) ToolCallResult {
	query, _ := args["query"].(string)
	if query == "" {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "query is required"}},
		}
	}

	params := SearchParams{
		Query:      query,
		Categories: "news",
		MaxResults: cfg.Search.DefaultMaxResults,
	}

	if v, ok := args["language"].(string); ok && v != "" {
		params.Language = v
	}
	if v, ok := args["time_range"].(string); ok && v != "" {
		params.TimeRange = v
	} else {
		params.TimeRange = "week"
	}
	if v, ok := args["safesearch"].(float64); ok {
		params.Safesearch = int(v)
	} else {
		params.Safesearch = cfg.Search.DefaultSafesearch
	}
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		n := int(v)
		if n > cfg.Search.MaxMaxResults {
			n = cfg.Search.MaxMaxResults
		}
		params.MaxResults = n
	}
	if v, ok := args["engines"].(string); ok && v != "" {
		for _, e := range strings.Split(v, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				params.Engines = append(params.Engines, e)
			}
		}
	}

	resp, respHeaders, err := searxng.Search(params)
	if err != nil {
		errData := map[string]any{"error": err.Error()}
		for k, v := range respHeaders {
			errData[k] = v
		}
		raw, _ := json.Marshal(errData)
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: string(raw)}},
		}
	}

	envelope := map[string]any{
		"query":                resp.Query,
		"number_of_results":    resp.NumberOfResults,
		"results":              resp.Results,
		"answers":              resp.Answers,
		"corrections":          resp.Corrections,
		"suggestions":          resp.Suggestions,
		"infoboxes":            resp.Infoboxes,
		"unresponsive_engines": resp.UnresponsiveEngines,
	}
	for k, v := range respHeaders {
		envelope[k] = v
	}

	raw, _ := json.MarshalIndent(envelope, "", "  ")
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(raw)}},
	}
}

func handleFetchMany(args map[string]any, cfg *Config) ToolCallResult {
	urlsRaw, ok := args["urls"].([]any)
	if !ok || len(urlsRaw) == 0 {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "urls is required (non-empty array)"}},
		}
	}

	urls := make([]string, 0, len(urlsRaw))
	for _, u := range urlsRaw {
		if s, ok := u.(string); ok && s != "" {
			urls = append(urls, s)
		}
	}
	if len(urls) == 0 {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "urls must contain at least one non-empty string"}},
		}
	}

	maxLength := cfg.Fetch.MaxContentLength
	if v, ok := args["max_length"].(float64); ok {
		if v > 0 {
			maxLength = int(v)
		} else if v == 0 {
			maxLength = math.MaxInt
		}
	}

	startIndex := 0
	if v, ok := args["start_index"].(float64); ok && v >= 0 {
		startIndex = int(v)
	}

	mode := "full"
	if v, ok := args["mode"].(string); ok && v != "" {
		mode = v
	}

	checkLLMSTxt := false
	if v, ok := args["check_llms_txt"].(bool); ok {
		checkLLMSTxt = v
	}

	cookies, _ := args["cookies"].(string)
	proxy, _ := args["proxy"].(string)

	fetchFormat, _ := args["format"].(string)
	if fetchFormat == "" {
		fetchFormat = "text"
	}

	type fetchResult struct {
		URL   string         `json:"url"`
		Error string         `json:"error,omitempty"`
		Data  map[string]any `json:"data,omitempty"`
	}

	results := make([]fetchResult, len(urls))
	sem := make(chan struct{}, cfg.Fetch.MaxConcurrent)
	done := make(chan struct{}, len(urls))

	for i, u := range urls {
		sem <- struct{}{}
		go func(idx int, urlStr string) {
			defer func() { <-sem; done <- struct{}{} }()

			connInfo := map[string]any{}
			var redirectChain []string

			client := &http.Client{
				Timeout: time.Duration(cfg.Fetch.Timeout) * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					if len(via) >= 10 {
						return fmt.Errorf("too many redirects")
					}
					redirectChain = append(redirectChain, req.URL.String())
					return nil
				},
			}

			fetchURL := urlStr
			if proxy != "" && !strings.HasPrefix(fetchURL, proxy) {
				fetchURL = proxy + fetchURL
			}

			req, err := http.NewRequest(http.MethodGet, fetchURL, nil)
			if err != nil {
				results[idx] = fetchResult{URL: urlStr, Error: fmt.Sprintf("invalid url: %s", err)}
				return
			}
			req.Header.Set("User-Agent", cfg.Fetch.UserAgent)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			req.Header.Set("Sec-Fetch-Site", "none")
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			req.Header.Set("Sec-Fetch-Dest", "document")
			if cookies != "" {
				req.Header.Set("Cookie", cookies)
			}

			trace := &httptrace.ClientTrace{
				GotConn: func(ci httptrace.GotConnInfo) {
					if ci.Conn != nil {
						connInfo["remote_addr"] = ci.Conn.RemoteAddr().String()
						connInfo["local_addr"] = ci.Conn.LocalAddr().String()
					}
				},
				DNSDone: func(di httptrace.DNSDoneInfo) {
					addrs := make([]string, len(di.Addrs))
					for i, a := range di.Addrs {
						addrs[i] = a.String()
					}
					connInfo["dns_results"] = addrs
				},
				TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
					if err == nil {
						connInfo["tls_version"] = tlsVersionString(cs.Version)
						connInfo["tls_cipher"] = tls.CipherSuiteName(cs.CipherSuite)
						connInfo["tls_server_name"] = cs.ServerName
					}
				},
			}
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

			resp, err := client.Do(req)
			if err != nil {
				results[idx] = fetchResult{URL: urlStr, Error: fmt.Sprintf("fetch failed: %s", err)}
				return
			}
			defer resp.Body.Close()

			ml := maxLength
			if fetchFormat == "full" {
				ml = math.MaxInt
			}

			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				results[idx] = fetchResult{URL: urlStr, Error: fmt.Sprintf("read failed: %s", err)}
				return
			}

			truncated := len(raw) > ml
			if truncated {
				raw = raw[:ml]
			}

			resources := extractExternalResources(string(raw), urlStr)

			ct := resp.Header.Get("content-type")
			isHTML := strings.Contains(ct, "text/html") || strings.Contains(ct, "xhtml")
			skipProcessing := fetchFormat == "raw" || fetchFormat == "html" || fetchFormat == "full"

			bodyStr := string(raw)
			var body string
			if skipProcessing {
				body = bodyStr
			} else if fetchFormat == "markdown" {
				if isHTML {
					body = htmlToMarkdown(bodyStr, mode)
				} else {
					body = bodyStr
				}
			} else {
				if isHTML {
					body = htmlToText(bodyStr, mode)
				} else {
					body = bodyStr
				}
			}

			// Apply start_index on the final processed body (not the raw HTML)
			if startIndex > 0 {
				if startIndex < len(body) {
					body = body[startIndex:]
				} else {
					body = ""
				}
			}

			selectedHeaders := map[string]string{}
			for _, h := range []string{"content-type", "content-length", "server", "date"} {
				if v := resp.Header.Get(h); v != "" {
					selectedHeaders[h] = v
				}
			}

			data := map[string]any{
				"url":                 urlStr,
				"status_code":         resp.StatusCode,
				"content_type":        ct,
				"content_length":      len(raw),
				"truncated":           truncated,
				"headers":             selectedHeaders,
				"body":                body,
				"external_resources":  resources,
			}

			if len(redirectChain) > 0 {
				data["redirect_chain"] = redirectChain
			}
			if len(connInfo) > 0 {
				if _, ok := connInfo["dns_results"]; !ok {
					connInfo["dns_results"] = []string{}
				}
				data["connection"] = connInfo
			}
			if checkLLMSTxt {
				if llms := fetchLLMSTXT(urlStr); llms != "" {
					data["llms_txt"] = llms
				} else {
					data["llms_txt"] = nil
				}
			}

			results[idx] = fetchResult{URL: urlStr, Data: data}
		}(i, u)
	}

	for range urls {
		<-done
	}

	raw, _ := json.MarshalIndent(results, "", "  ")
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(raw)}},
	}
}

func handleUUID(_ map[string]any) ToolCallResult {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	uuid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: uuid}},
	}
}

func handleBase64Encode(args map[string]any) ToolCallResult {
	text, _ := args["text"].(string)
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: base64.StdEncoding.EncodeToString([]byte(text))}},
	}
}

func handleBase64Decode(args map[string]any) ToolCallResult {
	encoded, ok := args["encoded"].(string)
	if !ok {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "invalid argument: 'encoded' must be a string"}},
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return ToolCallResult{
				IsError: true,
				Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("invalid base64: %s", err)}},
			}
		}
	}
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(decoded)}},
	}
}

func handleHash(args map[string]any) ToolCallResult {
	text, _ := args["text"].(string)
	algorithm, _ := args["algorithm"].(string)
	if algorithm == "" {
		algorithm = "sha256"
	}

	var hash string
	switch algorithm {
	case "sha256":
		h := sha256.Sum256([]byte(text))
		hash = hex.EncodeToString(h[:])
	case "sha512":
		h := sha512.Sum512([]byte(text))
		hash = hex.EncodeToString(h[:])
	case "md5":
		h := md5.Sum([]byte(text))
		hash = hex.EncodeToString(h[:])
	default:
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("unsupported algorithm: %s (use sha256, sha512, or md5)", algorithm)}},
		}
	}

	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: hash}},
	}
}

func handleRandomString(args map[string]any) ToolCallResult {
	length := 16
	if v, ok := args["length"].(float64); ok && v > 0 {
		length = int(v)
	}
	if length > 4096 {
		length = 4096
	}

	charset, _ := args["charset"].(string)
	if charset == "" {
		charset = "alphanumeric"
	}
	chars, ok := charsetMap[charset]
	if !ok {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("unsupported charset: %s (use alphanumeric, alphabetic, numeric, hex, or ascii)", charset)}},
		}
	}

	result := make([]byte, length)
	charsLen := big.NewInt(int64(len(chars)))
	for i := range result {
		n, err := rand.Int(rand.Reader, charsLen)
		if err != nil {
			return ToolCallResult{
				IsError: true,
				Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("random generation failed: %s", err)}},
			}
		}
		result[i] = chars[n.Int64()]
	}

	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(result)}},
	}
}

func handleDNSLookup(args map[string]any) ToolCallResult {
	name, ok := args["name"].(string)
	if !ok {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "missing required argument: 'name'"}},
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: "missing required argument: 'name'"}},
		}
	}
	rtype := "A"
	if v, ok := args["type"].(string); ok && v != "" {
		rtype = v
	}
	server, _ := args["server"].(string)
	port := 53
	if v, ok := args["port"].(float64); ok && v >= 1 && v <= 65535 {
		port = int(v)
	}
	records, err := dnsLookup(name, rtype, server, port)
	if err != nil {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{Type: "text", Text: err.Error()}},
		}
	}
	data, _ := json.Marshal(records)
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(data)}},
	}
}

func handleDateTime(args map[string]any) ToolCallResult {
	tzName := "UTC"
	if v, ok := args["timezone"].(string); ok && v != "" {
		tzName = v
	}

	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return ToolCallResult{
			IsError: true,
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("unknown timezone: %s", tzName),
			}},
		}
	}

	now := time.Now().In(loc)
	utc := now.UTC()
	_, offset := now.Zone()

	result := map[string]any{
		"iso_8601":       now.Format(time.RFC3339),
		"date":           now.Format("2006-01-02"),
		"time":           now.Format("15:04:05"),
		"timezone":       tzName,
		"utc_offset":     fmt.Sprintf("%+03d%02d", offset/3600, offset%3600/60),
		"unix_timestamp": now.Unix(),
		"unix_timestamp_ms": now.UnixMilli(),
		"day_of_week":    now.Weekday().String(),
		"day_of_year":    now.YearDay(),
		"week_number":    weekNumber(now),
		"year":           now.Year(),
		"month":          now.Format("01"),
		"month_name":     now.Month().String(),
		"day":            now.Format("02"),
		"hour_24":        now.Format("15"),
		"hour_12":        now.Format("03"),
		"minute":         now.Format("04"),
		"second":         now.Format("05"),
		"am_pm":          now.Format("PM"),
		"is_dst":         now.IsDST(),
		"utc_time":       utc.Format("15:04:05"),
		"utc_date":       utc.Format("2006-01-02"),
		"utc_iso_8601":   utc.Format(time.RFC3339),
	}

	raw, _ := json.MarshalIndent(result, "", "  ")
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: string(raw)}},
	}
}

func weekNumber(t time.Time) int {
	_, w := t.ISOWeek()
	return w
}

func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04X", v)
	}
}

var (
	tagRe              = regexp.MustCompile(`<[^>]*>`)
	newlineRe          = regexp.MustCompile(`(?is)<(br|/p|/div|/li|/h[1-6]|/tr|/blockquote|/pre|/section|/article|/header|/footer|/aside|/main|/nav)[^>]*>`)
	headingRe          = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]\s*>`)
	linkRe             = regexp.MustCompile(`(?is)<a\s[^>]*href\s*=\s*"([^"]+)"[^>]*>(.*?)</a\s*>`)
	imageRe            = regexp.MustCompile(`(?is)<img\s[^>]*src\s*=\s*"([^"]+)"[^>]*alt\s*=\s*"([^"]*)"`)
	imageAltRe         = regexp.MustCompile(`(?is)<img\s[^>]*alt\s*=\s*"([^"]*)"[^>]*src\s*=\s*"([^"]+)"`)
	strongRe           = regexp.MustCompile(`(?is)</?(?:strong|b)>`)
	emRe               = regexp.MustCompile(`(?is)</?(?:em|i)>`)
	codeRe             = regexp.MustCompile(`(?is)</?code>`)
	preRe              = regexp.MustCompile(`(?is)</?pre>`)
	hrRe               = regexp.MustCompile(`(?is)<hr[^>]*>`)
	listItemRe         = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li\s*>`)
	unorderedListRe    = regexp.MustCompile(`(?is)</?ul[^>]*>`)
	orderedListRe      = regexp.MustCompile(`(?is)</?ol[^>]*>`)
	brRe               = regexp.MustCompile(`(?is)<br[^>]*>`)
	whitespaceRe       = regexp.MustCompile(`\s+`)
	metaDescRe         = regexp.MustCompile(`(?is)<meta\s[^>]*?(?:name|property)\s*=\s*["'](?:description|og:description)["'][^>]*?content\s*=\s*["']([^"']+)["']`)
	resourceImgRe      = regexp.MustCompile(`(?is)<img\s[^>]*src\s*=\s*"([^"]+)"`)
	resourceScriptRe   = regexp.MustCompile(`(?is)<script\s[^>]*src\s*=\s*"([^"]+)"`)
	resourceLinkRe     = regexp.MustCompile(`(?is)<link\s[^>]*href\s*=\s*"([^"]+)"`)
	resourceCSSRe      = regexp.MustCompile(`(?is)<link\s[^>]*rel\s*=\s*"stylesheet"[^>]*href\s*=\s*"([^"]+)"`)
	resourceIframeRe   = regexp.MustCompile(`(?is)<iframe\s[^>]*src\s*=\s*"([^"]+)"`)
	resourceVideoRe    = regexp.MustCompile(`(?is)<video\s[^>]*src\s*=\s*"([^"]+)"`)
	resourceSourceRe   = regexp.MustCompile(`(?is)<source\s[^>]*src\s*=\s*"([^"]+)"`)
	resourceObjectRe   = regexp.MustCompile(`(?is)<object\s[^>]*data\s*=\s*"([^"]+)"`)
	noiseElements      = []string{"script", "style", "noscript", "template", "svg", "iframe"}
	boilerplatePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?is)(?:skip\s+to\s+(?:main\s+)?content|skip\s+navigation|accessibility\s+skip)`),
		regexp.MustCompile(`(?is)(?:cookie|privacy|gdpr|cmp)\s*(?:notice|banner|consent|settings|policy|preferences)`),
		regexp.MustCompile(`(?is)(?:accept\s+(?:all\s+)?cookies|reject\s+(?:all\s+)?cookies|cookie\s+settings|cookie\s+preferences)`),
		regexp.MustCompile(`(?is)(?:subscribe|newsletter|sign\s*up|join\s+(?:our\s+)?(?:mailing\s+list|newsletter))`),
		regexp.MustCompile(`(?is)(?:advertisement|sponsored|promoted|ad\s*choices|google\s+ads)`),
		regexp.MustCompile(`(?is)(?:share\s+(?:this\s+)?(?:article|page|story)|follow\s+us\s+on|social\s+media)`),
		regexp.MustCompile(`(?is)(?:comments?\s+(?:are\s+)?closed|leave\s+a\s+(?:reply|comment)|respond\s+to)`),
		regexp.MustCompile(`(?i)^(?:related\s+(?:articles?|posts?|stories?|content)|you\s+may\s+(?:also\s+)?like|recommended\s+for\s+you)`),
		regexp.MustCompile(`(?is)(?:back\s+to\s+(?:top|navigation)|scroll\s+(?:up|down)|load\s+more)`),
		regexp.MustCompile(`(?i)^\s*$\n`),
	}
)

func htmlToText(s string, mode string) string {
	if mode == "smart" {
		if text, _ := extractReadable(s, ""); text != "" && len(text) >= 500 {
			text = filterTextLines(text)
			for _, bp := range boilerplatePatterns {
				text = bp.ReplaceAllString(text, "")
			}
			text = whitespaceRe.ReplaceAllString(text, " ")
			text = strings.TrimSpace(text)
			if len(text) < 80 {
				if d := extractMetaDescription(s); d != "" {
					return d
				}
			}
			return text
		}
		// fall through to full mode if readability fails
	}

	// full mode (and smart fallback)
	original := s
	s = removeNoiseBlocks(s)
	s = newlineRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = whitespaceRe.ReplaceAllString(s, " ")
	result := strings.TrimSpace(s)

	if mode == "smart" && len(result) < 80 {
		if d := extractMetaDescription(original); d != "" {
			return d
		}
	}

	return result
}

func extractMetaDescription(s string) string {
	m := metaDescRe.FindStringSubmatch(s)
	if len(m) >= 2 {
		return html.UnescapeString(strings.TrimSpace(m[1]))
	}
	return ""
}

func removeNoiseBlocks(s string) string {
	var out strings.Builder
	out.Grow(len(s))

	for {
		low := strings.ToLower(s)
		bestIdx := -1
		var bestTag string

		for _, tag := range noiseElements {
			i := strings.Index(low, "<"+tag)
			if i < 0 {
				continue
			}
			after := i + 1 + len(tag)
			if after >= len(low) {
				continue
			}
			if !strings.ContainsRune(" >/\n\r\t", rune(low[after])) {
				continue // not a word boundary — e.g. "<scriptx>"
			}
			if bestIdx < 0 || i < bestIdx {
				bestIdx = i
				bestTag = tag
			}
		}

		if bestIdx < 0 {
			out.WriteString(s)
			break
		}

		out.WriteString(s[:bestIdx])

		ci := strings.Index(s[bestIdx:], ">")
		if ci < 0 {
			break
		}

		endTag := "</" + bestTag + ">"
		remaining := s[bestIdx+ci+1:]
		ei := strings.Index(strings.ToLower(remaining), endTag)
		if ei < 0 {
			break
		}

		s = remaining[ei+len(endTag):]
	}

	return out.String()
}

func filterTextLines(s string) string {
	lines := strings.Split(s, "\n")
	var out strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip lines where <30% of runes are letters (CSS class junk, path data, etc.)
		runes, letters := 0, 0
		for _, r := range line {
			runes++
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if runes == 0 || float64(letters)/float64(runes) < 0.3 {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(line)
	}
	return out.String()
}

func htmlToMarkdown(s string, mode string) string {
	if mode == "smart" {
		if text, html := extractReadable(s, ""); text != "" && len(text) >= 500 {
			if html != "" {
				// Convert extracted HTML to markdown without re-extraction
				md := markdownFromHTML(html)
				if md != "" {
					return md
				}
			}
			return text
		}
		// fall through to full mode
	}

	original := s
	smart := false

	s = removeNoiseBlocks(s)

	// Convert headings: <h1>text</h1> → # text
	s = headingRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := headingRe.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		level := parts[1]
		content := strings.TrimSpace(stripTags(parts[2]))
		prefix := strings.Repeat("#", int(level[0]-'0'))
		return "\n\n" + prefix + " " + content + "\n\n"
	})

	// Convert links: <a href="url">text</a> → [text](url)
	s = linkRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := linkRe.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		href := parts[1]
		text := strings.TrimSpace(stripTags(parts[2]))
		if text == "" {
			text = href
		}
		return "[" + text + "](" + href + ")"
	})

	// Convert images with alt: <img src="url" alt="text"> → ![text](url)
	s = imageRe.ReplaceAllString(s, "![$2]($1)")
	s = imageAltRe.ReplaceAllString(s, "![$1]($2)")

	// Convert <strong>/<b> → **text**
	s = strongRe.ReplaceAllString(s, "**")

	// Convert <em>/<i> → *text*
	s = emRe.ReplaceAllString(s, "*")

	// Convert <code> → `text`
	s = codeRe.ReplaceAllString(s, "`")

	// Convert <hr> → ---
	s = hrRe.ReplaceAllString(s, "\n\n---\n\n")

	// Convert <br> → newline
	s = brRe.ReplaceAllString(s, "\n")

	// Convert list items: <li>text</li> → - text
	s = listItemRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := listItemRe.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		content := strings.TrimSpace(stripTags(parts[1]))
		return "\n- " + content
	})

	// Strip <ul>/<ol> wrappers
	s = unorderedListRe.ReplaceAllString(s, "\n")
	s = orderedListRe.ReplaceAllString(s, "\n")

	// Block-level tags → newlines
	s = newlineRe.ReplaceAllString(s, "\n")

	// Strip remaining HTML tags
	s = tagRe.ReplaceAllString(s, "")

	s = html.UnescapeString(s)

	if smart {
		s = filterTextLines(s)
		for _, bp := range boilerplatePatterns {
			s = bp.ReplaceAllString(s, "")
		}
	}

	s = whitespaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	if smart && len(s) < 80 {
		if d := extractMetaDescription(original); d != "" {
			return d
		}
	}

	return s
}

func stripTags(s string) string {
	return strings.TrimSpace(tagRe.ReplaceAllString(s, ""))
}

// markdownFromHTML converts clean article HTML to Markdown.
// Assumes input is already extracted/noise-free (e.g. from go-readability).
func markdownFromHTML(s string) string {
	s = headingRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := headingRe.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		level := parts[1]
		content := strings.TrimSpace(stripTags(parts[2]))
		prefix := strings.Repeat("#", int(level[0]-'0'))
		return "\n\n" + prefix + " " + content + "\n\n"
	})

	s = linkRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := linkRe.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		href := parts[1]
		text := strings.TrimSpace(stripTags(parts[2]))
		if text == "" {
			text = href
		}
		return "[" + text + "](" + href + ")"
	})

	s = imageRe.ReplaceAllString(s, "![$2]($1)")
	s = imageAltRe.ReplaceAllString(s, "![$1]($2)")
	s = strongRe.ReplaceAllString(s, "**")
	s = emRe.ReplaceAllString(s, "*")
	s = codeRe.ReplaceAllString(s, "`")
	s = hrRe.ReplaceAllString(s, "\n\n---\n\n")
	s = brRe.ReplaceAllString(s, "\n")

	s = listItemRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := listItemRe.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		content := strings.TrimSpace(stripTags(parts[1]))
		return "\n- " + content
	})

	s = unorderedListRe.ReplaceAllString(s, "\n")
	s = orderedListRe.ReplaceAllString(s, "\n")
	s = newlineRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = whitespaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func extractExternalResources(rawHTML, pageURL string) []ExternalResource {
	type seenKey struct {
		typ string
		url string
	}
	seen := map[seenKey]bool{}
	var resources []ExternalResource

	addResource := func(typ, rawURL string) {
		absURL, err := resolveURL(pageURL, rawURL)
		if err != nil || absURL == "" || strings.HasPrefix(absURL, "data:") {
			return
		}
		key := seenKey{typ, absURL}
		if seen[key] {
			return
		}
		seen[key] = true
		resources = append(resources, ExternalResource{Type: typ, URL: absURL})
	}

	// images
	for _, m := range resourceImgRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("image", m[1])
		}
	}

	// scripts
	for _, m := range resourceScriptRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("script", m[1])
		}
	}

	// stylesheets
	for _, m := range resourceCSSRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("stylesheet", m[1])
		}
	}

	// favicon + other link types
	for _, m := range resourceLinkRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("link", m[1])
		}
	}

	// iframes
	for _, m := range resourceIframeRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("iframe", m[1])
		}
	}

	// videos
	for _, m := range resourceVideoRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("video", m[1])
		}
	}

	// source elements (audio/video/picture)
	for _, m := range resourceSourceRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("source", m[1])
		}
	}

	// object data
	for _, m := range resourceObjectRe.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) >= 2 {
			addResource("object", m[1])
		}
	}

	return resources
}

func resolveURL(base, raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return baseURL.ResolveReference(ref).String(), nil
}

func fetchLLMSTXT(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	llmsURL := u.Scheme + "://" + u.Host + "/llms.txt"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(llmsURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return ""
	}

	ct := resp.Header.Get("content-type")
	if !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "text/markdown") && !strings.HasPrefix(ct, "text/") {
		return ""
	}

	return string(body)
}
