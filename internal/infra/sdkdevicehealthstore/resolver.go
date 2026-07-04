package sdkdevicehealthstore

import (
	"context"
	"strings"
)

// DeviceIDResolver resolves SDK device aliases to canonical P1 device ids.
type DeviceIDResolver interface {
	ResolveSDKDeviceID(ctx context.Context, deviceID string) (string, error)
}

// DeviceIDResolverFunc adapts a function into a DeviceIDResolver.
type DeviceIDResolverFunc func(ctx context.Context, deviceID string) (string, error)

// ResolveSDKDeviceID implements DeviceIDResolver.
func (fn DeviceIDResolverFunc) ResolveSDKDeviceID(ctx context.Context, deviceID string) (string, error) {
	return fn(ctx, deviceID)
}

func resolveDeviceID(ctx context.Context, resolver DeviceIDResolver, deviceID string) string {
	normalized := strings.TrimSpace(deviceID)
	if normalized == "" || resolver == nil {
		return normalized
	}
	resolved, err := resolver.ResolveSDKDeviceID(ctx, normalized)
	if err != nil {
		return normalized
	}
	if canonical := strings.TrimSpace(resolved); canonical != "" {
		return canonical
	}
	return normalized
}
