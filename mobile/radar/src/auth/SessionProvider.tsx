import { RadarApiError } from '@story2u/radar-api/client';
import type { AuthUser } from '@story2u/radar-contracts/auth';
import type { ClientCapabilities } from '@story2u/radar-contracts/devices';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';
import { useRouter } from 'expo-router';
import * as Network from 'expo-network';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { AppState } from 'react-native';

import {
  loginWithNativeToken as requestNativeLogin,
  loginWithPassword as requestPasswordLogin,
  readClientCapabilities,
} from '../api/client';
import { disabledCapabilities } from '../capabilities/clientCapabilities';
import {
  readStoredSyncCapability,
  writeStoredSyncCapability,
} from '../capabilities/capabilityStore';
import {
  pauseInstalledDeviceAnalysis,
  recoverInstalledDeviceAnalysis,
} from '../agent/analysisRunCoordinator';
import { MobileConfigurationError } from '../config/apiBaseUrl';
import { getMobileApiBaseUrl } from '../config/mobileApiConfig';
import type { MessageKey } from '../i18n/catalog';
import type { Translator } from '../i18n/core';
import { logEvent } from '../logging/redactedLogger';
import {
  clearInstalledPushOwner,
  clearInstalledPushPreference,
  disableInstalledPush,
  enableInstalledPush,
  restoreInstalledPush,
  subscribeToInstalledPush,
} from '../push/nativePush';
import type { PushEnrollmentState } from '../push/pushCore';
import { clearLocalUserData, initializeRadarDatabase } from '../storage/database';
import type { CommandOutboxSummary } from '../sync/commandOutbox';
import {
  queueInstalledOpportunityStatus,
  readInstalledCommandSummary,
  dismissInstalledTerminalCommand,
  synchronizeInstalledOwner,
  synchronizeInstalledOwnerForCursorHint,
} from '../sync/installedSync';
import { isOfflineReadFailure } from '../sync/offlineFallback';
import { createNetworkRecoveryDetector } from '../sync/networkRecovery';
import {
  ensureDeviceRegistration,
  revokeInstalledDevice,
} from '../device/deviceSession';
import {
  currentDeviceIdStore,
  deviceRefreshTokenStore,
} from '../device/deviceSessionStorage';
import { legacyTokenStore } from './legacyToken';
import { currentTokenStore } from './migrateInstalledToken';
import { restoreInstalledSession } from './restoreInstalledSession';
import type { NativeIdentityProvider } from './nativeIdentityCore';
import {
  endSession,
  clearSessionTokens,
  persistReplacingLegacyToken,
  type SessionState,
} from './sessionCore';

export type InstalledSessionState =
  | { status: 'loading' }
  | SessionState<AuthUser>
  | { status: 'unavailable'; reason: SessionUnavailableReason };

export type SessionUnavailableReason = 'configuration' | 'network' | 'server';

interface SessionContextValue {
  capabilities: ClientCapabilities;
  commandSummary: CommandOutboxSummary;
  pushEnrollmentState: PushEnrollmentState;
  state: InstalledSessionState;
  loginWithNativeToken(provider: NativeIdentityProvider, idToken: string): Promise<void>;
  loginWithPassword(email: string, password: string): Promise<void>;
  logout(): Promise<void>;
  expireSession(): Promise<void>;
  dismissCommand(commandId: string): Promise<void>;
  queueOpportunityStatus(
    opportunityId: string,
    status: InternalOpportunityStatus,
  ): Promise<void>;
  enablePush(): Promise<void>;
  disablePush(): Promise<void>;
  synchronize(): Promise<void>;
  retry(): void;
}

const SessionContext = createContext<SessionContextValue | null>(null);
const emptyCommandSummary: CommandOutboxSummary = {
  pendingCount: 0,
  conflictCount: 0,
  failedCount: 0,
  attentionCommands: [],
};

function safeBootstrapError(error: unknown): SessionUnavailableReason {
  if (error instanceof MobileConfigurationError) return 'configuration';
  if (error instanceof RadarApiError && error.status >= 500) return 'server';
  return 'network';
}

const sessionUnavailableKeys: Record<SessionUnavailableReason, MessageKey> = {
  configuration: 'auth.session.configurationUnavailable',
  network: 'auth.session.networkUnavailable',
  server: 'auth.session.serverUnavailable',
};

export function sessionUnavailableMessage(reason: SessionUnavailableReason, t: Translator) {
  return t(sessionUnavailableKeys[reason]);
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [state, setState] = useState<InstalledSessionState>({ status: 'loading' });
  const [capabilities, setCapabilities] = useState<ClientCapabilities>(
    () => ({ ...disabledCapabilities }),
  );
  const [commandSummary, setCommandSummary] = useState<CommandOutboxSummary>(
    emptyCommandSummary,
  );
  const [pushEnrollmentState, setPushEnrollmentState] = useState<PushEnrollmentState>('disabled');
  const apiBaseUrl = useRef<string | null>(null);
  const bootstrapGeneration = useRef(0);
  const enrollmentController = useRef<AbortController | null>(null);

  const beginDeviceEnrollment = useCallback((baseUrl: string, ownerId: string) => {
    enrollmentController.current?.abort();
    const controller = new AbortController();
    enrollmentController.current = controller;
    void (async () => {
      try {
        await ensureDeviceRegistration(baseUrl, controller.signal);
        if (controller.signal.aborted) return;
        const database = await initializeRadarDatabase();
        setCommandSummary(await readInstalledCommandSummary(ownerId));
        let resolved: ClientCapabilities;
        try {
          resolved = await readClientCapabilities(baseUrl, controller.signal);
          await writeStoredSyncCapability(database, ownerId, resolved.syncAvailable);
        } catch (error) {
          if (controller.signal.aborted) return;
          if (!isOfflineReadFailure(error)) throw error;
          const storedSyncAvailable = await readStoredSyncCapability(database, ownerId);
          resolved = {
            ...disabledCapabilities,
            rnClientSupported: storedSyncAvailable,
            syncAvailable: storedSyncAvailable,
          };
          logEvent('device.capability_refresh_failed', {
            errorClass: error instanceof Error ? error.name : 'UnknownError',
          });
        }
        if (controller.signal.aborted) return;
        setCapabilities(resolved);
        void recoverInstalledDeviceAnalysis(
          baseUrl,
          ownerId,
          resolved.deviceAgentAvailable,
        ).catch((error: unknown) => {
          logEvent('agent.recovery_failed', {
            errorClass: error instanceof Error ? error.name : 'UnknownError',
          });
        });
        if (!resolved.syncAvailable) return;
        try {
          const result = await synchronizeInstalledOwner(baseUrl, ownerId, controller.signal);
          setCommandSummary(result.commands);
        } catch (error) {
          if (controller.signal.aborted) return;
          logEvent('sync.foreground_failed', {
            errorClass: error instanceof Error ? error.name : 'UnknownError',
          });
        }
      } catch (error) {
        if (controller.signal.aborted) return;
        logEvent('device.enrollment_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
        });
      }
    })();
  }, []);

  const bootstrap = useCallback(async () => {
    const generation = ++bootstrapGeneration.current;
    setState({ status: 'loading' });
    setCapabilities({ ...disabledCapabilities });
    setCommandSummary(emptyCommandSummary);
    setPushEnrollmentState('disabled');
    try {
      const baseUrl = getMobileApiBaseUrl();
      apiBaseUrl.current = baseUrl;
      const [, restored] = await Promise.all([
        initializeRadarDatabase(),
        restoreInstalledSession(baseUrl),
      ]);
      if (bootstrapGeneration.current === generation) {
        setState(restored);
        if (restored.status === 'authenticated') {
          beginDeviceEnrollment(baseUrl, restored.user.id);
        }
      }
    } catch (error) {
      logEvent('session.bootstrap_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
      });
      if (bootstrapGeneration.current === generation) {
        setState({ status: 'unavailable', reason: safeBootstrapError(error) });
      }
    }
  }, [beginDeviceEnrollment]);

  useEffect(() => {
    void bootstrap();
    return () => {
      bootstrapGeneration.current += 1;
      enrollmentController.current?.abort();
      pauseInstalledDeviceAnalysis();
    };
  }, [bootstrap]);

  useEffect(() => {
    if (state.status !== 'authenticated') return undefined;
    const subscription = AppState.addEventListener('change', (nextState) => {
      if (nextState !== 'active') {
        pauseInstalledDeviceAnalysis();
        return;
      }
      const baseUrl = apiBaseUrl.current;
      if (baseUrl) beginDeviceEnrollment(baseUrl, state.user.id);
    });
    return () => subscription.remove();
  }, [beginDeviceEnrollment, state]);

  useEffect(() => {
    if (state.status !== 'authenticated') return undefined;
    const detector = createNetworkRecoveryDetector();
    let eventObserved = false;
    let disposed = false;
    const subscription = Network.addNetworkStateListener((networkState) => {
      if (disposed) return;
      eventObserved = true;
      if (detector.observe(networkState)) {
        const baseUrl = apiBaseUrl.current;
        if (baseUrl) beginDeviceEnrollment(baseUrl, state.user.id);
      }
    });
    void Network.getNetworkStateAsync()
      .then((networkState) => {
        if (!disposed && !eventObserved) detector.seed(networkState);
      })
      .catch((error: unknown) => {
        if (disposed) return;
        logEvent('network.initial_state_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
        });
      });
    return () => {
      disposed = true;
      subscription.remove();
    };
  }, [beginDeviceEnrollment, state]);

  const loginWithPassword = useCallback(async (email: string, password: string) => {
    const baseUrl = apiBaseUrl.current;
    if (!baseUrl) throw new MobileConfigurationError('Sign-in service is not configured.');

    const response = await requestPasswordLogin(baseUrl, { email, password });
    await persistReplacingLegacyToken(currentTokenStore, legacyTokenStore, response.accessToken);
    setCapabilities({ ...disabledCapabilities });
    setCommandSummary(emptyCommandSummary);
    setPushEnrollmentState('disabled');
    setState({ status: 'authenticated', user: response.user });
    beginDeviceEnrollment(baseUrl, response.user.id);
  }, [beginDeviceEnrollment]);

  const loginWithNativeToken = useCallback(async (
    provider: NativeIdentityProvider,
    idToken: string,
  ) => {
    const baseUrl = apiBaseUrl.current;
    if (!baseUrl) throw new MobileConfigurationError('Sign-in service is not configured.');

    const response = await requestNativeLogin(baseUrl, provider, { idToken });
    await persistReplacingLegacyToken(currentTokenStore, legacyTokenStore, response.accessToken);
    setCapabilities({ ...disabledCapabilities });
    setCommandSummary(emptyCommandSummary);
    setPushEnrollmentState('disabled');
    setState({ status: 'authenticated', user: response.user });
    beginDeviceEnrollment(baseUrl, response.user.id);
  }, [beginDeviceEnrollment]);

  const logout = useCallback(async () => {
    if (state.status !== 'authenticated') return;
    enrollmentController.current?.abort();
    pauseInstalledDeviceAnalysis();
    const result = await endSession({
      clearToken: async () => {
        const baseUrl = apiBaseUrl.current;
        if (baseUrl) {
          const controller = new AbortController();
          const timeout = setTimeout(() => controller.abort(), 5_000);
          try {
            await revokeInstalledDevice(baseUrl, controller.signal);
          } catch (error) {
            logEvent('device.revoke_on_logout_failed', {
              errorClass: error instanceof Error ? error.name : 'UnknownError',
            });
          } finally {
            clearTimeout(timeout);
          }
        }
        await clearSessionTokens(
          currentTokenStore,
          legacyTokenStore,
          deviceRefreshTokenStore,
          currentDeviceIdStore,
        );
        await clearInstalledPushPreference();
      },
      clearLocalData: () => clearLocalUserData(state.user.id),
    });
    setCapabilities({ ...disabledCapabilities });
    setCommandSummary(emptyCommandSummary);
    setPushEnrollmentState('disabled');
    setState({ status: 'anonymous' });
    if (!result.localDataCleared) {
      logEvent('session.local_cache_cleanup_deferred', { reason: 'database-unavailable' });
    }
  }, [state]);

  const expireSession = useCallback(async () => {
    enrollmentController.current?.abort();
    pauseInstalledDeviceAnalysis();
    try {
      await clearSessionTokens(
        currentTokenStore,
        legacyTokenStore,
        deviceRefreshTokenStore,
        currentDeviceIdStore,
      );
      await clearInstalledPushOwner();
    } catch (error) {
      logEvent('session.expired_token_cleanup_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
      });
    } finally {
      setCapabilities({ ...disabledCapabilities });
      setCommandSummary(emptyCommandSummary);
      setPushEnrollmentState('disabled');
      setState({ status: 'requires-login', reason: 'expired' });
    }
  }, []);

  const synchronize = useCallback(async () => {
    if (state.status !== 'authenticated' || !capabilities.syncAvailable) return;
    const baseUrl = apiBaseUrl.current;
    if (!baseUrl) return;
    try {
      const result = await synchronizeInstalledOwner(baseUrl, state.user.id);
      setCommandSummary(result.commands);
      void recoverInstalledDeviceAnalysis(
        baseUrl,
        state.user.id,
        capabilities.deviceAgentAvailable,
      ).catch((error: unknown) => {
        logEvent('agent.recovery_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
        });
      });
    } catch (error) {
      logEvent('sync.manual_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
      });
    }
  }, [capabilities.deviceAgentAvailable, capabilities.syncAvailable, state]);

  useEffect(() => {
    if (
      state.status !== 'authenticated'
      || !capabilities.pushAvailable
      || !capabilities.syncAvailable
    ) {
      setPushEnrollmentState('disabled');
      return undefined;
    }
    const baseUrl = apiBaseUrl.current;
    if (!baseUrl) return undefined;
    const controller = new AbortController();
    void restoreInstalledPush(baseUrl, state.user.id, controller.signal)
      .then((resolved) => {
        if (!controller.signal.aborted) setPushEnrollmentState(resolved);
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setPushEnrollmentState('error');
        logEvent('push.restore_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
        });
      });
    const unsubscribe = subscribeToInstalledPush({
      baseUrl,
      ownerId: state.user.id,
      onCursorHint: (cursor, interacted) => {
        void synchronizeInstalledOwnerForCursorHint(baseUrl, state.user.id, cursor)
          .then((outcome) => {
            if (outcome.status === 'synchronized') {
              setCommandSummary(outcome.result.commands);
              void recoverInstalledDeviceAnalysis(
                baseUrl,
                state.user.id,
                capabilities.deviceAgentAvailable,
              ).catch((error: unknown) => {
                logEvent('agent.recovery_failed', {
                  errorClass: error instanceof Error ? error.name : 'UnknownError',
                });
              });
            }
          })
          .catch((error: unknown) => {
            logEvent('sync.push_hint_failed', {
              errorClass: error instanceof Error ? error.name : 'UnknownError',
            });
          });
        if (interacted) router.replace('/');
      },
    });
    return () => {
      controller.abort();
      unsubscribe();
    };
  }, [
    capabilities.deviceAgentAvailable,
    capabilities.pushAvailable,
    capabilities.syncAvailable,
    router,
    state,
  ]);

  const enablePush = useCallback(async () => {
    if (state.status !== 'authenticated' || !capabilities.pushAvailable) return;
    const baseUrl = apiBaseUrl.current;
    if (!baseUrl) return;
    setPushEnrollmentState('registering');
    try {
      setPushEnrollmentState(await enableInstalledPush(baseUrl, state.user.id));
    } catch (error) {
      setPushEnrollmentState('error');
      logEvent('push.enable_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
      });
    }
  }, [capabilities.pushAvailable, state]);

  const disablePush = useCallback(async () => {
    const baseUrl = apiBaseUrl.current;
    if (!baseUrl) return;
    setPushEnrollmentState('registering');
    try {
      await disableInstalledPush(baseUrl);
      setPushEnrollmentState('disabled');
    } catch (error) {
      setPushEnrollmentState('error');
      logEvent('push.disable_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
      });
    }
  }, []);

  const queueOpportunityStatus = useCallback(async (
    opportunityId: string,
    status: InternalOpportunityStatus,
  ) => {
    if (state.status !== 'authenticated' || !capabilities.syncAvailable) {
      throw new Error('Offline status queue is unavailable.');
    }
    const summary = await queueInstalledOpportunityStatus(
      state.user.id,
      opportunityId,
      status,
    );
    setCommandSummary(summary);
  }, [capabilities.syncAvailable, state]);

  const dismissCommand = useCallback(async (commandId: string) => {
    if (state.status !== 'authenticated') return;
    try {
      const summary = await dismissInstalledTerminalCommand(state.user.id, commandId);
      setCommandSummary(summary);
    } catch (error) {
      logEvent('sync.command_dismiss_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
      });
    }
  }, [state]);

  const value = useMemo<SessionContextValue>(
    () => ({
      capabilities,
      commandSummary,
      disablePush,
      dismissCommand,
      enablePush,
      pushEnrollmentState,
      state,
      loginWithNativeToken,
      loginWithPassword,
      logout,
      expireSession,
      queueOpportunityStatus,
      synchronize,
      retry: () => void bootstrap(),
    }),
    [
      bootstrap,
      capabilities,
      commandSummary,
      disablePush,
      dismissCommand,
      enablePush,
      expireSession,
      loginWithNativeToken,
      loginWithPassword,
      logout,
      queueOpportunityStatus,
      pushEnrollmentState,
      state,
      synchronize,
    ],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession() {
  const context = useContext(SessionContext);
  if (!context) throw new Error('useSession must be used within SessionProvider');
  return context;
}
