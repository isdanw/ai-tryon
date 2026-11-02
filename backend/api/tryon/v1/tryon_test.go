package v1

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTryonResIncludesImageMIME(t *testing.T) {
	encoded, err := json.Marshal(TryonRes{Image: "image-data", MIME: "image/jpeg", Model: "gemini"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"mime":"image/jpeg"`) {
		t.Fatalf("TryonRes JSON missing mime: %s", encoded)
	}
}
