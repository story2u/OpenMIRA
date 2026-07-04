package messages

import (
	"net/url"
	"testing"

	"wework-go/internal/auth"
)

func TestNewRequestAppliesLegacyDefaults(t *testing.T) {
	request := NewRequest(" conv-001 ", url.Values{}, auth.Session{AssigneeID: "cs-001"})

	if request.ConversationID != "conv-001" || request.Limit != 20 || request.Offset != 0 || request.Session.AssigneeID != "cs-001" {
		t.Fatalf("request = %+v", request)
	}
}

func TestNewRequestNormalizesPagingParameters(t *testing.T) {
	values := url.Values{
		"limit":         {"999"},
		"offset":        {"12"},
		"after_cursor":  {" 1000:1:trace-a "},
		"before_cursor": {" 900:2:trace-b "},
		"fresh":         {"1"},
	}

	request := NewRequest("conv-002", values, auth.Session{})

	if request.Limit != 500 || request.Offset != 12 || request.AfterCursor != "1000:1:trace-a" || request.BeforeCursor != "900:2:trace-b" || !request.Fresh {
		t.Fatalf("request = %+v", request)
	}
}
