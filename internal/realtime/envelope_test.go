package realtime

import "testing"

func TestBuildEnvelopeAddsStrongCursorMetadata(t *testing.T) {
	payload := map[string]any{"message_id": "m-1"}
	envelope, metadata := BuildEnvelope(EnvelopeInput{
		Channel: " conversations ",
		Event:   " conversation.message ",
		Topic:   " conversation.message ",
		Payload: payload,
		Cursor:  7,
	})

	if envelope["consistency"] != "strong" || envelope["cursor"] != int64(7) || envelope["scope_key"] != "conversations:conversation.message" || envelope["scope_topic"] != "conversation.message" {
		t.Fatalf("envelope = %#v", envelope)
	}
	if metadata.ScopeKey != "conversations:conversation.message" || metadata.ScopeTopic != "conversation.message" || metadata.Cursor != 7 || metadata.Consistency != "strong" {
		t.Fatalf("metadata = %#v", metadata)
	}
	envelopePayload := envelope["payload"].(map[string]any)
	envelopePayload["message_id"] = "mutated"
	if payload["message_id"] != "m-1" {
		t.Fatalf("source payload mutated: %#v", payload)
	}
}

func TestBuildEnvelopeDoesNotDoublePrefixFullScope(t *testing.T) {
	envelope, metadata := BuildEnvelope(EnvelopeInput{
		Channel: "conversations",
		Event:   "conversation.message",
		Topic:   "conversations:conversation.message",
		Cursor:  9,
	})

	if envelope["scope_key"] != "conversations:conversation.message" {
		t.Fatalf("scope_key = %#v", envelope["scope_key"])
	}
	if metadata.ScopeKey != "conversations:conversation.message" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestBuildEnvelopeMirrorsPythonStrongConsistencyEventSet(t *testing.T) {
	tests := []struct {
		name       string
		channel    string
		event      string
		topic      string
		wantStrong bool
		wantScope  string
	}{
		{
			name:       "message_created alias is replayable",
			channel:    "conversations",
			event:      "message_created",
			wantStrong: true,
			wantScope:  "conversations:message_created",
		},
		{
			name:       "conversation assignment alias is replayable",
			channel:    "conversations",
			event:      "conversation_assigned",
			wantStrong: true,
			wantScope:  "conversations:conversation_assigned",
		},
		{
			name:       "conversation assignment topic is replayable",
			channel:    "conversations",
			event:      "conversation.transferred",
			topic:      "conversation.assignment",
			wantStrong: true,
			wantScope:  "conversations:conversation.assignment",
		},
		{
			name:       "media ready is replayable",
			channel:    "conversations",
			event:      "conversation.media_ready",
			topic:      "conversation.media_ready",
			wantStrong: true,
			wantScope:  "conversations:conversation.media_ready",
		},
		{
			name:       "task status stays weak like Python",
			channel:    "tasks",
			event:      "task.status",
			topic:      "task.status",
			wantStrong: false,
			wantScope:  "tasks:task.status",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope, metadata := BuildEnvelope(EnvelopeInput{
				Channel: test.channel,
				Event:   test.event,
				Topic:   test.topic,
				Payload: map[string]any{"conversation_id": "conv-1"},
				Cursor:  42,
			})

			if metadata.ScopeKey != test.wantScope {
				t.Fatalf("scope key = %q, want %q", metadata.ScopeKey, test.wantScope)
			}
			if test.wantStrong {
				if envelope["consistency"] != "strong" || envelope["cursor"] != int64(42) || metadata.Cursor != 42 || metadata.Consistency != "strong" {
					t.Fatalf("strong envelope mismatch envelope=%#v metadata=%#v", envelope, metadata)
				}
				return
			}
			if envelope["consistency"] != "weak" || metadata.Cursor != 0 || metadata.Consistency != "weak" {
				t.Fatalf("weak envelope mismatch envelope=%#v metadata=%#v", envelope, metadata)
			}
			if _, ok := envelope["cursor"]; ok {
				t.Fatalf("weak envelope included cursor: %#v", envelope)
			}
		})
	}
}

func TestBuildEnvelopeKeepsWeakEventsWeak(t *testing.T) {
	envelope, metadata := BuildEnvelope(EnvelopeInput{
		Channel: "conversations",
		Event:   "typing",
		Topic:   "typing",
		Cursor:  12,
	})

	if envelope["consistency"] != "weak" {
		t.Fatalf("envelope = %#v", envelope)
	}
	if _, ok := envelope["cursor"]; ok {
		t.Fatalf("weak envelope included cursor: %#v", envelope)
	}
	if metadata.Cursor != 0 || metadata.Consistency != "weak" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
