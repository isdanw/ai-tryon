package tryon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiSendsKeyInHeaderNotURL(t *testing.T) {
	const apiKey = "secret-test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "" {
			t.Fatalf("API key must not be sent in URL query, got %q", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != apiKey {
			t.Fatalf("expected API key header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]}`))
	}))
	defer server.Close()

	gen := NewGemini(&ModelConfig{
		Name:    "browser-custom",
		APIKey:  apiKey,
		BaseURL: server.URL,
		Model:   "test-model",
	})
	result, err := gen.Generate(context.Background(), "cGVyc29u", "Z2FybWVudA==", "prompt", "1024x1536")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if result != "aW1hZ2U=" {
		t.Fatalf("unexpected image result: %q", result)
	}
}
