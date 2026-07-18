import type {
  ClientCapabilities,
  Device,
  DeviceRegistrationRequest,
  DeviceSession,
  PushEnvironment,
  PushProvider,
  PushRegistration,
  PushRegistrationRequest,
} from '@story2u/radar-contracts/devices';
import { Type } from 'typebox';

import { AuthUserSchema } from './auth';
import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const refreshTokenPattern = '^radar_device_1_[A-Za-z0-9_-]{43}$';
const capabilityKeyPattern = '^[A-Za-z][A-Za-z0-9._-]{0,63}$';
const localePattern = '^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$';
const semverPattern = '^\\d+\\.\\d+\\.\\d+(?:[-+][0-9A-Za-z.-]+)?$';

const NullableBoundedString = (maximum: number) => Type.Union([
  Type.String({ maxLength: maximum }),
  Type.Null(),
]);

const CapabilityValueSchema = Type.Union([
  Type.Boolean(),
  Type.Integer({ minimum: -1_000_000, maximum: 1_000_000 }),
  Type.String({ maxLength: 256 }),
]);

const CapabilitiesSchema = Type.Record(
  Type.String({ pattern: capabilityKeyPattern }),
  CapabilityValueSchema,
  { maxProperties: 64 },
);

export const DeviceSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    platform: Type.Union([Type.Literal('ios'), Type.Literal('android')]),
    status: Type.Union([Type.Literal('active'), Type.Literal('revoked')]),
    displayName: Type.String({ maxLength: 100 }),
    appVariant: Type.Union([Type.Literal('development'), Type.Literal('production')]),
    appVersion: Type.String({ minLength: 1, maxLength: 32 }),
    appBuild: Type.String({ minLength: 1, maxLength: 32 }),
    osVersion: NullableBoundedString(64),
    locale: NullableBoundedString(35),
    timezone: NullableBoundedString(64),
    capabilities: CapabilitiesSchema,
    lastSeenAt: Type.String({ minLength: 20, maxLength: 64 }),
    revokedAt: Type.Union([Type.String({ minLength: 20, maxLength: 64 }), Type.Null()]),
    createdAt: Type.String({ minLength: 20, maxLength: 64 }),
    updatedAt: Type.String({ minLength: 20, maxLength: 64 }),
  },
  { additionalProperties: false },
);

export const DeviceRegistrationSchema = Type.Object(
  {
    installationId: Type.String({ pattern: uuidPattern }),
    platform: Type.Union([Type.Literal('ios'), Type.Literal('android')]),
    displayName: Type.String({ maxLength: 100 }),
    appVariant: Type.Union([Type.Literal('development'), Type.Literal('production')]),
    appVersion: Type.String({ pattern: semverPattern, maxLength: 32 }),
    appBuild: Type.String({ minLength: 1, maxLength: 32, pattern: '^[0-9A-Za-z._+-]+$' }),
    osVersion: Type.Optional(NullableBoundedString(64)),
    locale: Type.Optional(Type.Union([
      Type.String({ pattern: localePattern, maxLength: 35 }),
      Type.Null(),
    ])),
    timezone: Type.Optional(NullableBoundedString(64)),
    capabilities: Type.Optional(CapabilitiesSchema),
  },
  { additionalProperties: false },
);

export const DeviceSessionSchema = Type.Object(
  {
    accessToken: Type.String({ minLength: 16, maxLength: 16_384 }),
    tokenType: Type.Literal('bearer'),
    deviceRefreshToken: Type.String({ pattern: refreshTokenPattern }),
    deviceRefreshTokenExpiresAt: Type.String({ minLength: 20, maxLength: 64 }),
    device: DeviceSchema,
    user: AuthUserSchema,
  },
  { additionalProperties: false },
);

export const ClientCapabilitiesSchema = Type.Object(
  {
    agentToolsAvailable: Type.Boolean(),
    deviceAgentAvailable: Type.Boolean(),
    e2eeAvailable: Type.Boolean(),
    hostedFallbackAvailable: Type.Boolean(),
    pushAvailable: Type.Boolean(),
    rnClientSupported: Type.Boolean(),
    syncAvailable: Type.Boolean(),
    signalAppetiteSyncAvailable: Type.Boolean(),
  },
  { additionalProperties: false },
);

const PushProviderSchema = Type.Union([Type.Literal('apns'), Type.Literal('fcm')]);
const PushEnvironmentSchema = Type.Union([
  Type.Literal('sandbox'),
  Type.Literal('production'),
]);

export const PushRegistrationRequestSchema = Type.Object(
  {
    provider: PushProviderSchema,
    environment: PushEnvironmentSchema,
    token: Type.String({ minLength: 16, maxLength: 4096 }),
  },
  { additionalProperties: false },
);

export const PushRegistrationSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    provider: PushProviderSchema,
    environment: PushEnvironmentSchema,
    status: Type.Union([
      Type.Literal('active'),
      Type.Literal('invalidated'),
      Type.Literal('revoked'),
    ]),
    tokenFingerprint: Type.String({ minLength: 12, maxLength: 12 }),
    lastRegisteredAt: Type.String({ minLength: 20, maxLength: 64 }),
    lastSuccessAt: Type.Union([Type.String({ minLength: 20, maxLength: 64 }), Type.Null()]),
    lastNotifiedCursor: Type.Integer({ minimum: 0 }),
  },
  { additionalProperties: false },
);

const decodeDevice: ResponseDecoder<Device> = typeboxDecoder(DeviceSchema);
const decodeDevices: ResponseDecoder<Device[]> = typeboxDecoder(
  Type.Array(DeviceSchema, { maxItems: 100 }),
);
const decodeDeviceSession: ResponseDecoder<DeviceSession> = typeboxDecoder(DeviceSessionSchema);
const decodeClientCapabilities: ResponseDecoder<ClientCapabilities> = typeboxDecoder(
  ClientCapabilitiesSchema,
);
const decodePushRegistration: ResponseDecoder<PushRegistration> = typeboxDecoder(
  PushRegistrationSchema,
);
const parsePushRegistration = typeboxDecoder(PushRegistrationRequestSchema);
const parseRegistration = typeboxDecoder(DeviceRegistrationSchema);

function explicitDeviceRefreshToken(value: string) {
  if (!(new RegExp(refreshTokenPattern)).test(value)) {
    throw new Error('Invalid device refresh token');
  }
  return { Authorization: `Bearer ${value}` };
}

function explicitDeviceId(value: string) {
  if (!(new RegExp(uuidPattern)).test(value)) throw new Error('Invalid device ID');
  return value;
}

export function createDevicesApi(client: RadarApiClient) {
  return {
    register(
      input: DeviceRegistrationRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<DeviceSession> {
      const payload = parseRegistration(input) as DeviceRegistrationRequest;
      return client.request('/api/v1/devices/register', {
        ...init,
        method: 'POST',
        body: JSON.stringify(payload),
        decode: decodeDeviceSession,
      });
    },

    list(init: Pick<RequestInit, 'signal'> = {}): Promise<Device[]> {
      return client.request('/api/v1/devices', { ...init, decode: decodeDevices });
    },

    capabilities(init: Pick<RequestInit, 'signal'> = {}): Promise<ClientCapabilities> {
      return client.request('/api/v1/devices/current/capabilities', {
        ...init,
        decode: decodeClientCapabilities,
      });
    },

    registerPushToken(
      input: PushRegistrationRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<PushRegistration> {
      const payload = parsePushRegistration(input) as PushRegistrationRequest;
      return client.request('/api/v1/devices/current/push-registration', {
        ...init,
        method: 'PUT',
        body: JSON.stringify(payload),
        decode: decodePushRegistration,
      });
    },

    revokePushToken(
      provider: PushProvider,
      environment: PushEnvironment,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<void> {
      if (provider !== 'apns' && provider !== 'fcm') throw new Error('Invalid push provider');
      if (environment !== 'sandbox' && environment !== 'production') {
        throw new Error('Invalid push environment');
      }
      return client.request(
        `/api/v1/devices/current/push-registration/${provider}/${environment}`,
        { ...init, method: 'DELETE' },
      );
    },

    rotateCredential(
      refreshToken: string,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<DeviceSession> {
      return client.request('/api/v1/devices/credentials/rotate', {
        ...init,
        method: 'POST',
        headers: explicitDeviceRefreshToken(refreshToken),
        decode: decodeDeviceSession,
      });
    },

    revoke(deviceId: string, init: Pick<RequestInit, 'signal'> = {}): Promise<Device> {
      return client.request(`/api/v1/devices/${explicitDeviceId(deviceId)}/revoke`, {
        ...init,
        method: 'POST',
        decode: decodeDevice,
      });
    },
  };
}
