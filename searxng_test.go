package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearXNGClient_Search_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("q") != "test query" {
			t.Errorf("expected q=test query, got %s", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("expected format=json")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "99")
		json.NewEncoder(w).Encode(SearXNGResponse{
			Query:           "test query",
			NumberOfResults: 42,
			Results: []SearXNGResult{
				{Title: "Result A", URL: "https://a.example", Content: "snippet a", Engine: "google", Score: 0.95, Category: "general"},
				{Title: "Result B", URL: "https://b.example", Content: "snippet b", Engine: "bing", Score: 0.80, Category: "general"},
			},
			Answers:             []any{"answer 1"},
			Corrections:         []string{"did you mean X"},
			Suggestions:         []string{"suggestion 1"},
			UnresponsiveEngines: []any{[]any{"engine1", "timeout"}},
		})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	resp, headers, err := client.Search(SearchParams{Query: "test query"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Query != "test query" {
		t.Errorf("query = %q, want %q", resp.Query, "test query")
	}
	if resp.NumberOfResults != 42 {
		t.Errorf("number_of_results = %d, want 42", resp.NumberOfResults)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(resp.Results))
	}
	if resp.Results[0].Title != "Result A" {
		t.Errorf("title = %q, want %q", resp.Results[0].Title, "Result A")
	}
	if len(resp.Answers) != 1 || resp.Answers[0] != "answer 1" {
		t.Errorf("answers = %v", resp.Answers)
	}
	sc, ok := headers["status_code"].(int)
	if !ok {
		t.Errorf("status_code type = %T, want int", headers["status_code"])
	} else if sc != 200 {
		t.Errorf("status_code = %d, want 200", sc)
	}
	if headers["x-ratelimit-remaining"] != "99" {
		t.Errorf("x-ratelimit-remaining = %v", headers["x-ratelimit-remaining"])
	}
}

func TestSearXNGClient_Search_ResultLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SearXNGResponse{
			Results: []SearXNGResult{
				{Title: "A"}, {Title: "B"}, {Title: "C"},
				{Title: "D"}, {Title: "E"},
			},
		})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	resp, _, err := client.Search(SearchParams{Query: "x", MaxResults: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("got %d results, want 2", len(resp.Results))
	}
}

func TestSearXNGClient_Search_ConnectionRefused(t *testing.T) {
	client := NewSearXNGClient("http://127.0.0.1:1", 1)
	_, _, err := client.Search(SearchParams{Query: "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSearXNGClient_Search_NonOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`format not enabled`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	_, _, err := client.Search(SearchParams{Query: "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSearXNGClient_Search_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	_, _, err := client.Search(SearchParams{Query: "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSearXNGClient_Search_Params(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("categories") != "news" {
			t.Errorf("categories = %q", q.Get("categories"))
		}
		if q.Get("language") != "de" {
			t.Errorf("language = %q", q.Get("language"))
		}
		if q.Get("pageno") != "2" {
			t.Errorf("pageno = %q", q.Get("pageno"))
		}
		if q.Get("time_range") != "week" {
			t.Errorf("time_range = %q", q.Get("time_range"))
		}
		if q.Get("safesearch") != "1" {
			t.Errorf("safesearch = %q", q.Get("safesearch"))
		}
		json.NewEncoder(w).Encode(SearXNGResponse{})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSearXNGClient(ts.URL, 5)
	_, _, err := client.Search(SearchParams{
		Query:      "x",
		Categories: "news",
		Language:   "de",
		PageNo:     2,
		TimeRange:  "week",
		Safesearch: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
