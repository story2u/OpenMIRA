package main

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/config"
	"wework-go/internal/infra/sqldb"
)

// TestBuildHandlerRequiresDatabaseForArchiveVoiceRetry keeps manual retry durable.
func TestBuildHandlerRequiresDatabaseForArchiveVoiceRetry(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveVoiceTranscriptionRetryCandidate: true,
		SessionJWTSecret:                        "session-secret",
		SessionJWTIssuer:                        "wework-cloud",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestVoiceTranscriptionConfiguredAcceptsAPITokenOrJWT(t *testing.T) {
	base := config.Config{
		VoiceTranscriptionCozeBaseURL: "https://coze.example/run",
		VoiceTranscriptionWorkflowID:  "workflow-1",
	}
	if voiceTranscriptionConfigured(base) {
		t.Fatal("configured without token = true, want false")
	}
	apiToken := base
	apiToken.VoiceTranscriptionAPIToken = "token"
	if !voiceTranscriptionConfigured(apiToken) {
		t.Fatal("configured with api token = false, want true")
	}
	defaultsWithToken := config.Config{VoiceTranscriptionAPIToken: "token"}
	if !voiceTranscriptionConfigured(defaultsWithToken) {
		t.Fatal("configured with executor defaults and api token = false, want true")
	}
	jwt := base
	jwt.VoiceTranscriptionJWTClientID = "client"
	jwt.VoiceTranscriptionJWTPublicKeyID = "kid"
	jwt.VoiceTranscriptionJWTPrivateKeyPEM = "pem"
	if !voiceTranscriptionConfigured(jwt) {
		t.Fatal("configured with jwt = false, want true")
	}
}
