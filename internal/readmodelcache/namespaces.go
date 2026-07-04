// Package readmodelcache defines the shared conversation read-model cache
// namespaces used by legacy-compatible write candidates.
package readmodelcache

const (
	ConversationListNamespace  = "conversation-list"
	PanelSnapshotNamespace     = "conversation-panel-snapshot"
	AccountStatsNamespace      = "conversation-account-stats"
	CSWorkbenchSearchNamespace = "cs-workbench-search"
)

// AllNamespaces returns a fresh ordered slice for the Python-compatible
// conversation read-model invalidation batch.
func AllNamespaces() []string {
	return []string{
		ConversationListNamespace,
		PanelSnapshotNamespace,
		AccountStatsNamespace,
		CSWorkbenchSearchNamespace,
	}
}
