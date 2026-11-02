package tryon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testRealInputModerationBody = `{"error":{"code":"InputImageSensitiveContentDetected","message":"The request failed because the input image may contain sensitive information. Request id: 0217935454372124d9266badd4680e9e72735e419b86df37e4f46","param":"","type":"BadRequest"}}`

const testOutputModerationBody = `{"error":{"code":"OutputImageSensitiveContentDetected.PolicyViolation","message":"The request failed because the output image may contain sensitive information. Request id: 0217935454372124d9266badd4680e9e72735e419b86df37e4f46","param":"","type":"BadRequest"}}`

const testPlainBadRequestBody = `{"error":{"code":"InvalidParameter","message":"bad"}}`

func TestParseModerationBlockedInput(t *testing.T) {
	got := parseModerationBlocked([]byte(testRealInputModerationBody))
	if got == nil {
		t.Fatal("parseModerationBlocked returned nil for input moderation body")
	}
	if got.Kind != ModerationBlockedInput {
		t.Fatalf("Kind = %q, want %q", got.Kind, ModerationBlockedInput)
	}
	if got.UpstreamCode != "InputImageSensitiveContentDetected" {
		t.Fatalf("UpstreamCode = %q", got.UpstreamCode)
	}
	const wantRequestID = "0217935454372124d9266badd4680e9e72735e419b86df37e4f46"
	if got.RequestID != wantRequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, wantRequestID)
	}
}

func TestParseModerationBlockedOutputWithSuffix(t *testing.T) {
	got := parseModerationBlocked([]byte(testOutputModerationBody))
	if got == nil {
		t.Fatal("parseModerationBlocked returned nil for output moderation body")
	}
	if got.Kind != ModerationBlockedOutput {
		t.Fatalf("Kind = %q, want %q", got.Kind, ModerationBlockedOutput)
	}
	if got.UpstreamCode != "OutputImageSensitiveContentDetected.PolicyViolation" {
		t.Fatalf("UpstreamCode = %q", got.UpstreamCode)
	}
}

func TestParseModerationBlockedIgnoresOtherErrors(t *testing.T) {
	if got := parseModerationBlocked([]byte(testPlainBadRequestBody)); got != nil {
		t.Fatalf("expected nil for non-moderation body, got %#v", got)
	}
	if got := parseModerationBlocked([]byte(`not json`)); got != nil {
		t.Fatalf("expected nil for malformed body, got %#v", got)
	}
}

func TestWrapGenerateErrorKeepsModerationBlocked(t *testing.T) {
	modErr := &ModerationBlockedError{Kind: ModerationBlockedInput, UpstreamCode: "InputImageSensitiveContentDetected"}
	wrapped := wrapGenerateError(modErr)
	if wrapped != error(modErr) {
		t.Fatalf("expected original moderation error, got %v", wrapped)
	}
	got, ok := AsModerationBlocked(wrapped)
	if !ok || got.Kind != ModerationBlockedInput {
		t.Fatalf("expected AsModerationBlocked to match, got %#v ok=%v", got, ok)
	}
	if strings.Contains(wrapped.Error(), "换装生成失败") {
		t.Fatalf("moderation error should not contain generic wrap text: %v", wrapped)
	}
}

func TestWrapGenerateErrorWrapsPlainError(t *testing.T) {
	wrapped := wrapGenerateError(errors.New("boom"))
	if !strings.Contains(wrapped.Error(), "换装生成失败，请稍后重试") {
		t.Fatalf("expected generic wrap text, got %v", wrapped)
	}
	if _, ok := AsModerationBlocked(wrapped); ok {
		t.Fatalf("plain error should not match AsModerationBlocked")
	}
}

func newModerationStub(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newStubDoubao(srv *httptest.Server) *Doubao {
	return NewDoubao(&ModelConfig{
		Name:           "doubao",
		Provider:       "doubao",
		APIKey:         "fake-key",
		BaseURL:        srv.URL,
		Model:          "seedream-test",
		ClientProvided: false,
	})
}

func TestDoubaoGenerateModerationBlocked(t *testing.T) {
	srv := newModerationStub(t, http.StatusBadRequest, testRealInputModerationBody)
	_, err := newStubDoubao(srv).Generate(context.Background(), "aGVsbG8=", "d29ybGQ=", "prompt", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	modErr, ok := AsModerationBlocked(err)
	if !ok {
		t.Fatalf("expected moderation blocked error, got %v", err)
	}
	if modErr.Kind != ModerationBlockedInput {
		t.Fatalf("Kind = %q, want %q", modErr.Kind, ModerationBlockedInput)
	}
	if modErr.RequestID != "0217935454372124d9266badd4680e9e72735e419b86df37e4f46" {
		t.Fatalf("RequestID = %q", modErr.RequestID)
	}
}

func TestDoubaoGeneratePlainBadRequestNotModeration(t *testing.T) {
	srv := newModerationStub(t, http.StatusBadRequest, testPlainBadRequestBody)
	_, err := newStubDoubao(srv).Generate(context.Background(), "aGVsbG8=", "d29ybGQ=", "prompt", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := AsModerationBlocked(err); ok {
		t.Fatalf("plain 400 must not be treated as moderation: %v", err)
	}
	if !strings.Contains(err.Error(), "豆包 API 错误") {
		t.Fatalf("expected generic doubao error text, got %v", err)
	}
}
