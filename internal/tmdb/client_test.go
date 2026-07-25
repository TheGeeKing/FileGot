package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientSearchAndAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := request.URL.Query().Get("year"); got != "2024" {
			t.Fatalf("year = %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[{"id":1,"title":"Dune: Part Two","original_title":"Dune: Part Two","release_date":"2024-02-27"}]}`))
	}))
	defer server.Close()

	client := NewWithHTTPClient("secret", server.URL, server.Client())
	results, err := client.SearchMovies(context.Background(), "Dune Part Two", 2024, "en-US", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != 1 {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestClientErrorAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"status_message":"Invalid token"}`))
	}))
	defer server.Close()

	client := NewWithHTTPClient("bad", server.URL, server.Client())
	_, err := client.Translations(context.Background())
	apiErr, ok := err.(*Error)
	if !ok || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Translations() error = %#v", err)
	}

	blocked := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer blocked.Close()
	client = NewWithHTTPClient("token", blocked.URL, &http.Client{Timeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Translations(ctx); err != context.Canceled {
		t.Fatalf("canceled request error = %v", err)
	}
}

func TestClientRetriesTransientFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) < 2 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = writer.Write([]byte(`["en-US","fr-FR"]`))
	}))
	defer server.Close()

	client := NewWithHTTPClient("token", server.URL, server.Client())
	languages, err := client.Translations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(languages) != 2 || requests.Load() != 2 {
		t.Fatalf("translations=%v requests=%d", languages, requests.Load())
	}
}
