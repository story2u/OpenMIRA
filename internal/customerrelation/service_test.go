package customerrelation

import (
	"context"
	"testing"
	"time"
)

func TestHandleCallbackXMLDeletedRelationPayload(t *testing.T) {
	eventTime := time.Unix(1778116784, 0).UTC()
	repository := &fakeRepository{row: RelationRow{
		RelationStatus: RelationStatusDeletedByCustomer,
		Source:         "callback",
		DeletedAt:      &eventTime,
		StateChanged:   true,
	}}
	service := Service{
		Repository: repository,
		NormalizeWeWorkUserID: func(value string) string {
			return "lisi"
		},
	}

	payload, ok, err := service.HandleCallbackXML(context.Background(), "ent-1", "ww755a6b4a180003f4", callbackXML(ChangeTypeDelFollowUser))
	if err != nil {
		t.Fatalf("HandleCallbackXML returned error: %v", err)
	}
	if !ok {
		t.Fatal("callback was not handled")
	}
	if repository.event.ChangeType != ChangeTypeDelFollowUser || repository.event.WeWorkUserID != "lisi" || repository.event.ExternalUserID != "wmexternal123" {
		t.Fatalf("event = %+v", repository.event)
	}
	if repository.event.RawEventHash == "" || !repository.event.EventTime.Equal(eventTime) {
		t.Fatalf("event hash/time = %q %s", repository.event.RawEventHash, repository.event.EventTime)
	}
	if payload["conversation_id"] != "ww:lisi:wmexternal123" ||
		payload["customer_deleted_current_member"] != true ||
		payload["customer_relation_badge_text"] != "客户已删除" ||
		payload["customer_relation_badge_level"] != "danger" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleCallbackXMLSupportsSuiteInfoType(t *testing.T) {
	repository := &fakeRepository{row: RelationRow{RelationStatus: RelationStatusDeletedByCustomer, Source: "callback"}}
	service := Service{Repository: repository}

	payload, ok, err := service.HandleCallbackXML(context.Background(), "ent-1", "corp-1", suiteCallbackXML(ChangeTypeDelExternalContact))
	if err != nil {
		t.Fatalf("HandleCallbackXML returned error: %v", err)
	}
	if !ok {
		t.Fatal("callback was not handled")
	}
	if repository.event.EventType != EventTypeChangeExternalContact || repository.event.ChangeType != ChangeTypeDelExternalContact {
		t.Fatalf("event = %+v", repository.event)
	}
	if payload["customer_relation_status"] != RelationStatusDeletedByCustomer {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleCallbackXMLAddRelationFirstAdd(t *testing.T) {
	restoredAt := time.Unix(1778116784, 0).UTC()
	repository := &fakeRepository{row: RelationRow{
		RelationStatus:   RelationStatusActive,
		Source:           "callback",
		RestoredAt:       &restoredAt,
		StateChanged:     true,
		RelationFirstAdd: true,
	}}
	service := Service{Repository: repository}

	payload, ok, err := service.HandleCallbackXML(context.Background(), "ent-1", "corp-1", callbackXML(ChangeTypeAddExternalContact))
	if err != nil {
		t.Fatalf("HandleCallbackXML returned error: %v", err)
	}
	if !ok || payload["customer_relation_status"] != RelationStatusActive || payload["relation_first_add"] != true {
		t.Fatalf("payload=%#v ok=%t", payload, ok)
	}
	if payload["customer_relation_badge_text"] != "" || payload["customer_relation_badge_level"] != "normal" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleCallbackXMLEditProfileDoesNotPersistRelation(t *testing.T) {
	repository := &fakeRepository{}
	service := Service{Repository: repository}

	payload, ok, err := service.HandleCallbackXML(context.Background(), "ent-1", "corp-1", callbackXML(ChangeTypeEditExternalContact))
	if err != nil {
		t.Fatalf("HandleCallbackXML returned error: %v", err)
	}
	if !ok {
		t.Fatal("callback was not handled")
	}
	if repository.called {
		t.Fatal("edit_external_contact should not upsert relation state")
	}
	if payload["change_type"] != ChangeTypeEditExternalContact ||
		payload["contact_profile_refresh_required"] != true ||
		payload["wework_user_id"] != "lisi" ||
		payload["raw_wework_user_id"] != "Li-Si" ||
		payload["external_userid"] != "wmexternal123" ||
		payload["raw_external_userid"] != "wmEXTERNAL123" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleCallbackXMLIgnoresUnsupportedEvents(t *testing.T) {
	service := Service{Repository: &fakeRepository{}}

	payload, ok, err := service.HandleCallbackXML(context.Background(), "ent-1", "corp-1", `<xml><Event>unknown</Event></xml>`)
	if err != nil {
		t.Fatalf("HandleCallbackXML returned error: %v", err)
	}
	if ok || payload != nil {
		t.Fatalf("payload=%#v ok=%t, want ignored", payload, ok)
	}
}

func TestHandleCallbackXMLRequiresRelationIDs(t *testing.T) {
	service := Service{Repository: &fakeRepository{}}

	_, ok, err := service.HandleCallbackXML(context.Background(), "ent-1", "corp-1", `<xml><Event>change_external_contact</Event><ChangeType>add_external_contact</ChangeType></xml>`)
	if !ok || err != ErrMissingRelationIDs {
		t.Fatalf("ok=%t err=%v, want missing ids", ok, err)
	}
}

type fakeRepository struct {
	event  Event
	row    RelationRow
	called bool
}

func (repository *fakeRepository) UpsertEvent(ctx context.Context, event Event) (RelationRow, error) {
	repository.event = event
	repository.called = true
	row := repository.row
	row.EnterpriseID = event.EnterpriseID
	row.WeWorkUserID = event.WeWorkUserID
	row.ExternalUserID = event.ExternalUserID
	if row.Source == "" {
		row.Source = event.Source
	}
	return row, nil
}

func callbackXML(changeType string) string {
	return `
<xml>
  <ToUserName><![CDATA[ww755a6b4a180003f4]]></ToUserName>
  <CreateTime>1778116784</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[change_external_contact]]></Event>
  <ChangeType><![CDATA[` + changeType + `]]></ChangeType>
  <UserID><![CDATA[Li-Si]]></UserID>
  <ExternalUserID><![CDATA[wmEXTERNAL123]]></ExternalUserID>
</xml>`
}

func suiteCallbackXML(changeType string) string {
	return `
<xml>
  <SuiteId><![CDATA[suite-1]]></SuiteId>
  <AuthCorpId><![CDATA[ww755a6b4a180003f4]]></AuthCorpId>
  <InfoType><![CDATA[change_external_contact]]></InfoType>
  <TimeStamp>1778116784</TimeStamp>
  <ChangeType><![CDATA[` + changeType + `]]></ChangeType>
  <UserID><![CDATA[Li-Si]]></UserID>
  <ExternalUserID><![CDATA[wmEXTERNAL123]]></ExternalUserID>
</xml>`
}
