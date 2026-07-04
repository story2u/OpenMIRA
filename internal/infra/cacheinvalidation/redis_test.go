package cacheinvalidation

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestInvalidateNamespacesBumpsVersionsAndPublishesBatch(t *testing.T) {
	client := &recordingRedisClient{}
	invalidator := New(client, Options{Prefix: "custom", Channel: "custom:invalidate"})

	err := invalidator.InvalidateNamespaces(context.Background(), " conversation-list ", "", "conversation-panel-snapshot", "conversation-list")
	if err != nil {
		t.Fatalf("InvalidateNamespaces returned error: %v", err)
	}
	wantKeys := []string{"custom:version:conversation-list", "custom:version:conversation-panel-snapshot"}
	if !reflect.DeepEqual(client.incrKeys, wantKeys) {
		t.Fatalf("incr keys = %+v, want %+v", client.incrKeys, wantKeys)
	}
	if client.publishChannel != "custom:invalidate" {
		t.Fatalf("publish channel = %q", client.publishChannel)
	}
	if client.publishMessage != `{"namespaces":["conversation-list","conversation-panel-snapshot"]}` {
		t.Fatalf("publish message = %q", client.publishMessage)
	}
}

func TestInvalidateNamespacesReturnsIncrErrorBeforePublish(t *testing.T) {
	client := &recordingRedisClient{incrErr: errors.New("redis down")}
	invalidator := New(client, Options{})

	err := invalidator.InvalidateNamespaces(context.Background(), "conversation-list")
	if !errors.Is(err, client.incrErr) {
		t.Fatalf("err = %v, want incr error", err)
	}
	if client.publishChannel != "" {
		t.Fatalf("publish should not run on incr error")
	}
}

type recordingRedisClient struct {
	incrKeys       []string
	incrErr        error
	publishChannel string
	publishMessage string
	publishErr     error
}

func (client *recordingRedisClient) Incr(ctx context.Context, key string) *redis.IntCmd {
	client.incrKeys = append(client.incrKeys, key)
	cmd := redis.NewIntCmd(ctx)
	if client.incrErr != nil {
		cmd.SetErr(client.incrErr)
		return cmd
	}
	cmd.SetVal(int64(len(client.incrKeys)))
	return cmd
}

func (client *recordingRedisClient) Publish(ctx context.Context, channel string, message any) *redis.IntCmd {
	client.publishChannel = channel
	client.publishMessage, _ = message.(string)
	cmd := redis.NewIntCmd(ctx)
	if client.publishErr != nil {
		cmd.SetErr(client.publishErr)
		return cmd
	}
	cmd.SetVal(1)
	return cmd
}
