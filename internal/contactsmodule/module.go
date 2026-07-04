// Package contactsmodule assembles cached WeWork contact read/sync components.
package contactsmodule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/contacts"
	"wework-go/internal/contactsyncscheduler"
	"wework-go/internal/infra/contactcache"
	"wework-go/internal/infra/contactidentitymaster"
	"wework-go/internal/infra/customerrelations"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/infra/weworkcontactapi"
)

var (
	// ErrStoreRequired means contact reads were assembled without a cache store.
	ErrStoreRequired = errors.New("contact cache store is required")
	// ErrSyncStoreRequired means sync routes were assembled without writable cache stores.
	ErrSyncStoreRequired = errors.New("contact sync cache stores are required")
	// ErrEnterpriseStoreRequired means sync routes were assembled without enterprise secrets.
	ErrEnterpriseStoreRequired = errors.New("contact sync enterprise store is required")
)

// ContactClient is the contacts.Service client boundary for WeCom contact APIs.
type ContactClient interface {
	contacts.ExternalContactGetter
	contacts.ExternalContactIDLister
	contacts.InternalUserGetter
	contacts.InternalUserLister
}

// Options contains dependencies needed by the contact candidate module.
type Options struct {
	DB                   *sql.DB
	DBDialect            string
	Store                contacts.Store
	ExternalWriter       contacts.ExternalContactWriter
	ExternalSkipper      contacts.ExternalContactRefreshSkipper
	CorpWriter           contacts.CorpUserWriter
	StaleExternal        contacts.StaleExternalContactLister
	StaleCorp            contacts.StaleCorpUserLister
	ContactClient        ContactClient
	Enterprises          contacts.EnterpriseSecretStore
	SchedulerEnterprises contactsyncscheduler.EnterpriseLister
	Identity             contacts.IdentityWriter
	Relations            contacts.FollowUserRelationReconciler
	AvatarStorage        contacts.AvatarStorage
	BuildSync            bool
	Now                  func() time.Time
}

// Module groups the unmounted contact service and optional sync scheduler.
type Module struct {
	Service              contacts.Service
	Scheduler            contactsyncscheduler.Scheduler
	StoreRepository      *contactcache.Repository
	EnterpriseRepository *enterprisestore.Repository
	IdentityRepository   *contactidentitymaster.Repository
	RelationRepository   *customerrelations.Repository
}

// New wires SQL-backed or injected contact read/sync dependencies.
func New(options Options) (Module, error) {
	store := options.Store
	var storeRepository *contactcache.Repository
	if store == nil && options.DB != nil {
		storeRepository = contactcache.NewSQLRepository(options.DB, options.DBDialect)
		storeRepository.Now = options.Now
		store = storeRepository
	}
	if store == nil {
		return Module{}, ErrStoreRequired
	}

	service := contacts.Service{
		Store:         store,
		AvatarStorage: options.AvatarStorage,
		Now:           options.Now,
	}
	module := Module{
		Service:         service,
		StoreRepository: storeRepository,
	}
	if !options.BuildSync {
		return module, nil
	}

	externalWriter := firstExternalWriter(options.ExternalWriter, store)
	externalSkipper := firstExternalSkipper(options.ExternalSkipper, store)
	corpWriter := firstCorpWriter(options.CorpWriter, store)
	staleExternal := firstStaleExternal(options.StaleExternal, store)
	staleCorp := firstStaleCorp(options.StaleCorp, store)
	if externalWriter == nil || externalSkipper == nil || corpWriter == nil || staleExternal == nil || staleCorp == nil {
		return Module{}, ErrSyncStoreRequired
	}

	enterpriseSecrets := options.Enterprises
	schedulerEnterprises := options.SchedulerEnterprises
	var enterpriseRepository *enterprisestore.Repository
	if (enterpriseSecrets == nil || schedulerEnterprises == nil) && options.DB != nil {
		enterpriseRepository = enterprisestore.NewSQLRepository(options.DB)
		adapter := EnterpriseAdapter{Store: enterpriseRepository}
		if enterpriseSecrets == nil {
			enterpriseSecrets = adapter
		}
		if schedulerEnterprises == nil {
			schedulerEnterprises = adapter
		}
	}
	if enterpriseSecrets == nil {
		return Module{}, ErrEnterpriseStoreRequired
	}

	contactClient := options.ContactClient
	if contactClient == nil {
		contactClient = ContactClientAdapter{Client: &weworkcontactapi.Client{Now: options.Now}}
	}

	identity := options.Identity
	var identityRepository *contactidentitymaster.Repository
	if identity == nil && options.DB != nil {
		identityRepository = contactidentitymaster.NewSQLRepository(options.DB, options.DBDialect)
		identityRepository.Now = options.Now
		identity = identityRepository
	}
	relations := options.Relations
	var relationRepository *customerrelations.Repository
	if relations == nil && options.DB != nil {
		relationRepository = customerrelations.NewSQLRepository(options.DB)
		relations = RelationReconciler{Repository: relationRepository}
	}

	service.ExternalContactWriter = externalWriter
	service.ExternalContactGetter = contactClient
	service.ExternalContactIDLister = contactClient
	service.ExternalContactSkipper = externalSkipper
	service.CorpUserWriter = corpWriter
	service.InternalUserGetter = contactClient
	service.InternalUserLister = contactClient
	service.StaleExternalContacts = staleExternal
	service.StaleCorpUsers = staleCorp
	service.Enterprises = enterpriseSecrets
	service.Identity = identity
	service.Relations = relations

	module.Service = service
	module.EnterpriseRepository = enterpriseRepository
	module.IdentityRepository = identityRepository
	module.RelationRepository = relationRepository
	if schedulerEnterprises != nil {
		module.Scheduler = contactsyncscheduler.Scheduler{
			Service:     service,
			Enterprises: schedulerEnterprises,
		}
	}
	return module, nil
}

// EnterpriseStore is the subset shared by enterprisestore.Repository and tests.
type EnterpriseStore interface {
	GetEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.EnterpriseRecord, error)
	ListEnterprises(ctx context.Context) ([]enterprisestore.EnterpriseRecord, error)
}

// EnterpriseAdapter exposes enterprise secrets and scheduler records.
type EnterpriseAdapter struct {
	Store EnterpriseStore
}

// GetEnterpriseSecrets adapts the legacy enterprises table to contacts.Service.
func (adapter EnterpriseAdapter) GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (contacts.EnterpriseSecrets, bool, error) {
	if adapter.Store == nil {
		return contacts.EnterpriseSecrets{}, false, nil
	}
	record, err := adapter.Store.GetEnterprise(ctx, enterpriseID)
	if err != nil || record == nil {
		return contacts.EnterpriseSecrets{}, false, err
	}
	return contacts.EnterpriseSecrets{
		EnterpriseID:          record.EnterpriseID,
		CorpID:                record.CorpID,
		CorpSecret:            record.CorpSecret,
		ContactSecret:         record.ContactSecret,
		ExternalContactSecret: record.ExternalContactSecret,
	}, true, nil
}

// ListEnterprises adapts enabled flags for contactsyncscheduler.
func (adapter EnterpriseAdapter) ListEnterprises(ctx context.Context) ([]contactsyncscheduler.Enterprise, error) {
	if adapter.Store == nil {
		return nil, ErrEnterpriseStoreRequired
	}
	records, err := adapter.Store.ListEnterprises(ctx)
	if err != nil {
		return nil, err
	}
	enterprises := make([]contactsyncscheduler.Enterprise, 0, len(records))
	for _, record := range records {
		enterprises = append(enterprises, contactsyncscheduler.Enterprise{
			EnterpriseID: strings.TrimSpace(record.EnterpriseID),
			Enabled:      record.Enabled,
		})
	}
	return enterprises, nil
}

// WeWorkContactClient is the infra client boundary used by ContactClientAdapter.
type WeWorkContactClient interface {
	GetExternalContact(ctx context.Context, request weworkcontactapi.GetExternalContactRequest) (map[string]any, error)
	ListExternalContactIDs(ctx context.Context, request weworkcontactapi.ListExternalContactIDsRequest) ([]string, error)
	GetInternalUser(ctx context.Context, request weworkcontactapi.GetInternalUserRequest) (map[string]any, error)
	ListInternalUsers(ctx context.Context, request weworkcontactapi.ListInternalUsersRequest) ([]map[string]any, error)
}

// ContactClientAdapter converts contacts request types to infra/weworkcontactapi.
type ContactClientAdapter struct {
	Client WeWorkContactClient
}

func (adapter ContactClientAdapter) GetExternalContact(ctx context.Context, request contacts.ExternalContactGetRequest) (map[string]any, error) {
	if adapter.Client == nil {
		return nil, fmt.Errorf("wework contact api client is not configured")
	}
	return adapter.Client.GetExternalContact(ctx, weworkcontactapi.GetExternalContactRequest{
		EnterpriseID:   request.EnterpriseID,
		CorpID:         request.CorpID,
		CorpSecret:     request.CorpSecret,
		ExternalUserID: request.ExternalUserID,
	})
}

func (adapter ContactClientAdapter) ListExternalContactIDs(ctx context.Context, request contacts.ListExternalContactIDsRequest) ([]string, error) {
	if adapter.Client == nil {
		return nil, fmt.Errorf("wework contact api client is not configured")
	}
	return adapter.Client.ListExternalContactIDs(ctx, weworkcontactapi.ListExternalContactIDsRequest{
		EnterpriseID: request.EnterpriseID,
		CorpID:       request.CorpID,
		CorpSecret:   request.CorpSecret,
		UserID:       request.UserID,
	})
}

func (adapter ContactClientAdapter) GetInternalUser(ctx context.Context, request contacts.InternalUserGetRequest) (map[string]any, error) {
	if adapter.Client == nil {
		return nil, fmt.Errorf("wework contact api client is not configured")
	}
	return adapter.Client.GetInternalUser(ctx, weworkcontactapi.GetInternalUserRequest{
		EnterpriseID: request.EnterpriseID,
		CorpID:       request.CorpID,
		CorpSecret:   request.CorpSecret,
		UserID:       request.UserID,
	})
}

func (adapter ContactClientAdapter) ListInternalUsers(ctx context.Context, request contacts.ListInternalUsersRequest) ([]map[string]any, error) {
	if adapter.Client == nil {
		return nil, fmt.Errorf("wework contact api client is not configured")
	}
	return adapter.Client.ListInternalUsers(ctx, weworkcontactapi.ListInternalUsersRequest{
		EnterpriseID: request.EnterpriseID,
		CorpID:       request.CorpID,
		CorpSecret:   request.CorpSecret,
	})
}

// RelationRepository is the customer relation reconciliation boundary.
type RelationRepository interface {
	ReconcileExternalContactFollowUsers(ctx context.Context, input customerrelations.FollowUserReconcileInput) (customerrelations.FollowUserReconcileResult, error)
}

// RelationReconciler adapts contacts follow_user reconciliation to infra storage.
type RelationReconciler struct {
	Repository RelationRepository
}

func (adapter RelationReconciler) ReconcileExternalContactFollowUsers(ctx context.Context, input contacts.FollowUserRelationReconcileInput) error {
	if adapter.Repository == nil {
		return nil
	}
	_, err := adapter.Repository.ReconcileExternalContactFollowUsers(ctx, customerrelations.FollowUserReconcileInput{
		EnterpriseID:   input.EnterpriseID,
		ExternalUserID: input.ExternalUserID,
		FollowUserIDs:  input.FollowUserIDs,
		EventTime:      input.EventTime,
		Source:         input.Source,
	})
	return err
}

func firstExternalWriter(explicit contacts.ExternalContactWriter, store contacts.Store) contacts.ExternalContactWriter {
	if explicit != nil {
		return explicit
	}
	if writer, ok := store.(contacts.ExternalContactWriter); ok {
		return writer
	}
	return nil
}

func firstExternalSkipper(explicit contacts.ExternalContactRefreshSkipper, store contacts.Store) contacts.ExternalContactRefreshSkipper {
	if explicit != nil {
		return explicit
	}
	if skipper, ok := store.(contacts.ExternalContactRefreshSkipper); ok {
		return skipper
	}
	return nil
}

func firstCorpWriter(explicit contacts.CorpUserWriter, store contacts.Store) contacts.CorpUserWriter {
	if explicit != nil {
		return explicit
	}
	if writer, ok := store.(contacts.CorpUserWriter); ok {
		return writer
	}
	return nil
}

func firstStaleExternal(explicit contacts.StaleExternalContactLister, store contacts.Store) contacts.StaleExternalContactLister {
	if explicit != nil {
		return explicit
	}
	if lister, ok := store.(contacts.StaleExternalContactLister); ok {
		return lister
	}
	return nil
}

func firstStaleCorp(explicit contacts.StaleCorpUserLister, store contacts.Store) contacts.StaleCorpUserLister {
	if explicit != nil {
		return explicit
	}
	if lister, ok := store.(contacts.StaleCorpUserLister); ok {
		return lister
	}
	return nil
}
