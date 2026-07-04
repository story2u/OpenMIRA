// Stream channel service tests freeze the Python-compatible catalog shape.
// They also keep the optional stats provider isolated from any live WebSocket
// hub dependency.
package streamchannels

import (
	"context"
	"testing"
)

// TestServiceChannelsBuildsLegacyCatalog verifies channel names and stats shape.
func TestServiceChannelsBuildsLegacyCatalog(t *testing.T) {
	service := Service{Stats: fakeStatsProvider{stats: []ConnectionStat{{
		Channel:         "devices",
		Connections:     2,
		QueuedMessages:  3,
		DroppedMessages: 1,
	}}}}

	payload, err := service.Channels(context.Background())
	if err != nil {
		t.Fatalf("Channels returned error: %v", err)
	}
	channels := payload["channels"].([]map[string]any)
	if len(channels) != 3 || channels[0]["name"] != "devices" || channels[1]["name"] != "tasks" {
		t.Fatalf("unexpected channels payload: %#v", channels)
	}
	connections := payload["connections"].([]map[string]any)
	if len(connections) != 1 || connections[0]["channel"] != "devices" || connections[0]["queued_messages"] != 3 {
		t.Fatalf("unexpected connections payload: %#v", connections)
	}
}

type fakeStatsProvider struct {
	stats []ConnectionStat
}

// ChannelStats returns prebuilt stats for service tests.
func (provider fakeStatsProvider) ChannelStats(ctx context.Context) ([]ConnectionStat, error) {
	return provider.stats, nil
}
