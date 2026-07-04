// Package archivesdk implements the built-in archive SDK bridge boundary.
package archivesdk

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"wework-go/internal/archiveadmin"
	"wework-go/internal/archivepull"
	"wework-go/internal/infra/enterprisestore"
)

const (
	DefaultEnterpriseID = archiveadmin.DefaultEnterpriseID
	DefaultSource       = archiveadmin.DefaultSource
	DefaultPullLimit    = 200
)

var (
	// ErrEnterpriseNotFound mirrors the legacy FastAPI 404 detail.
	ErrEnterpriseNotFound = errors.New("enterprise not found")
	// ErrCorpCredentialsRequired means neither request nor enterprise config has official credentials.
	ErrCorpCredentialsRequired = errors.New("corp_id and corp_secret are required")
	// ErrSDKFileIDRequired mirrors the legacy media bridge validation detail.
	ErrSDKFileIDRequired = errors.New("sdk_file_id is required")
	// ErrBridgeUnavailable means no local official SDK bridge is wired into the API process.
	ErrBridgeUnavailable = errors.New("official sdk bridge is not configured")
	// ErrEnterpriseStoreUnavailable means enterprise credentials cannot be read.
	ErrEnterpriseStoreUnavailable = errors.New("archive enterprise store is unavailable")
)

// Payload is a JSON-compatible response shape.
type Payload map[string]any

// PullRequest carries POST /api/v1/archive/sdk/pull input.
type PullRequest struct {
	EnterpriseID      string
	Source            string
	Cursor            *string
	Limit             int
	CorpID            string
	CorpSecret        string
	PrivateKeyPEM     string
	PrivateKeyVersion string
}

// MediaPullRequest carries POST /api/v1/archive/sdk/media/pull input.
type MediaPullRequest struct {
	EnterpriseID string
	Source       string
	SDKFileID    string
	IndexBuf     string
	CorpID       string
	CorpSecret   string
}

// EnterpriseStore reads archive SDK credentials from enterprise bindings.
type EnterpriseStore interface {
	GetArchivePullEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.ArchivePullEnterprise, error)
}

// OfficialBridge performs the actual finance SDK calls.
type OfficialBridge interface {
	PullOfficial(ctx context.Context, input OfficialPullInput) (Payload, error)
	PullOfficialMedia(ctx context.Context, input OfficialMediaPullInput) (Payload, error)
}

// OfficialPullInput is the normalized finance SDK message pull request.
type OfficialPullInput struct {
	EnterpriseID      string
	Source            string
	Cursor            *string
	Limit             int
	CorpID            string
	CorpSecret        string
	PrivateKeyPEM     string
	PrivateKeyVersion string
}

// OfficialMediaPullInput is the normalized finance SDK media pull request.
type OfficialMediaPullInput struct {
	EnterpriseID string
	Source       string
	SDKFileID    string
	IndexBuf     string
	CorpID       string
	CorpSecret   string
}

// Service owns archive SDK bridge request normalization.
type Service struct {
	Enterprises EnterpriseStore
	Bridge      OfficialBridge
}

// Pull invokes the official SDK bridge for message pull.
func (service Service) Pull(ctx context.Context, request PullRequest) (Payload, error) {
	enterpriseID := defaultText(request.EnterpriseID, DefaultEnterpriseID)
	source := defaultText(request.Source, DefaultSource)
	limit := request.Limit
	if limit <= 0 {
		limit = DefaultPullLimit
	}
	limit = archivepull.ClampSDKLimit(limit, archivepull.DefaultSDKLimitMaximum)
	enterprise, err := service.enterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	corpID, corpSecret, err := resolveCredentials(enterprise, request.CorpID, request.CorpSecret)
	if err != nil {
		return nil, err
	}
	if service.Bridge == nil {
		return nil, ErrBridgeUnavailable
	}
	return service.Bridge.PullOfficial(ctx, OfficialPullInput{
		EnterpriseID:      enterpriseID,
		Source:            source,
		Cursor:            request.Cursor,
		Limit:             limit,
		CorpID:            corpID,
		CorpSecret:        corpSecret,
		PrivateKeyPEM:     defaultText(fieldFromEnterprise(enterprise, func(record *enterprisestore.ArchivePullEnterprise) string { return record.PrivateKeyPEM }), request.PrivateKeyPEM),
		PrivateKeyVersion: defaultText(fieldFromEnterprise(enterprise, func(record *enterprisestore.ArchivePullEnterprise) string { return record.PrivateKeyVersion }), request.PrivateKeyVersion),
	})
}

// PullMedia invokes the official SDK bridge for media pull.
func (service Service) PullMedia(ctx context.Context, request MediaPullRequest) (Payload, error) {
	enterpriseID := defaultText(request.EnterpriseID, DefaultEnterpriseID)
	source := defaultText(request.Source, DefaultSource)
	sdkFileID := strings.TrimSpace(request.SDKFileID)
	if sdkFileID == "" {
		return nil, ErrSDKFileIDRequired
	}
	enterprise, err := service.enterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	corpID, corpSecret, err := resolveCredentials(enterprise, request.CorpID, request.CorpSecret)
	if err != nil {
		return nil, err
	}
	if service.Bridge == nil {
		return nil, ErrBridgeUnavailable
	}
	return service.Bridge.PullOfficialMedia(ctx, OfficialMediaPullInput{
		EnterpriseID: enterpriseID,
		Source:       source,
		SDKFileID:    sdkFileID,
		IndexBuf:     strings.TrimSpace(request.IndexBuf),
		CorpID:       corpID,
		CorpSecret:   corpSecret,
	})
}

func (service Service) enterprise(ctx context.Context, enterpriseID string) (*enterprisestore.ArchivePullEnterprise, error) {
	if enterpriseID == DefaultEnterpriseID && service.Enterprises == nil {
		return nil, nil
	}
	if service.Enterprises == nil {
		return nil, ErrEnterpriseStoreUnavailable
	}
	enterprise, err := service.Enterprises.GetArchivePullEnterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	if enterpriseID != DefaultEnterpriseID && enterprise == nil {
		return nil, fmt.Errorf("%w: %s", ErrEnterpriseNotFound, enterpriseID)
	}
	return enterprise, nil
}

func resolveCredentials(enterprise *enterprisestore.ArchivePullEnterprise, requestCorpID string, requestCorpSecret string) (string, string, error) {
	corpID := defaultText(fieldFromEnterprise(enterprise, func(record *enterprisestore.ArchivePullEnterprise) string { return record.CorpID }), requestCorpID)
	corpSecret := defaultText(fieldFromEnterprise(enterprise, func(record *enterprisestore.ArchivePullEnterprise) string { return record.CorpSecret }), requestCorpSecret)
	if corpID == "" || corpSecret == "" {
		return "", "", ErrCorpCredentialsRequired
	}
	return corpID, corpSecret, nil
}

func fieldFromEnterprise(enterprise *enterprisestore.ArchivePullEnterprise, read func(*enterprisestore.ArchivePullEnterprise) string) string {
	if enterprise == nil || read == nil {
		return ""
	}
	return read(enterprise)
}

func defaultText(preferred string, fallback string) string {
	value := strings.TrimSpace(preferred)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}
