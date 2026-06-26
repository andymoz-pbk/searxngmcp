package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type SearchParams struct {
	Query      string
	Categories string
	Language   string
	PageNo     int
	TimeRange  string
	Safesearch int
	MaxResults int
	Engines    []string
}

type SearXNGResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Engine        string  `json:"engine"`
	Score         float64 `json:"score"`
	Category      string  `json:"category"`
	PublishedDate *string `json:"publishedDate,omitempty"`
}

type SearXNGResponse struct {
	Query               string          `json:"query"`
	NumberOfResults     int             `json:"number_of_results"`
	Results             []SearXNGResult `json:"results"`
	Answers             []string        `json:"answers"`
	Corrections         []string        `json:"corrections"`
	Suggestions         []string        `json:"suggestions"`
	Infoboxes           []any           `json:"infoboxes"`
	UnresponsiveEngines []any           `json:"unresponsive_engines"`
}

type SearXNGClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewSearXNGClient(baseURL string, timeout int) *SearXNGClient {
	return &SearXNGClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (c *SearXNGClient) Search(params SearchParams) (*SearXNGResponse, map[string]any, error) {
	q := url.Values{}
	q.Set("format", "json")
	q.Set("q", params.Query)

	if params.Categories != "" {
		q.Set("categories", params.Categories)
	}
	if params.Language != "" {
		q.Set("language", params.Language)
	}
	if params.PageNo > 0 {
		q.Set("pageno", strconv.Itoa(params.PageNo))
	}
	if params.TimeRange != "" {
		q.Set("time_range", params.TimeRange)
	}
	q.Set("safesearch", strconv.Itoa(params.Safesearch))

	for _, e := range params.Engines {
		q.Add("engines[]", e)
	}

	u := fmt.Sprintf("%s/search?%s", c.baseURL, q.Encode())

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("searxng unreachable: %w", err)
	}
	defer resp.Body.Close()

	respHeaders := map[string]any{
		"status_code": resp.StatusCode,
	}
	for _, h := range []string{"content-type", "x-ratelimit-remaining", "x-ratelimit-reset", "retry-after"} {
		if v := resp.Header.Get(h); v != "" {
			respHeaders[h] = v
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, respHeaders, fmt.Errorf("searxng returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	limited := io.LimitReader(resp.Body, 10*1024*1024)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, respHeaders, fmt.Errorf("reading searxng response: %w", err)
	}

	var searxngResp SearXNGResponse
	if err := json.Unmarshal(raw, &searxngResp); err != nil {
		return nil, respHeaders, fmt.Errorf("decoding searxng response: %w", err)
	}

	if params.MaxResults > 0 && len(searxngResp.Results) > params.MaxResults {
		searxngResp.Results = searxngResp.Results[:params.MaxResults]
	}

	return &searxngResp, respHeaders, nil
}
