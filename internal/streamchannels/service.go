// Package streamchannels builds the legacy realtime channel catalog payload.
// The candidate is read-only and does not start a WebSocket hub, subscribe to
// Redis, or publish events.
package streamchannels

import "context"

// ConnectionStat mirrors ws_hub.channel_stats() entries.
type ConnectionStat struct {
	Channel         string
	Connections     int
	QueuedMessages  int
	DroppedMessages int
}

// StatsProvider loads live WS connection stats when a Go hub is available.
type StatsProvider interface {
	ChannelStats(ctx context.Context) ([]ConnectionStat, error)
}

// Service builds the /api/v1/stream/channels response.
type Service struct {
	Stats StatsProvider
}

// Channels returns the static channel catalog plus optional connection stats.
func (service Service) Channels(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	connections := []map[string]any{}
	if service.Stats != nil {
		stats, err := service.Stats.ChannelStats(ctx)
		if err != nil {
			return nil, err
		}
		connections = connectionPayload(stats)
	}
	return map[string]any{
		"channels":    channelPayload(),
		"connections": connections,
	}, nil
}

// channelPayload returns the legacy static channel catalog.
func channelPayload() []map[string]any {
	return []map[string]any{
		{
			"name": "devices",
			"topics": []string{
				"device.heartbeat",
				"device.status",
				"account.changed",
				"wework.login",
				"sop.changed",
				"ai.config",
			},
			"description": "设备状态、心跳、账号变更、企微登录与配置变更",
		},
		{
			"name":        "tasks",
			"topics":      []string{"task.status"},
			"description": "任务执行状态变化",
		},
		{
			"name": "conversations",
			"topics": []string{
				"conversation.message",
				"conversation.media_ready",
				"conversation.voice_transcription_ready",
				"friend.added",
				"conversation.assignment",
			},
			"description": "会话消息、好友事件与分配状态变化",
		},
	}
}

// connectionPayload converts hub stats to the legacy response field names.
func connectionPayload(stats []ConnectionStat) []map[string]any {
	output := make([]map[string]any, 0, len(stats))
	for _, item := range stats {
		output = append(output, map[string]any{
			"channel":          item.Channel,
			"connections":      item.Connections,
			"queued_messages":  item.QueuedMessages,
			"dropped_messages": item.DroppedMessages,
		})
	}
	return output
}
