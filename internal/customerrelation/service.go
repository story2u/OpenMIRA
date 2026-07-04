// Package customerrelation parses WeCom customer-contact callbacks.
package customerrelation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	EventTypeChangeExternalContact = "change_external_contact"

	ChangeTypeDelExternalContact  = "del_external_contact"
	ChangeTypeDelFollowUser       = "del_follow_user"
	ChangeTypeAddExternalContact  = "add_external_contact"
	ChangeTypeEditExternalContact = "edit_external_contact"

	RelationStatusActive            = "active"
	RelationStatusDeletedByCustomer = "deleted_by_customer"
)

var (
	// ErrMissingRelationIDs matches the legacy callback validation boundary.
	ErrMissingRelationIDs = errors.New("customer relation callback missing UserID or ExternalUserID")
	// ErrUnsupportedRelationChangeType means a parsed event is not a relation state change.
	ErrUnsupportedRelationChangeType = errors.New("unsupported customer relation change_type")
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Repository persists customer/member relation events.
type Repository interface {
	UpsertEvent(ctx context.Context, event Event) (RelationRow, error)
}

// Service applies supported WeCom customer-contact callback XML.
type Service struct {
	Repository            Repository
	NormalizeWeWorkUserID func(string) string
	Now                   func() time.Time
}

// Event is the normalized relation write command.
type Event struct {
	EnterpriseID   string
	WeWorkUserID   string
	ExternalUserID string
	EventType      string
	ChangeType     string
	EventTime      time.Time
	RawEventHash   string
	Source         string
}

// RelationRow is the persisted relation state needed to build realtime payloads.
type RelationRow struct {
	EnterpriseID     string
	WeWorkUserID     string
	ExternalUserID   string
	RelationStatus   string
	Source           string
	EventType        string
	ChangeType       string
	DeletedAt        *time.Time
	RestoredAt       *time.Time
	LastSeenAt       *time.Time
	LastEventAt      *time.Time
	RawEventHash     string
	StateChanged     bool
	IgnoredStale     bool
	RelationFirstAdd bool
}

// Payload is the legacy websocket/API payload shape for relation callbacks.
type Payload map[string]any

type parsedEvent struct {
	EventType         string
	ChangeType        string
	WeWorkUserID      string
	RawWeWorkUserID   string
	ExternalUserID    string
	RawExternalUserID string
	EventTime         time.Time
}

// HandleCallbackXML persists one supported relation callback and returns the payload.
func (service Service) HandleCallbackXML(ctx context.Context, enterpriseID string, corpID string, xmlText string) (Payload, bool, error) {
	parsed, ok, err := service.ParseCallbackXML(xmlText)
	if err != nil || !ok {
		return nil, ok, err
	}
	if parsed.ChangeType == ChangeTypeEditExternalContact {
		return service.buildProfileEditPayload(enterpriseID, corpID, parsed), true, nil
	}
	if service.Repository == nil {
		return nil, true, errors.New("customer relation repository is not configured")
	}
	row, err := service.Repository.UpsertEvent(ctx, Event{
		EnterpriseID:   strings.TrimSpace(enterpriseID),
		WeWorkUserID:   parsed.WeWorkUserID,
		ExternalUserID: parsed.ExternalUserID,
		EventType:      parsed.EventType,
		ChangeType:     parsed.ChangeType,
		EventTime:      parsed.EventTime,
		RawEventHash:   rawEventHash(xmlText),
		Source:         "callback",
	})
	if err != nil {
		return nil, true, err
	}
	return service.buildRelationPayload(enterpriseID, corpID, row, parsed.ChangeType, parsed.EventTime), true, nil
}

// ParseCallbackXML returns a normalized relation event, or ok=false for unsupported callbacks.
func (service Service) ParseCallbackXML(xmlText string) (parsedEvent, bool, error) {
	fields, err := parseXMLFields(xmlText)
	if err != nil {
		return parsedEvent{}, false, err
	}
	eventType := firstNonEmpty(fields["Event"], fields["InfoType"])
	changeType := fields["ChangeType"]
	if eventType != EventTypeChangeExternalContact || !supportedChangeType(changeType) {
		return parsedEvent{}, false, nil
	}
	rawWeWorkUserID := fields["UserID"]
	rawExternalUserID := fields["ExternalUserID"]
	weworkUserID := service.normalizeWeWorkUserID(rawWeWorkUserID)
	externalUserID := normalizeExternalUserID(rawExternalUserID)
	if weworkUserID == "" || externalUserID == "" {
		return parsedEvent{}, true, ErrMissingRelationIDs
	}
	return parsedEvent{
		EventType:         eventType,
		ChangeType:        changeType,
		WeWorkUserID:      weworkUserID,
		RawWeWorkUserID:   strings.TrimSpace(rawWeWorkUserID),
		ExternalUserID:    externalUserID,
		RawExternalUserID: strings.TrimSpace(rawExternalUserID),
		EventTime:         service.parseEventTime(firstNonEmpty(fields["CreateTime"], fields["TimeStamp"])),
	}, true, nil
}

func (service Service) buildProfileEditPayload(enterpriseID string, corpID string, parsed parsedEvent) Payload {
	return Payload{
		"enterprise_id":                    strings.TrimSpace(enterpriseID),
		"corp_id":                          strings.TrimSpace(corpID),
		"wework_user_id":                   parsed.WeWorkUserID,
		"raw_wework_user_id":               defaultText(parsed.RawWeWorkUserID, parsed.WeWorkUserID),
		"external_userid":                  parsed.ExternalUserID,
		"raw_external_userid":              defaultText(parsed.RawExternalUserID, parsed.ExternalUserID),
		"conversation_id":                  conversationID(parsed.WeWorkUserID, parsed.ExternalUserID),
		"change_type":                      ChangeTypeEditExternalContact,
		"occurred_at":                      formatBeijingAPIISO(parsed.EventTime),
		"state_changed":                    false,
		"ignored_stale":                    false,
		"relation_first_add":               false,
		"contact_profile_refresh_required": true,
	}
}

func (service Service) buildRelationPayload(enterpriseID string, corpID string, row RelationRow, changeType string, eventTime time.Time) Payload {
	status := defaultText(row.RelationStatus, RelationStatusActive)
	deleted := status == RelationStatusDeletedByCustomer
	weworkUserID := service.normalizeWeWorkUserID(row.WeWorkUserID)
	externalUserID := normalizeExternalUserID(row.ExternalUserID)
	return Payload{
		"enterprise_id":                   strings.TrimSpace(enterpriseID),
		"corp_id":                         strings.TrimSpace(corpID),
		"wework_user_id":                  weworkUserID,
		"external_userid":                 externalUserID,
		"conversation_id":                 conversationID(weworkUserID, externalUserID),
		"change_type":                     strings.TrimSpace(changeType),
		"customer_relation_status":        status,
		"customer_relation_source":        defaultText(row.Source, "callback"),
		"customer_relation_deleted_at":    formatOptionalBeijing(row.DeletedAt),
		"customer_relation_restored_at":   formatOptionalBeijing(row.RestoredAt),
		"customer_deleted_current_member": deleted,
		"customer_relation_badge_text":    map[bool]string{true: "客户已删除", false: ""}[deleted],
		"customer_relation_badge_level":   map[bool]string{true: "danger", false: "normal"}[deleted],
		"occurred_at":                     formatBeijingAPIISO(eventTime),
		"state_changed":                   row.StateChanged,
		"ignored_stale":                   row.IgnoredStale,
		"relation_first_add":              row.RelationFirstAdd,
	}
}

func (service Service) normalizeWeWorkUserID(value string) string {
	if service.NormalizeWeWorkUserID != nil {
		return strings.TrimSpace(service.NormalizeWeWorkUserID(value))
	}
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func (service Service) parseEventTime(value string) time.Time {
	text := strings.TrimSpace(value)
	if text != "" {
		if seconds, err := parseUnixSeconds(text); err == nil {
			return time.Unix(seconds, 0).UTC()
		}
	}
	return service.now()
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func supportedChangeType(changeType string) bool {
	switch strings.TrimSpace(changeType) {
	case ChangeTypeDelExternalContact, ChangeTypeDelFollowUser, ChangeTypeAddExternalContact, ChangeTypeEditExternalContact:
		return true
	default:
		return false
	}
}

func parseXMLFields(xmlText string) (map[string]string, error) {
	decoder := xml.NewDecoder(strings.NewReader(strings.TrimSpace(xmlText)))
	fields := map[string]string{}
	wanted := map[string]struct{}{
		"Event":          {},
		"InfoType":       {},
		"ChangeType":     {},
		"UserID":         {},
		"ExternalUserID": {},
		"CreateTime":     {},
		"TimeStamp":      {},
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fields, nil
			}
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if _, ok := wanted[start.Name.Local]; !ok {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err != nil {
			return nil, err
		}
		fields[start.Name.Local] = strings.TrimSpace(value)
	}
}

func rawEventHash(xmlText string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(xmlText)))
	return hex.EncodeToString(sum[:])
}

func normalizeExternalUserID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func conversationID(weworkUserID string, externalUserID string) string {
	weworkUserID = strings.TrimSpace(weworkUserID)
	externalUserID = strings.TrimSpace(externalUserID)
	if weworkUserID == "" || externalUserID == "" {
		return ""
	}
	return "ww:" + weworkUserID + ":" + externalUserID
}

func formatOptionalBeijing(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return formatBeijingAPIISO(*value)
}

func formatBeijingAPIISO(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(beijingLocation).Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func parseUnixSeconds(value string) (int64, error) {
	var seconds int64
	for _, ch := range strings.TrimSpace(value) {
		if ch < '0' || ch > '9' {
			return 0, errors.New("invalid unix timestamp")
		}
		seconds = seconds*10 + int64(ch-'0')
	}
	return seconds, nil
}
