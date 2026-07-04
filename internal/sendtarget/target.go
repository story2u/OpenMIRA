// Package sendtarget defines the shared contact target resolution boundary.
package sendtarget

import (
	"context"
	"strings"
)

// Resolver resolves conversation-bound send targets before task creation.
type Resolver interface {
	ResolveSendTarget(ctx context.Context, request Request) (Target, error)
}

// Request carries the target resolution inputs from a send payload.
type Request struct {
	ConversationID     string
	DeviceID           string
	FallbackReceiver   string
	FallbackAliases    string
	FallbackSenderName string
	FallbackSenderID   string
	PreferRPASafeName  bool
}

// Target is the normalized recipient context used in SDK tasks.
type Target struct {
	Receiver             string
	Aliases              string
	ConversationID       string
	SenderID             string
	SenderName           string
	ContactProfileUpdate map[string]any
}

// ContactIdentityError maps stale or unavailable contact identity resolution to HTTP 409.
type ContactIdentityError struct {
	Detail string
	Cause  error
}

func (err ContactIdentityError) Error() string {
	if strings.TrimSpace(err.Detail) != "" {
		return strings.TrimSpace(err.Detail)
	}
	return "contact identity is not ready; refresh has been requested, please retry later"
}

func (err ContactIdentityError) Unwrap() error {
	return err.Cause
}
