package connector_test

import (
	"testing"
	"time"

	"im-go/internal/connector"
	"im-go/internal/incominghandler"
	"im-go/internal/incomingqueue"
)

func TestFakeWebhookConnectorBuildsInboundQueuePayload(t *testing.T) {
	now := time.Date(2026, 7, 4, 9, 30, 0, 0, time.UTC)
	webhook := connector.FakeWebhookConnector{
		ConnectorID: "internal-webhook",
		Channel:     connector.ChannelInternalWebhook,
		Now:         func() time.Time { return now },
	}

	payload := webhook.BuildIncomingQueuePayload(connector.InboundEvent{
		EventID:     "evt-1",
		TraceID:     "trace-1",
		TenantID:    "tenant-1",
		AccountID:   "account-1",
		EndpointID:  "endpoint-1",
		MessageID:   int64(123),
		MessageType: connector.MessageTypeText,
		Content:     "hello from internal webhook",
		Sender: connector.ContactIdentity{
			ExternalUserID: "customer-1",
			DisplayName:    "Alice",
			AvatarURL:      "https://example.test/avatar.png",
			Remark:         "VIP",
		},
		Conversation: connector.ConversationBinding{
			ConversationID:         "conv-1",
			ExternalConversationID: "external-conv-1",
			Type:                   "single",
			DisplayName:            "Alice chat",
		},
		Media: []connector.MediaAttachment{{
			AttachmentID: "media-1",
			Type:         connector.MessageTypeImage,
			URL:          "https://example.test/image.png",
			MIMEType:     "image/png",
			Bytes:        42,
		}},
		OccurredAt: now,
		Metadata:   map[string]any{"source": "test"},
	})

	if payload["event_type"] != incomingqueue.EventTypeConnectorInbound || payload["kind"] != connector.KindConnectorInboundMessage {
		t.Fatalf("payload header = %#v", payload)
	}
	data := payload["data"].(map[string]any)
	if data["connector_id"] != "internal-webhook" || data["channel"] != connector.ChannelInternalWebhook {
		t.Fatalf("connector data = %#v", data)
	}
	if data["message_origin"] != "connector:internal.webhook" || data["conversation_key"] != "internal.webhook:account-1:external-conv-1:customer-1" {
		t.Fatalf("message routing data = %#v", data)
	}
	media := data["media"].([]map[string]any)
	if len(media) != 1 || media[0]["attachment_id"] != "media-1" || media[0]["mime_type"] != "image/png" {
		t.Fatalf("media = %#v", media)
	}
}

func TestFakeWebhookConnectorPayloadFeedsIncomingWriteInput(t *testing.T) {
	now := time.Date(2026, 7, 4, 9, 30, 0, 0, time.UTC)
	webhook := connector.FakeWebhookConnector{Now: func() time.Time { return now }}
	payload := webhook.BuildIncomingQueuePayload(connector.InboundEvent{
		EventID:       "evt-1",
		TenantID:      "tenant-1",
		AccountID:     "account-1",
		ChannelUserID: "channel-account-1",
		MessageType:   connector.MessageTypeText,
		Content:       "hello",
		Sender:        connector.ContactIdentity{ExternalUserID: "customer-1", DisplayName: "Alice"},
		Conversation: connector.ConversationBinding{
			ConversationID: "conv-1",
			Type:           "single",
		},
		OccurredAt: now,
	})

	input := incominghandler.BuildDeviceMessageInput(payload, now)
	if input.Options.IngestSource != incominghandler.IngestSourceConnectorInbound || input.Options.CanonicalSource != incominghandler.CanonicalSourceConnector {
		t.Fatalf("options = %+v", input.Options)
	}
	if input.Message.TenantID != "tenant-1" || input.Message.AccountID != "account-1" || input.Message.ChannelUserID != "channel-account-1" || input.Message.ExternalUserID != "customer-1" {
		t.Fatalf("message identity = %+v", input.Message)
	}
	if input.Message.WeWorkUserID != "channel-account-1" || input.Message.MessageOrigin != "connector:internal.webhook" {
		t.Fatalf("message channel fields = %+v", input.Message)
	}
}
