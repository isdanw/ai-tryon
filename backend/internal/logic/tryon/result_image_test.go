package tryon

import (
	"encoding/base64"
	"testing"
)

func TestDetectGeneratedImageMIME(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{name: "png", data: []byte("\x89PNG\r\n\x1a\nrest"), want: "image/png"},
		{name: "jpeg", data: []byte("\xff\xd8\xff\xe0rest"), want: "image/jpeg"},
		{name: "webp", data: []byte("RIFF\x04\x00\x00\x00WEBPrest"), want: "image/webp"},
		{name: "gif rejected", data: []byte("GIF89arest"), wantErr: true},
		{name: "text rejected", data: []byte("plain text"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(tt.data)
			got, err := DetectGeneratedImageMIME(encoded)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("DetectGeneratedImageMIME() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DetectGeneratedImageMIME() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("DetectGeneratedImageMIME() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectGeneratedImageMIMERejectsInvalidBase64(t *testing.T) {
	if _, err := DetectGeneratedImageMIME("not-valid-base64"); err == nil {
		t.Fatal("DetectGeneratedImageMIME() error = nil, want invalid base64 error")
	}
}

func TestDetectGeneratedImageMIMEAcceptsDataURL(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nrest"))
	got, err := DetectGeneratedImageMIME("data:" + "image/png;base64," + png)
	if err != nil {
		t.Fatalf("DetectGeneratedImageMIME() error = %v", err)
	}
	if got != "image/png" {
		t.Fatalf("DetectGeneratedImageMIME() = %q, want image/png", got)
	}
}
