package archivereconcile

import "testing"

func TestBuildConfigDefaultsToArchivePrimaryWithoutEnterprise(t *testing.T) {
	config := BuildConfig(nil)
	if config.ArchiveReconcileEnabled || config.ArchiveSource != DefaultArchiveSource || config.PrimaryMode != ArchivePrimaryMode {
		t.Fatalf("config = %+v", config)
	}
	if !ShouldDirectIngest(config) || ArchiveSyncReason(config) != "device_message_received" {
		t.Fatalf("decision config = %+v", config)
	}
}

func TestBuildConfigUsesDevicePrimaryWhenArchiveReady(t *testing.T) {
	config := BuildConfig(&Enterprise{
		Enabled:             true,
		ArchiveSource:       " custom ",
		IncomingPrimaryMode: "device_primary",
		ArchivePullURL:      "https://archive.example/pull",
	})
	if !config.ArchiveReconcileEnabled || config.ArchiveSource != "custom" || config.PrimaryMode != DevicePrimaryMode {
		t.Fatalf("config = %+v", config)
	}
	if !ShouldDirectIngest(config) || ArchiveSyncReason(config) != "device_message_received" {
		t.Fatalf("decision config = %+v", config)
	}
}

func TestBuildConfigArchivePrimaryDefersDirectIngestWhenReady(t *testing.T) {
	config := BuildConfig(&Enterprise{
		Enabled:             true,
		IncomingPrimaryMode: "archive_primary",
		CorpSecret:          "secret",
	})
	if !config.ArchiveReconcileEnabled || config.PrimaryMode != ArchivePrimaryMode {
		t.Fatalf("config = %+v", config)
	}
	if ShouldDirectIngest(config) || ArchiveSyncReason(config) != "archive_primary_device_hint" {
		t.Fatalf("decision config = %+v", config)
	}
}

func TestNormalizeIncomingPrimaryModeFallsBackToArchivePrimary(t *testing.T) {
	if NormalizeIncomingPrimaryMode(" DEVICE_PRIMARY ") != DevicePrimaryMode {
		t.Fatal("device primary not normalized")
	}
	if NormalizeIncomingPrimaryMode("bad") != ArchivePrimaryMode || NormalizeIncomingPrimaryMode("") != ArchivePrimaryMode {
		t.Fatal("invalid mode should fall back to archive_primary")
	}
}
