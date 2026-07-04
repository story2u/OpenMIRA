package workbenchmodule

import (
	"context"
	"database/sql"
	"time"

	"wework-go/internal/infra/contactidentitymaster"
	"wework-go/internal/infra/weworkcontactapi"
	"wework-go/internal/workbench"
)

func newCustomerProfileContactClient(now func() time.Time) workbench.CustomerProfileContactClient {
	return customerProfileContactClient{client: &weworkcontactapi.Client{Now: now}}
}

func newCustomerProfileIdentityStore(db *sql.DB, dialect string, now func() time.Time) workbench.CustomerProfileIdentityStore {
	repository := contactidentitymaster.NewSQLRepository(db, dialect)
	repository.Now = now
	return repository
}

type customerProfileContactClient struct {
	client *weworkcontactapi.Client
}

func (adapter customerProfileContactClient) GetExternalContact(ctx context.Context, request workbench.CustomerProfileExternalContactGetRequest) (map[string]any, error) {
	return adapter.client.GetExternalContact(ctx, weworkcontactapi.GetExternalContactRequest{
		EnterpriseID:   request.EnterpriseID,
		CorpID:         request.CorpID,
		CorpSecret:     request.CorpSecret,
		ExternalUserID: request.ExternalUserID,
	})
}

func (adapter customerProfileContactClient) RemarkExternalContact(ctx context.Context, request workbench.CustomerProfileRemarkRequest) error {
	return adapter.client.RemarkExternalContact(ctx, weworkcontactapi.RemarkRequest{
		EnterpriseID:   request.EnterpriseID,
		CorpID:         request.CorpID,
		CorpSecret:     request.CorpSecret,
		UserID:         request.UserID,
		ExternalUserID: request.ExternalUserID,
		Remark:         request.Remark,
		Description:    request.Description,
		RemarkMobiles:  request.RemarkMobiles,
	})
}

func (adapter customerProfileContactClient) GetExternalCorpTagList(ctx context.Context, request workbench.CustomerProfileTagListRequest) (map[string]any, error) {
	return adapter.client.GetExternalCorpTagList(ctx, weworkcontactapi.CorpTagListRequest{
		EnterpriseID: request.EnterpriseID,
		CorpID:       request.CorpID,
		CorpSecret:   request.CorpSecret,
	})
}

func (adapter customerProfileContactClient) AddExternalCorpTags(ctx context.Context, request workbench.CustomerProfileAddTagsRequest) error {
	return adapter.client.AddExternalCorpTags(ctx, weworkcontactapi.AddCorpTagsRequest{
		EnterpriseID: request.EnterpriseID,
		CorpID:       request.CorpID,
		CorpSecret:   request.CorpSecret,
		TagNames:     request.TagNames,
		GroupID:      request.GroupID,
		GroupName:    request.GroupName,
	})
}

func (adapter customerProfileContactClient) MarkExternalContactTags(ctx context.Context, request workbench.CustomerProfileMarkTagsRequest) error {
	return adapter.client.MarkExternalContactTags(ctx, weworkcontactapi.MarkExternalContactTagsRequest{
		EnterpriseID:   request.EnterpriseID,
		CorpID:         request.CorpID,
		CorpSecret:     request.CorpSecret,
		UserID:         request.UserID,
		ExternalUserID: request.ExternalUserID,
		AddTagIDs:      request.AddTagIDs,
		RemoveTagIDs:   request.RemoveTagIDs,
	})
}
