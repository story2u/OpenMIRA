package workbench

import (
	"context"
	"sort"
	"strings"

	"wework-go/internal/auth"
)

// DiagnosticDeviceMapRequest carries the authenticated admin session.
type DiagnosticDeviceMapRequest struct {
	Session auth.Session
}

// NewDiagnosticDeviceMapRequest preserves the authenticated admin session.
func NewDiagnosticDeviceMapRequest(session auth.Session) DiagnosticDeviceMapRequest {
	return DiagnosticDeviceMapRequest{Session: session}
}

// DiagnosticDeviceMap builds /api/v1/admin/diagnostic/device-map.
func (service Service) DiagnosticDeviceMap(ctx context.Context, request DiagnosticDeviceMapRequest) (Payload, error) {
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Payload, 0, len(accounts))
	for _, account := range accounts {
		deviceID := strings.TrimSpace(account.DeviceID)
		weworkUserID := strings.TrimSpace(account.WeWorkUserID)
		if deviceID == "" || weworkUserID == "" {
			continue
		}
		items = append(items, Payload{
			"archive_user":   "archive_user:" + weworkUserID,
			"device_id":      deviceID,
			"wework_user_id": weworkUserID,
			"account_id":     strings.TrimSpace(account.AccountID),
			"account_name":   strings.TrimSpace(account.AccountName),
		})
	}
	sort.SliceStable(items, func(i int, j int) bool {
		leftUser, rightUser := strings.TrimSpace(items[i]["wework_user_id"].(string)), strings.TrimSpace(items[j]["wework_user_id"].(string))
		if leftUser != rightUser {
			return leftUser < rightUser
		}
		return strings.TrimSpace(items[i]["device_id"].(string)) < strings.TrimSpace(items[j]["device_id"].(string))
	})
	return Payload{"total": len(items), "items": items}, nil
}
