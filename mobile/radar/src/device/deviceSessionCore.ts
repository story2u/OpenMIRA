import type { SessionState } from '../auth/sessionCore';

export interface SecureValueStore {
  clear(): Promise<void>;
  read(): Promise<string | null>;
  write(value: string): Promise<void>;
}

export interface DeviceCredentialStores {
  access: SecureValueStore;
  deviceId: SecureValueStore;
  refresh: SecureValueStore;
}

export interface DeviceSessionCredentials {
  accessToken: string;
  deviceId: string;
  refreshToken: string;
}

export class DeviceCredentialStorageError extends Error {
  constructor() {
    super('Device credentials could not be stored securely.');
    this.name = 'DeviceCredentialStorageError';
  }
}

export class InvalidStoredDeviceCredentialError extends Error {
  constructor() {
    super('Stored device credential is invalid.');
    this.name = 'InvalidStoredDeviceCredentialError';
  }
}

const refreshTokenPattern = /^radar_device_1_[A-Za-z0-9_-]{43}$/;
const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

export function validateDeviceRefreshToken(value: string) {
  if (!refreshTokenPattern.test(value)) throw new InvalidStoredDeviceCredentialError();
  return value;
}

export function validateStoredDeviceId(value: string) {
  if (!uuidPattern.test(value)) throw new InvalidStoredDeviceCredentialError();
  return value;
}

function validateCredentials(credentials: DeviceSessionCredentials) {
  if (
    credentials.accessToken.length < 16
    || credentials.accessToken.length > 16_384
    || /[\r\n]/.test(credentials.accessToken)
  ) {
    throw new DeviceCredentialStorageError();
  }
  try {
    validateStoredDeviceId(credentials.deviceId);
    validateDeviceRefreshToken(credentials.refreshToken);
  } catch {
    throw new DeviceCredentialStorageError();
  }
}

async function restoreValue(store: SecureValueStore, previous: string | null) {
  if (previous === null) {
    await store.clear();
    return;
  }
  await store.write(previous);
  if ((await store.read()) !== previous) throw new DeviceCredentialStorageError();
}

export async function persistDeviceSessionCredentials(
  stores: DeviceCredentialStores,
  credentials: DeviceSessionCredentials,
) {
  validateCredentials(credentials);
  const [previousAccess, previousDeviceId, previousRefresh] = await Promise.all([
    stores.access.read(),
    stores.deviceId.read(),
    stores.refresh.read(),
  ]);

  try {
    await Promise.all([
      stores.deviceId.write(credentials.deviceId),
      stores.refresh.write(credentials.refreshToken),
    ]);
    const [deviceId, refreshToken] = await Promise.all([
      stores.deviceId.read(),
      stores.refresh.read(),
    ]);
    if (deviceId !== credentials.deviceId || refreshToken !== credentials.refreshToken) {
      throw new DeviceCredentialStorageError();
    }
    await stores.access.write(credentials.accessToken);
    if ((await stores.access.read()) !== credentials.accessToken) {
      throw new DeviceCredentialStorageError();
    }
  } catch {
    const restored = await Promise.allSettled([
      restoreValue(stores.access, previousAccess),
      restoreValue(stores.deviceId, previousDeviceId),
      restoreValue(stores.refresh, previousRefresh),
    ]);
    if (restored.some((result) => result.status === 'rejected')) {
      throw new DeviceCredentialStorageError();
    }
    throw new DeviceCredentialStorageError();
  }
}

export interface DeviceAwareSessionDependencies<User> {
  clearCredentials(): Promise<void>;
  isUnauthorized(error: unknown): boolean;
  loadUser(accessToken: string): Promise<User>;
  migrateAccessToken(): Promise<unknown>;
  readAccessToken(): Promise<string | null>;
  readRefreshToken(): Promise<string | null>;
  rotateSession(refreshToken: string): Promise<User>;
}

export async function restoreDeviceAwareSession<User>(
  dependencies: DeviceAwareSessionDependencies<User>,
): Promise<SessionState<User>> {
  try {
    await dependencies.migrateAccessToken();
  } catch {
    return { status: 'requires-login', reason: 'migration-failed' };
  }

  const accessToken = await dependencies.readAccessToken();
  if (accessToken) {
    try {
      return { status: 'authenticated', user: await dependencies.loadUser(accessToken) };
    } catch (error) {
      if (!dependencies.isUnauthorized(error)) throw error;
    }
  }

  const refreshToken = await dependencies.readRefreshToken();
  if (!refreshToken) {
    if (!accessToken) return { status: 'anonymous' };
    await dependencies.clearCredentials();
    return { status: 'requires-login', reason: 'expired' };
  }

  try {
    return {
      status: 'authenticated',
      user: await dependencies.rotateSession(validateDeviceRefreshToken(refreshToken)),
    };
  } catch (error) {
    if (!dependencies.isUnauthorized(error)) throw error;
    await dependencies.clearCredentials();
    return { status: 'requires-login', reason: 'expired' };
  }
}
