package tryon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const probeTestKey = "sk-prox55-unit-test-not-a-real-key"

func newHits() *atomic.Int32 {
	hits := &atomic.Int32{}
	return hits
}

func assertProbe(t *testing.T, res ProbeResult, layer, probe string, requests int) {
	t.Helper()
	if res.Layer != layer {
		t.Fatalf("expected layer %q, got %q (message=%q)", layer, res.Layer, res.Message)
	}
	if probe != "" && res.Probe != probe {
		t.Fatalf("expected probe %q, got %q", probe, res.Probe)
	}
	if res.UpstreamRequests != requests {
		t.Fatalf("expected %d upstream requests, got %d", requests, res.UpstreamRequests)
	}
	if res.Message == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestProbeServerPlaceholderKeySkipsUpstream(t *testing.T) {
	hits := newHits()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()

	res := TestConnection(context.Background(), ProbeOptions{
		Mode:          "server",
		Provider:      "doubao",
		BaseURL:       server.URL,
		APIKey:        "__FILL_BY_USER__",
		Model:         "doubao-seedream-test",
		EnvKey:        "ARK_API_KEY",
		AllowGenerate: true,
	})

	assertProbe(t, res, probeLayerConfigMissing, probeKindNone, 0)
	if hits.Load() != 0 {
		t.Fatalf("expected zero upstream requests, mock received %d", hits.Load())
	}
	if !strings.Contains(res.Message, "ARK_API_KEY") || !strings.Contains(res.Message, "占位符") {
		t.Fatalf("unexpected placeholder message: %q", res.Message)
	}
	if res.UpstreamStatus != 0 {
		t.Fatalf("expected upstream status 0, got %d", res.UpstreamStatus)
	}
}

func TestProbeConfigMissingSkipsUpstream(t *testing.T) {
	hits := newHits()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()

	t.Run("empty api key", func(t *testing.T) {
		res := TestConnection(context.Background(), ProbeOptions{
			Mode:             "personal",
			Provider:         "openai",
			BaseURL:          server.URL,
			APIKey:           "",
			Model:            "gpt-image-2",
			ClientRestricted: true,
			AllowGenerate:    true,
		})
		assertProbe(t, res, probeLayerConfigMissing, probeKindNone, 0)
	})

	t.Run("private base url", func(t *testing.T) {
		res := TestConnection(context.Background(), ProbeOptions{
			Mode:             "personal",
			Provider:         "openai",
			BaseURL:          "http://10.0.0.1/v1",
			APIKey:           probeTestKey,
			Model:            "gpt-image-2",
			ClientRestricted: true,
			AllowGenerate:    true,
		})
		assertProbe(t, res, probeLayerConfigMissing, probeKindNone, 0)
		if !strings.Contains(res.Message, "内网") {
			t.Fatalf("expected private network rejection message, got %q", res.Message)
		}
	})

	t.Run("unsupported personal provider", func(t *testing.T) {
		res := TestConnection(context.Background(), ProbeOptions{
			Mode:          "personal",
			Provider:      "ark",
			BaseURL:       server.URL,
			APIKey:        probeTestKey,
			Model:         "doubao-test",
			AllowGenerate: true,
		})
		assertProbe(t, res, probeLayerConfigMissing, probeKindNone, 0)
	})

	if hits.Load() != 0 {
		t.Fatalf("expected zero upstream requests across config cases, mock received %d", hits.Load())
	}
}

func TestProbeOpenAI(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantLayer  string
		wantStatus int
	}{
		{"model present", http.StatusOK, `{"data":[{"id":"gpt-image-2"}]}`, probeLayerOK, http.StatusOK},
		{"model absent", http.StatusOK, `{"data":[{"id":"gpt-image-1"}]}`, probeLayerModel, http.StatusOK},
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"bad key","code":"invalid_api_key"}}`, probeLayerAuth, http.StatusUnauthorized},
		{"forbidden", http.StatusForbidden, `{"error":{"message":"no access"}}`, probeLayerForbidden, http.StatusForbidden},
		{"server error", http.StatusInternalServerError, `{"error":{"message":"boom"}}`, probeLayerUpstreamError, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := newHits()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.URL.Path != "/models" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer "+probeTestKey {
					t.Errorf("unexpected authorization header")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			res := TestConnection(context.Background(), ProbeOptions{
				Mode:          "server",
				Provider:      "openai",
				BaseURL:       server.URL,
				APIKey:        probeTestKey,
				Model:         "gpt-image-2",
				AllowGenerate: true,
			})
			assertProbe(t, res, tc.wantLayer, probeKindReadonly, 1)
			if res.UpstreamStatus != tc.wantStatus {
				t.Fatalf("expected upstream status %d, got %d", tc.wantStatus, res.UpstreamStatus)
			}
			if hits.Load() != 1 {
				t.Fatalf("expected exactly one upstream request, got %d", hits.Load())
			}
		})
	}
}

func TestProbeDoubaoGenerateFallback(t *testing.T) {
	cases := []struct {
		name         string
		genStatus    int
		genBody      string
		wantLayer    string
		wantRequests int
		wantStatus   int
		wantProbe    string
	}{
		{
			name:         "generate ok",
			genStatus:    http.StatusOK,
			genBody:      `{"data":[{"url":"https://example.invalid/result.png"}]}`,
			wantLayer:    probeLayerOK,
			wantRequests: 2,
			wantStatus:   http.StatusOK,
			wantProbe:    probeKindGenerate,
		},
		{
			name:         "generate unauthorized",
			genStatus:    http.StatusUnauthorized,
			genBody:      `{"error":{"message":"bad key"}}`,
			wantLayer:    probeLayerAuth,
			wantRequests: 2,
			wantStatus:   http.StatusUnauthorized,
			wantProbe:    probeKindGenerate,
		},
		{
			name:         "generate model missing",
			genStatus:    http.StatusNotFound,
			genBody:      `{"error":{"message":"not found"}}`,
			wantLayer:    probeLayerModel,
			wantRequests: 2,
			wantStatus:   http.StatusNotFound,
			wantProbe:    probeKindGenerate,
		},
		{
			name:         "generate 400 model not found",
			genStatus:    http.StatusBadRequest,
			genBody:      `{"error":{"code":"ModelNotFound","message":"the model is not activated"}}`,
			wantLayer:    probeLayerModel,
			wantRequests: 2,
			wantStatus:   http.StatusBadRequest,
			wantProbe:    probeKindGenerate,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := newHits()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				switch r.URL.Path {
				case "/models":
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"error":{"message":"not found"}}`)
				case "/images/generations":
					if r.Method != http.MethodPost {
						t.Errorf("expected POST for generate, got %s", r.Method)
					}
					var payload probeGenerateRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode generate payload: %v", err)
					}
					if payload.Model != "doubao-seedream-test" || payload.Prompt != "ping" || payload.Size != "1024x1024" || payload.ResponseFormat != "url" || payload.Watermark {
						t.Errorf("unexpected generate payload: %+v", payload)
					}
					w.WriteHeader(tc.genStatus)
					_, _ = io.WriteString(w, tc.genBody)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()

			res := TestConnection(context.Background(), ProbeOptions{
				Mode:          "server",
				Provider:      "doubao",
				BaseURL:       server.URL,
				APIKey:        probeTestKey,
				Model:         "doubao-seedream-test",
				AllowGenerate: true,
			})
			assertProbe(t, res, tc.wantLayer, tc.wantProbe, tc.wantRequests)
			if res.UpstreamStatus != tc.wantStatus {
				t.Fatalf("expected upstream status %d, got %d", tc.wantStatus, res.UpstreamStatus)
			}
			if hits.Load() != 2 {
				t.Fatalf("expected exactly two upstream requests, got %d", hits.Load())
			}
		})
	}
}

func TestProbeDoubaoReadonlyModelAbsent(t *testing.T) {
	hits := newHits()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"another-model"}]}`)
	}))
	defer server.Close()

	res := TestConnection(context.Background(), ProbeOptions{
		Mode:          "server",
		Provider:      "doubao",
		BaseURL:       server.URL,
		APIKey:        probeTestKey,
		Model:         "doubao-seedream-test",
		AllowGenerate: true,
	})
	assertProbe(t, res, probeLayerModel, probeKindReadonly, 1)
	if hits.Load() != 1 {
		t.Fatalf("expected only the readonly request, got %d", hits.Load())
	}
}

func TestProbeDoubaoGenerateGateBlocksNonLocalCaller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/images/generations" {
			t.Error("generate probe must not be sent when the safety gate blocks it")
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	res := TestConnection(context.Background(), ProbeOptions{
		Mode:          "server",
		Provider:      "doubao",
		BaseURL:       server.URL,
		APIKey:        probeTestKey,
		Model:         "doubao-seedream-test",
		AllowGenerate: false,
	})
	assertProbe(t, res, probeLayerForbidden, probeKindGenerate, 0)
	if res.UpstreamStatus != 0 {
		t.Fatalf("expected upstream status 0, got %d", res.UpstreamStatus)
	}
}

func TestProbeGemini(t *testing.T) {
	t.Run("model available", func(t *testing.T) {
		hits := newHits()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			if r.URL.Query().Get("key") != probeTestKey {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"status":"INVALID_ARGUMENT"}}`)
				return
			}
			switch r.URL.Path {
			case "/models":
				_, _ = io.WriteString(w, `{"models":[{"name":"models/gemini-test"}]}`)
			case "/models/gemini-test":
				_, _ = io.WriteString(w, `{"name":"models/gemini-test"}`)
			default:
				t.Errorf("unexpected path %s", r.URL.Path)
			}
		}))
		defer server.Close()

		res := TestConnection(context.Background(), ProbeOptions{
			Mode:          "server",
			Provider:      "gemini",
			BaseURL:       server.URL,
			APIKey:        probeTestKey,
			Model:         "gemini-test",
			AllowGenerate: true,
		})
		assertProbe(t, res, probeLayerOK, probeKindReadonly, 2)
		if hits.Load() != 2 {
			t.Fatalf("expected two upstream requests, got %d", hits.Load())
		}
	})

	t.Run("model missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/models" {
				_, _ = io.WriteString(w, `{"models":[]}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"model not found"}}`)
		}))
		defer server.Close()

		res := TestConnection(context.Background(), ProbeOptions{
			Mode:          "server",
			Provider:      "gemini",
			BaseURL:       server.URL,
			APIKey:        probeTestKey,
			Model:         "gemini-missing",
			AllowGenerate: true,
		})
		assertProbe(t, res, probeLayerModel, probeKindReadonly, 2)
	})

	t.Run("invalid api key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":400,"status":"API_KEY_INVALID","message":"API key not valid"}}`)
		}))
		defer server.Close()

		res := TestConnection(context.Background(), ProbeOptions{
			Mode:          "server",
			Provider:      "gemini",
			BaseURL:       server.URL,
			APIKey:        probeTestKey,
			Model:         "gemini-test",
			AllowGenerate: true,
		})
		assertProbe(t, res, probeLayerAuth, probeKindReadonly, 1)
		if res.ErrorCode != "API_KEY_INVALID" {
			t.Fatalf("expected API_KEY_INVALID error code, got %q", res.ErrorCode)
		}
	})
}

func TestProbeTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()

	res := TestConnection(context.Background(), ProbeOptions{
		Mode:          "server",
		Provider:      "openai",
		BaseURL:       server.URL,
		APIKey:        probeTestKey,
		Model:         "gpt-image-2",
		AllowGenerate: true,
		Timeout:       30 * time.Millisecond,
	})
	assertProbe(t, res, probeLayerTimeout, probeKindReadonly, 1)
}

func TestProbeUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := server.URL
	server.Close()

	res := TestConnection(context.Background(), ProbeOptions{
		Mode:          "server",
		Provider:      "openai",
		BaseURL:       baseURL,
		APIKey:        probeTestKey,
		Model:         "gpt-image-2",
		AllowGenerate: true,
		Timeout:       2 * time.Second,
	})
	assertProbe(t, res, probeLayerUnreachable, probeKindReadonly, 1)
}

func TestProbeResultDoesNotLeakSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-image-2"}]}`)
		case "/images/generations":
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"ZmFrZQ=="}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	results := []ProbeResult{
		TestConnection(context.Background(), ProbeOptions{
			Mode: "server", Provider: "openai", BaseURL: server.URL,
			APIKey: probeTestKey, Model: "gpt-image-2", AllowGenerate: true,
		}),
		TestConnection(context.Background(), ProbeOptions{
			Mode: "server", Provider: "doubao", BaseURL: server.URL,
			APIKey: probeTestKey, Model: "doubao-test", AllowGenerate: true,
		}),
	}

	for _, res := range results {
		encoded, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), probeTestKey) {
			t.Fatalf("probe result leaked api key: %s", encoded)
		}
		if strings.Contains(string(encoded), server.URL) {
			t.Fatalf("probe result leaked full base url: %s", encoded)
		}
	}
}
