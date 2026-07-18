import { createAuthApi } from '@story2u/radar-api/auth';
import { createAnalysisRunsApi } from '@story2u/radar-api/analysis-runs';
import { createRadarApiClient } from '@story2u/radar-api/client';
import { createDevicesApi } from '@story2u/radar-api/devices';
import { createMessagesApi } from '@story2u/radar-api/messages';
import { createInteractiveAgentApi } from '@story2u/radar-api/interactive-agent';
import { createOpportunityActionsApi } from '@story2u/radar-api/opportunity-actions';
import { createOpportunitiesApi } from '@story2u/radar-api/opportunities';
import { createSettingsApi } from '@story2u/radar-api/settings';
import { createSubscriptionsApi } from '@story2u/radar-api/subscriptions';
import { createSyncApi } from '@story2u/radar-api/sync';
import { createSignalAppetiteSyncApi } from '@story2u/radar-api/signal-appetite-sync';
import { createTemplatesApi } from '@story2u/radar-api/templates';
import { createTelegramApi } from '@story2u/radar-api/telegram';
import type {
  AnalysisRun,
  AnalysisRunClaim,
  AnalysisRunCompleteRequest,
  AnalysisRunLinks,
} from '@story2u/radar-contracts/analysis-runs';
import type {
  InteractiveAgentApprovalDecision,
  InteractiveAgentApprovalDecisionRequest,
  InteractiveAgentApprovedSend,
  InteractiveAgentApprovedSendRequest,
  InteractiveAgentTurn,
  InteractiveAgentTurnClaim,
} from '@story2u/radar-contracts/interactive-agent';
import type {
  AuthToken,
  AuthUser,
  NativeLoginRequest,
  PasswordLoginRequest,
} from '@story2u/radar-contracts/auth';
import type { OAuthProvider } from '@story2u/radar-api/auth';
import type { MessagePage } from '@story2u/radar-contracts/messages';
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
import type {
  InternalOpportunityStatus,
  ManualReplyResult,
} from '@story2u/radar-contracts/opportunity-actions';
import type {
  Dashboard,
  DashboardQuery,
  OpportunityDetail,
} from '@story2u/radar-contracts/opportunities';
import type { ReplyTemplate } from '@story2u/radar-contracts/templates';
import type {
  DetectionSettings,
  DetectionSettingsUpdate,
  NotificationSettings,
  NotificationSettingsUpdate,
  SettingsBundle,
  WorkSchedule,
  WorkScheduleUpdate,
} from '@story2u/radar-contracts/settings';
import type {
  SubscriptionCatalogPlan,
  SubscriptionManagement,
  SubscriptionManagementClient,
  SubscriptionUsage,
} from '@story2u/radar-contracts/subscriptions';
import type {
  SyncAck,
  SyncAckRequest,
  SyncBootstrap,
  SyncBootstrapQuery,
  SyncChanges,
  SyncChangesQuery,
} from '@story2u/radar-contracts/sync';
import type {
  SignalAppetiteEventsAppend,
  SignalAppetiteEventsAppendRequest,
  SignalAppetiteEventsPage,
  SignalAppetiteEventsQuery,
} from '@story2u/radar-contracts/signal-appetite-sync';
import type {
  TelegramConnection,
  TelegramConnectionHealth,
} from '@story2u/radar-contracts/telegram';
import { fetch as expoFetch } from 'expo/fetch';

import { currentTokenStore } from '../auth/migrateInstalledToken';

export function createMobileApiClient(
  baseUrl: string,
  getAccessToken: () => Promise<string | null> | string | null = currentTokenStore.read,
) {
  return createRadarApiClient({
    baseUrl,
    fetch: expoFetch,
    getAccessToken,
  });
}

export function readAuthenticatedUser(baseUrl: string, accessToken: string): Promise<AuthUser> {
  return createAuthApi(createMobileApiClient(baseUrl, () => null)).getCurrentUser(accessToken);
}

export function loginWithPassword(
  baseUrl: string,
  payload: PasswordLoginRequest,
): Promise<AuthToken> {
  return createAuthApi(createMobileApiClient(baseUrl, () => null)).loginWithPassword(payload);
}

export function loginWithNativeToken(
  baseUrl: string,
  provider: OAuthProvider,
  payload: NativeLoginRequest,
): Promise<AuthToken> {
  return createAuthApi(createMobileApiClient(baseUrl, () => null))
    .loginWithNativeToken(provider, payload);
}

export function registerDevice(
  baseUrl: string,
  payload: DeviceRegistrationRequest,
  signal?: AbortSignal,
): Promise<DeviceSession> {
  return createDevicesApi(createMobileApiClient(baseUrl)).register(payload, { signal });
}

export function rotateDeviceCredential(
  baseUrl: string,
  refreshToken: string,
): Promise<DeviceSession> {
  return createDevicesApi(createMobileApiClient(baseUrl, () => null))
    .rotateCredential(refreshToken);
}

export function revokeDevice(
  baseUrl: string,
  deviceId: string,
  signal?: AbortSignal,
): Promise<Device> {
  return createDevicesApi(createMobileApiClient(baseUrl)).revoke(deviceId, { signal });
}

export function readClientCapabilities(
  baseUrl: string,
  signal?: AbortSignal,
): Promise<ClientCapabilities> {
  return createDevicesApi(createMobileApiClient(baseUrl)).capabilities({ signal });
}

export function claimMessageAnalysisRun(
  baseUrl: string,
  messageId: string,
  signal?: AbortSignal,
): Promise<AnalysisRunClaim> {
  return createAnalysisRunsApi(createMobileApiClient(baseUrl)).claim(
    { messageId },
    { signal },
  );
}

export function claimShadowMessageAnalysisRun(
  baseUrl: string,
  signal?: AbortSignal,
): Promise<AnalysisRunClaim | null> {
  return createAnalysisRunsApi(createMobileApiClient(baseUrl)).claimShadow({ signal });
}

export function claimNextMessageAnalysisRun(
  baseUrl: string,
  signal?: AbortSignal,
): Promise<AnalysisRunClaim | null> {
  return createAnalysisRunsApi(createMobileApiClient(baseUrl)).claimNext({ signal });
}

function runScopedAnalysisApi(baseUrl: string) {
  return createAnalysisRunsApi(createMobileApiClient(baseUrl, () => null));
}

export function heartbeatMessageAnalysisRun(
  baseUrl: string,
  runId: string,
  runToken: string,
  expectedLockVersion: number,
  signal?: AbortSignal,
): Promise<AnalysisRun> {
  return runScopedAnalysisApi(baseUrl).heartbeat(
    runId,
    runToken,
    { expectedLockVersion },
    { signal },
  );
}

export function inspectMessageAnalysisLinks(
  baseUrl: string,
  runId: string,
  runToken: string,
  signal?: AbortSignal,
): Promise<AnalysisRunLinks> {
  return runScopedAnalysisApi(baseUrl).inspectLinks(runId, runToken, { signal });
}

export function completeMessageAnalysisRun(
  baseUrl: string,
  runId: string,
  runToken: string,
  input: AnalysisRunCompleteRequest,
  signal?: AbortSignal,
): Promise<AnalysisRun> {
  return runScopedAnalysisApi(baseUrl).complete(runId, runToken, input, { signal });
}

export function failMessageAnalysisRun(
  baseUrl: string,
  runId: string,
  runToken: string,
  expectedLockVersion: number,
  failureCode: string,
  signal?: AbortSignal,
): Promise<AnalysisRun> {
  return runScopedAnalysisApi(baseUrl).fail(
    runId,
    runToken,
    { expectedLockVersion, failureCode },
    { signal },
  );
}

export function expireMessageAnalysisRun(
  baseUrl: string,
  runId: string,
  signal?: AbortSignal,
): Promise<AnalysisRun> {
  return createAnalysisRunsApi(createMobileApiClient(baseUrl)).expire(runId, { signal });
}

export function claimInteractiveAgentTurn(
  baseUrl: string,
  localSessionId: string,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<InteractiveAgentTurnClaim> {
  return createInteractiveAgentApi(createMobileApiClient(baseUrl)).claim(
    { localSessionId, idempotencyKey },
    { signal },
  );
}

function runScopedInteractiveAgentApi(baseUrl: string) {
  return createInteractiveAgentApi(createMobileApiClient(baseUrl, () => null));
}

export function heartbeatInteractiveAgentTurn(
  baseUrl: string,
  turnId: string,
  turnToken: string,
  expectedLockVersion: number,
  signal?: AbortSignal,
): Promise<InteractiveAgentTurn> {
  return runScopedInteractiveAgentApi(baseUrl).heartbeat(
    turnId,
    turnToken,
    { expectedLockVersion },
    { signal },
  );
}

export function completeInteractiveAgentTurn(
  baseUrl: string,
  turnId: string,
  turnToken: string,
  expectedLockVersion: number,
  signal?: AbortSignal,
): Promise<InteractiveAgentTurn> {
  return runScopedInteractiveAgentApi(baseUrl).complete(
    turnId,
    turnToken,
    { expectedLockVersion },
    { signal },
  );
}

export function failInteractiveAgentTurn(
  baseUrl: string,
  turnId: string,
  turnToken: string,
  expectedLockVersion: number,
  failureCode: string,
  signal?: AbortSignal,
): Promise<InteractiveAgentTurn> {
  return runScopedInteractiveAgentApi(baseUrl).fail(
    turnId,
    turnToken,
    { expectedLockVersion, failureCode },
    { signal },
  );
}

export function expireInteractiveAgentTurn(
  baseUrl: string,
  turnId: string,
  signal?: AbortSignal,
): Promise<InteractiveAgentTurn> {
  return createInteractiveAgentApi(createMobileApiClient(baseUrl)).expire(turnId, { signal });
}

export function decideInteractiveAgentAction(
  baseUrl: string,
  turnToken: string,
  payload: InteractiveAgentApprovalDecisionRequest,
  signal?: AbortSignal,
): Promise<InteractiveAgentApprovalDecision> {
  return runScopedInteractiveAgentApi(baseUrl).decideAction(turnToken, payload, { signal });
}

export function sendApprovedInteractiveAgentReply(
  baseUrl: string,
  approvalToken: string,
  payload: InteractiveAgentApprovedSendRequest,
  signal?: AbortSignal,
): Promise<InteractiveAgentApprovedSend> {
  return runScopedInteractiveAgentApi(baseUrl)
    .sendApprovedReply(approvalToken, payload, { signal });
}

export function registerPushToken(
  baseUrl: string,
  payload: PushRegistrationRequest,
  signal?: AbortSignal,
): Promise<PushRegistration> {
  return createDevicesApi(createMobileApiClient(baseUrl)).registerPushToken(payload, { signal });
}

export function revokePushToken(
  baseUrl: string,
  provider: PushProvider,
  environment: PushEnvironment,
  signal?: AbortSignal,
): Promise<void> {
  return createDevicesApi(createMobileApiClient(baseUrl))
    .revokePushToken(provider, environment, { signal });
}

export function readSyncBootstrap(
  baseUrl: string,
  query: SyncBootstrapQuery = {},
  signal?: AbortSignal,
): Promise<SyncBootstrap> {
  return createSyncApi(createMobileApiClient(baseUrl)).bootstrap(query, { signal });
}

export function readSyncChanges(
  baseUrl: string,
  query: SyncChangesQuery,
  signal?: AbortSignal,
): Promise<SyncChanges> {
  return createSyncApi(createMobileApiClient(baseUrl)).changes(query, { signal });
}

export function acknowledgeSync(
  baseUrl: string,
  payload: SyncAckRequest,
  signal?: AbortSignal,
): Promise<SyncAck> {
  return createSyncApi(createMobileApiClient(baseUrl)).acknowledge(payload, { signal });
}

export function appendSignalAppetiteEvents(
  baseUrl: string,
  payload: SignalAppetiteEventsAppendRequest,
  signal?: AbortSignal,
): Promise<SignalAppetiteEventsAppend> {
  return createSignalAppetiteSyncApi(createMobileApiClient(baseUrl)).append(payload, { signal });
}

export function readSyncedSignalAppetiteEvents(
  baseUrl: string,
  query: SignalAppetiteEventsQuery,
  signal?: AbortSignal,
): Promise<SignalAppetiteEventsPage> {
  return createSignalAppetiteSyncApi(createMobileApiClient(baseUrl)).list(query, { signal });
}

export function readDashboard(
  baseUrl: string,
  query: DashboardQuery,
  signal?: AbortSignal,
): Promise<Dashboard> {
  return createOpportunitiesApi(createMobileApiClient(baseUrl)).getDashboard(query, { signal });
}

export interface OpportunityDetailBundle {
  detail: OpportunityDetail;
  messages: MessagePage;
}

const MOBILE_MESSAGE_PAGE_SIZE = 20;

function createReadApis(baseUrl: string, accessToken: string | null) {
  const client = createMobileApiClient(baseUrl, () => accessToken);
  return {
    actions: createOpportunityActionsApi(client),
    messages: createMessagesApi(client),
    opportunities: createOpportunitiesApi(client),
    settings: createSettingsApi(client),
    subscriptions: createSubscriptionsApi(client),
    templates: createTemplatesApi(client),
    telegram: createTelegramApi(client),
  };
}

export async function readOpportunityDetailBundle(
  baseUrl: string,
  opportunityId: string,
  signal?: AbortSignal,
): Promise<OpportunityDetailBundle> {
  const accessToken = await currentTokenStore.read();
  const api = createReadApis(baseUrl, accessToken);
  const [detail, messages] = await Promise.all([
    api.opportunities.getById(opportunityId, { signal }),
    api.messages.page(
      { opportunity_id: opportunityId, limit: MOBILE_MESSAGE_PAGE_SIZE, offset: 0 },
      { signal },
    ),
  ]);
  return { detail, messages };
}

export async function readMessagePage(
  baseUrl: string,
  opportunityId: string,
  offset: number,
  signal?: AbortSignal,
): Promise<MessagePage> {
  const accessToken = await currentTokenStore.read();
  return createReadApis(baseUrl, accessToken).messages.page(
    {
      opportunity_id: opportunityId,
      limit: MOBILE_MESSAGE_PAGE_SIZE,
      offset,
    },
    { signal },
  );
}

async function authenticatedApis(baseUrl: string) {
  const accessToken = await currentTokenStore.read();
  return createReadApis(baseUrl, accessToken);
}

export async function sendOpportunityManualReply(
  baseUrl: string,
  opportunityId: string,
  text: string,
  idempotencyKey: string,
): Promise<ManualReplyResult> {
  return (await authenticatedApis(baseUrl)).actions.manualReply(
    opportunityId,
    { text, mark_following: true },
    idempotencyKey,
  );
}

export async function generateOpportunityAIDraft(
  baseUrl: string,
  opportunityId: string,
): Promise<string> {
  return (await authenticatedApis(baseUrl)).actions
    .generateAIDraft(opportunityId)
    .then((result) => result.draft);
}

export async function updateOpportunityStatus(
  baseUrl: string,
  opportunityId: string,
  status: InternalOpportunityStatus,
  options: {
    expectedVersion?: number;
    idempotencyKey?: string;
    signal?: AbortSignal;
  } = {},
): Promise<OpportunityDetail> {
  return (await authenticatedApis(baseUrl)).actions.updateStatus(opportunityId, status, options);
}

export async function claimOpportunity(
  baseUrl: string,
  opportunityId: string,
  signal?: AbortSignal,
): Promise<OpportunityDetail> {
  return (await authenticatedApis(baseUrl)).actions.claim(opportunityId, { signal });
}

export async function readReplyTemplates(baseUrl: string): Promise<ReplyTemplate[]> {
  return (await authenticatedApis(baseUrl)).templates.list();
}

export function readSettings(
  baseUrl: string,
  signal?: AbortSignal,
): Promise<SettingsBundle> {
  return createSettingsApi(createMobileApiClient(baseUrl)).get({ signal });
}

export async function saveDetectionSettings(
  baseUrl: string,
  input: DetectionSettingsUpdate,
): Promise<DetectionSettings> {
  return (await authenticatedApis(baseUrl)).settings.updateDetection(input);
}

export async function saveWorkSchedule(
  baseUrl: string,
  input: WorkScheduleUpdate,
): Promise<WorkSchedule> {
  return (await authenticatedApis(baseUrl)).settings.updateWorkSchedule(input);
}

export async function saveNotificationSettings(
  baseUrl: string,
  input: NotificationSettingsUpdate,
): Promise<NotificationSettings> {
  return (await authenticatedApis(baseUrl)).settings.updateNotifications(input);
}

export interface TelegramOverview {
  health: TelegramConnectionHealth;
  connections: TelegramConnection[];
}

export async function readTelegramOverview(
  baseUrl: string,
  signal?: AbortSignal,
): Promise<TelegramOverview> {
  const accessToken = await currentTokenStore.read();
  const telegram = createReadApis(baseUrl, accessToken).telegram;
  const [health, connections] = await Promise.all([
    telegram.health({ signal }),
    telegram.connections({ signal }),
  ]);
  return { health, connections };
}

export async function setTelegramConnectionEnabled(
  baseUrl: string,
  connectionId: string,
  enabled: boolean,
): Promise<TelegramConnection> {
  return (await authenticatedApis(baseUrl)).telegram.updateConnection(connectionId, enabled);
}

export interface SubscriptionOverview {
  catalog: SubscriptionCatalogPlan[];
  management: SubscriptionManagement;
  usage: SubscriptionUsage;
}

export async function readSubscriptionOverview(
  baseUrl: string,
  managementClient: SubscriptionManagementClient,
  signal?: AbortSignal,
): Promise<SubscriptionOverview> {
  const accessToken = await currentTokenStore.read();
  const subscriptions = createReadApis(baseUrl, accessToken).subscriptions;
  const [usage, catalog, management] = await Promise.all([
    subscriptions.usage({ signal }),
    subscriptions.catalog({ signal }),
    subscriptions.management(managementClient, { signal }),
  ]);
  return { catalog, management, usage };
}

export async function syncSubscriptionOverview(
  baseUrl: string,
  managementClient: SubscriptionManagementClient,
): Promise<Pick<SubscriptionOverview, 'management' | 'usage'>> {
  const subscriptions = (await authenticatedApis(baseUrl)).subscriptions;
  const usage = await subscriptions.sync();
  const management = await subscriptions.management(managementClient);
  return { management, usage };
}
