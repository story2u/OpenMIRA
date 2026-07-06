package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIntegrationAPIOverview(t *testing.T) {
	handler := buildHandler(time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		OverviewStats struct {
			ActiveChannels int `json:"activeChannels"`
			TotalChannels  int `json:"totalChannels"`
		} `json:"overviewStats"`
		Channels []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"channels"`
		TrafficSeries []struct {
			Hour string `json:"hour"`
		} `json:"trafficSeries"`
	}
	decodeJSON(t, response.Body.Bytes(), &body)
	if body.OverviewStats.ActiveChannels != 5 || body.OverviewStats.TotalChannels != 6 {
		t.Fatalf("overview stats = %+v, want active=5 total=6", body.OverviewStats)
	}
	if len(body.Channels) != 6 {
		t.Fatalf("channels len = %d, want 6", len(body.Channels))
	}
	if len(body.TrafficSeries) != 24 {
		t.Fatalf("traffic len = %d, want 24", len(body.TrafficSeries))
	}
}

func TestIntegrationAPIChannelActions(t *testing.T) {
	handler := buildHandler(time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/channels/ch_wecom/test", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("test status = %d, want 200", response.Code)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/channels/ch_wecom/disable", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200", response.Code)
	}
	var body struct {
		Channel struct {
			Status string `json:"status"`
		} `json:"channel"`
	}
	decodeJSON(t, response.Body.Bytes(), &body)
	if body.Channel.Status != "disabled" {
		t.Fatalf("status = %q, want disabled", body.Channel.Status)
	}
}

func TestIntegrationAPIMessageFlowFilter(t *testing.T) {
	handler := buildHandler(time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/message-flow?channel=wecom&status=success", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		MessageEvents []struct {
			Channel string `json:"channel"`
			Status  string `json:"status"`
		} `json:"messageEvents"`
	}
	decodeJSON(t, response.Body.Bytes(), &body)
	if len(body.MessageEvents) == 0 {
		t.Fatalf("messageEvents is empty")
	}
	for _, event := range body.MessageEvents {
		if event.Channel != "wecom" || event.Status != "success" {
			t.Fatalf("unexpected event %+v", event)
		}
	}
}

func TestIntegrationAPIConversationSendCreatesOutboxItem(t *testing.T) {
	handler := buildHandler(time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC))
	payload := bytes.NewBufferString(`{"content":"Please review the quote.","sender":"Sarah Chen"}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv_101/messages", payload)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", response.Code, response.Body.String())
	}
	var body struct {
		OutboxItem struct {
			ConversationID string `json:"conversationId"`
			Status         string `json:"status"`
		} `json:"outboxItem"`
	}
	decodeJSON(t, response.Body.Bytes(), &body)
	if body.OutboxItem.ConversationID != "conv_101" || body.OutboxItem.Status != "pending" {
		t.Fatalf("outbox item = %+v", body.OutboxItem)
	}
}

func TestIntegrationAPIOutboxActions(t *testing.T) {
	handler := buildHandler(time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/outbox/out_2/retry", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		OutboxItem struct {
			Status     string `json:"status"`
			RetryCount int    `json:"retryCount"`
		} `json:"outboxItem"`
	}
	decodeJSON(t, response.Body.Bytes(), &body)
	if body.OutboxItem.Status != "sending" || body.OutboxItem.RetryCount != 4 {
		t.Fatalf("outbox item = %+v, want sending retry=4", body.OutboxItem)
	}
}

func decodeJSON(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode json: %v body=%s", err, string(data))
	}
}
