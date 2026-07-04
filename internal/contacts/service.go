// Package contacts builds read-only payloads for cached WeWork contacts.
package contacts

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	// ErrStoreUnavailable means the contact cache store is not configured.
	ErrStoreUnavailable = errors.New("contact sync service unavailable")
	// ErrExternalContactNotFound matches the legacy external-contact 404 detail.
	ErrExternalContactNotFound = errors.New("external contact not found")
	// ErrCorpUserNotFound matches the legacy corp-user 404 detail.
	ErrCorpUserNotFound = errors.New("corp user not found")
)

// Payload is a JSON-compatible response body.
type Payload map[string]any

// Store reads cached WeWork contact rows.
type Store interface {
	GetExternalContact(ctx context.Context, enterpriseID string, externalUserID string) (Payload, bool, error)
	GetCorpUser(ctx context.Context, enterpriseID string, userID string) (Payload, bool, error)
}

// AvatarStorage normalizes avatar references before contact payloads are cached.
type AvatarStorage interface {
	PersistAvatarReference(ctx context.Context, enterpriseID string, sourceKey string, avatarValue string) string
}

// Service owns read-only contact cache lookups.
type Service struct {
	Store                   Store
	ExternalContactWriter   ExternalContactWriter
	ExternalContactGetter   ExternalContactGetter
	ExternalContactIDLister ExternalContactIDLister
	ExternalContactSkipper  ExternalContactRefreshSkipper
	CorpUserWriter          CorpUserWriter
	InternalUserGetter      InternalUserGetter
	InternalUserLister      InternalUserLister
	StaleExternalContacts   StaleExternalContactLister
	StaleCorpUsers          StaleCorpUserLister
	Enterprises             EnterpriseSecretStore
	Identity                IdentityWriter
	Relations               FollowUserRelationReconciler
	AvatarStorage           AvatarStorage
	Now                     func() time.Time
}

// ExternalContactRequest carries GET /api/v1/contacts/external/{external_userid}.
type ExternalContactRequest struct {
	EnterpriseID   string
	ExternalUserID string
}

// CorpUserRequest carries GET /api/v1/contacts/corp-user/{userid}.
type CorpUserRequest struct {
	EnterpriseID string
	UserID       string
}

// ExternalContact returns one cached external contact row.
func (service Service) ExternalContact(ctx context.Context, request ExternalContactRequest) (Payload, error) {
	if service.Store == nil {
		return nil, ErrStoreUnavailable
	}
	payload, ok, err := service.Store.GetExternalContact(ctx, strings.TrimSpace(request.EnterpriseID), strings.TrimSpace(request.ExternalUserID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrExternalContactNotFound
	}
	return payload, nil
}

// CorpUser returns one cached internal corp user row.
func (service Service) CorpUser(ctx context.Context, request CorpUserRequest) (Payload, error) {
	if service.Store == nil {
		return nil, ErrStoreUnavailable
	}
	payload, ok, err := service.Store.GetCorpUser(ctx, strings.TrimSpace(request.EnterpriseID), strings.TrimSpace(request.UserID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrCorpUserNotFound
	}
	return payload, nil
}
