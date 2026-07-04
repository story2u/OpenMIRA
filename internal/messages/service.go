package messages

import (
	"context"
	"errors"
)

// ErrStoreUnavailable means the route was assembled without a message store.
var ErrStoreUnavailable = errors.New("conversation messages store is unavailable")

// Service builds legacy conversation message page payloads.
type Service struct {
	Store Store
}

// List returns one message page using the legacy response envelope.
func (service Service) List(ctx context.Context, request Request) (Payload, error) {
	if service.Store == nil {
		return nil, ErrStoreUnavailable
	}
	query := Query{
		ConversationID: request.ConversationID,
		Limit:          request.Limit,
		Offset:         request.Offset,
		After:          DecodeCursor(request.AfterCursor),
		Before:         DecodeCursor(request.BeforeCursor),
	}.Normalized()
	if query.Before != nil {
		query.After = nil
	}
	page, err := service.Store.List(ctx, query)
	if err != nil {
		return nil, err
	}
	firstCursor := ""
	lastCursor := ""
	if len(page.Records) > 0 {
		firstCursor = EncodeCursor(page.Records[0])
		lastCursor = EncodeCursor(page.Records[len(page.Records)-1])
	} else {
		if query.Before != nil {
			firstCursor = query.Before.Raw
		}
		if query.After != nil {
			lastCursor = query.After.Raw
		}
	}
	return Payload{
		"messages":      SerializeRecords(page.Records),
		"total":         page.Total,
		"offset":        query.Offset,
		"limit":         query.Limit,
		"has_more":      page.HasMore,
		"first_cursor":  nullableCursor(firstCursor),
		"last_cursor":   nullableCursor(lastCursor),
		"before_cursor": nullableCursor(firstCursor),
		"after_cursor":  nullableCursor(lastCursor),
		"next_cursor":   nullableCursor(firstCursor),
	}, nil
}

func nullableCursor(cursor string) any {
	if cursor == "" {
		return nil
	}
	return cursor
}
