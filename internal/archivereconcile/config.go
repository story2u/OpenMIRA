// Package archivereconcile contains incoming/archive primary-mode rules.
package archivereconcile

import "strings"

const (
	ArchivePrimaryMode   = "archive_primary"
	DevicePrimaryMode    = "device_primary"
	DefaultArchiveSource = "self_decrypt"
)

// Enterprise captures the archive fields needed for incoming reconcile decisions.
type Enterprise struct {
	Enabled             bool
	ArchiveSource       string
	IncomingPrimaryMode string
	ArchivePullURL      string
	CorpSecret          string
}

// Config is the normalized reconcile decision for one enterprise.
type Config struct {
	ArchiveReconcileEnabled bool
	ArchiveSource           string
	PrimaryMode             string
}

// NormalizeIncomingPrimaryMode mirrors Python enterprise_service.normalize_incoming_primary_mode.
func NormalizeIncomingPrimaryMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), DevicePrimaryMode) {
		return DevicePrimaryMode
	}
	return ArchivePrimaryMode
}

// BuildConfig mirrors Python build_archive_reconcile_config.
func BuildConfig(enterprise *Enterprise) Config {
	if enterprise == nil || !enterprise.Enabled {
		return Config{ArchiveSource: DefaultArchiveSource, PrimaryMode: ArchivePrimaryMode}
	}
	source := strings.TrimSpace(enterprise.ArchiveSource)
	if source == "" {
		source = DefaultArchiveSource
	}
	mode := NormalizeIncomingPrimaryMode(enterprise.IncomingPrimaryMode)
	archiveReady := strings.TrimSpace(enterprise.ArchivePullURL) != "" || strings.TrimSpace(enterprise.CorpSecret) != ""
	return Config{
		ArchiveReconcileEnabled: archiveReady,
		ArchiveSource:           source,
		PrimaryMode:             mode,
	}
}

// ShouldDirectIngest reports whether device incoming should be written immediately.
func ShouldDirectIngest(config Config) bool {
	return config.PrimaryMode == DevicePrimaryMode || !config.ArchiveReconcileEnabled
}

// ArchiveSyncReason returns the archive sync trigger reason for the decision.
func ArchiveSyncReason(config Config) string {
	if ShouldDirectIngest(config) {
		return "device_message_received"
	}
	return "archive_primary_device_hint"
}
