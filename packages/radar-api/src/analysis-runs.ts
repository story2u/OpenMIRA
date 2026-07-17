import { AnalysisSchema } from '@story2u/radar-agent/analysis';
import type {
  AnalysisRun,
  AnalysisRunClaim,
  AnalysisRunClaimRequest,
  AnalysisRunCompleteRequest,
  AnalysisRunFailRequest,
  AnalysisRunHeartbeatRequest,
  AnalysisRunLinks,
} from '@story2u/radar-contracts/analysis-runs';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const failureCodePattern = '^[a-z][a-z0-9_.-]{0,63}$';
const DateTimeSchema = Type.String({ minLength: 20, maxLength: 64 });
const NullableDateTimeSchema = Type.Union([DateTimeSchema, Type.Null()]);
const NullableString = (maximum: number) => Type.Union([
  Type.String({ maxLength: maximum }),
  Type.Null(),
]);

const AnalysisRunStatusSchema = Type.Union([
  Type.Literal('claimed'),
  Type.Literal('running'),
  Type.Literal('completed'),
  Type.Literal('failed'),
  Type.Literal('expired'),
]);

const AnalysisRunExecutorSchema = Type.Union([
  Type.Literal('device'),
  Type.Literal('server'),
]);

const AnalysisRunModeSchema = Type.Union([
  Type.Literal('primary'),
  Type.Literal('shadow'),
]);

export const AnalysisRunSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    messageId: Type.String({ pattern: uuidPattern }),
    deviceId: Type.String({ pattern: uuidPattern }),
    status: AnalysisRunStatusSchema,
    executedBy: AnalysisRunExecutorSchema,
    mode: AnalysisRunModeSchema,
    runtimeVersion: Type.String({ minLength: 1, maxLength: 64 }),
    schemaVersion: Type.Integer({ minimum: 1, maximum: 100 }),
    modelAlias: Type.String({ minLength: 1, maxLength: 64 }),
    policyVersion: Type.String({ minLength: 1, maxLength: 64 }),
    sourceMessageVersion: Type.Integer({ minimum: 1 }),
    lockVersion: Type.Integer({ minimum: 1 }),
    leaseExpiresAt: DateTimeSchema,
    claimedAt: DateTimeSchema,
    heartbeatAt: NullableDateTimeSchema,
    completedAt: NullableDateTimeSchema,
    failedAt: NullableDateTimeSchema,
    expiredAt: NullableDateTimeSchema,
    failureCode: NullableString(64),
    shadowMatch: Type.Union([Type.Boolean(), Type.Null()]),
    shadowDifferenceCount: Type.Union([
      Type.Integer({ minimum: 0 }),
      Type.Null(),
    ]),
  },
  { additionalProperties: false },
);

export const AnalysisRunInputSchema = Type.Object(
  {
    messageId: Type.String({ pattern: uuidPattern }),
    sourceMessageVersion: Type.Integer({ minimum: 1 }),
    channel: Type.Union([Type.Literal('telegram'), Type.Literal('wecom')]),
    senderDisplayName: NullableString(256),
    sourceType: Type.String({ minLength: 1, maxLength: 32 }),
    groupName: NullableString(256),
    text: Type.String({ maxLength: 20_000 }),
    links: Type.Array(Type.String({ minLength: 1, maxLength: 2048 }), { maxItems: 10 }),
  },
  { additionalProperties: false },
);

export const AnalysisRunClaimSchema = Type.Object(
  {
    ...AnalysisRunSchema.properties,
    runToken: Type.String({ minLength: 16, maxLength: 16_384 }),
    input: AnalysisRunInputSchema,
  },
  { additionalProperties: false },
);

const LinkInspectionSchema = Type.Object(
  {
    url: Type.String({ minLength: 1, maxLength: 2048 }),
    final_url: NullableString(2048),
    status: Type.Union([
      Type.Literal('unverified'),
      Type.Literal('verifying'),
      Type.Literal('safe'),
      Type.Literal('suspicious'),
      Type.Literal('malicious'),
    ]),
    http_status: Type.Union([Type.Integer({ minimum: 100, maximum: 599 }), Type.Null()]),
    content_type: NullableString(128),
    title: NullableString(500),
    text: Type.String({ maxLength: 20_000 }),
    emails: Type.Array(Type.String({ minLength: 1, maxLength: 320 }), { maxItems: 20 }),
    risk_reasons: Type.Array(Type.String({ minLength: 1, maxLength: 500 }), { maxItems: 20 }),
  },
  { additionalProperties: false },
);

export const AnalysisRunLinksSchema = Type.Object(
  {
    runId: Type.String({ pattern: uuidPattern }),
    sourceMessageVersion: Type.Integer({ minimum: 1 }),
    fetchedAt: DateTimeSchema,
    evidence: Type.Array(LinkInspectionSchema, { maxItems: 10 }),
  },
  { additionalProperties: false },
);

const AnalysisRunShadowClaimSchema = Type.Object(
  { claim: Type.Union([AnalysisRunClaimSchema, Type.Null()]) },
  { additionalProperties: false },
);

const AnalysisRunClaimRequestSchema = Type.Object(
  { messageId: Type.String({ pattern: uuidPattern }) },
  { additionalProperties: false },
);
const AnalysisRunHeartbeatRequestSchema = Type.Object(
  { expectedLockVersion: Type.Integer({ minimum: 1 }) },
  { additionalProperties: false },
);
const AnalysisRunCompleteRequestSchema = Type.Object(
  {
    expectedLockVersion: Type.Integer({ minimum: 1 }),
    result: AnalysisSchema,
  },
  { additionalProperties: false },
);
const AnalysisRunFailRequestSchema = Type.Object(
  {
    expectedLockVersion: Type.Integer({ minimum: 1 }),
    failureCode: Type.String({ pattern: failureCodePattern }),
  },
  { additionalProperties: false },
);

const decodeRun: ResponseDecoder<AnalysisRun> = typeboxDecoder(AnalysisRunSchema);
const decodeClaim: ResponseDecoder<AnalysisRunClaim> = typeboxDecoder(AnalysisRunClaimSchema);
const decodeLinks: ResponseDecoder<AnalysisRunLinks> = typeboxDecoder(AnalysisRunLinksSchema);
const decodeShadowClaim: ResponseDecoder<{ claim: AnalysisRunClaim | null }> = typeboxDecoder(
  AnalysisRunShadowClaimSchema,
);
const parseClaim = typeboxDecoder(AnalysisRunClaimRequestSchema);
const parseHeartbeat = typeboxDecoder(AnalysisRunHeartbeatRequestSchema);
const parseComplete = typeboxDecoder(AnalysisRunCompleteRequestSchema);
const parseFail = typeboxDecoder(AnalysisRunFailRequestSchema);

function explicitRunId(value: string) {
  if (!(new RegExp(uuidPattern)).test(value)) throw new Error('Invalid analysis run ID');
  return value;
}

function runAuthorization(token: string) {
  if (token.length < 16 || token.length > 16_384) throw new Error('Invalid analysis run token');
  return { Authorization: `Bearer ${token}` };
}

export function createAnalysisRunsApi(client: RadarApiClient) {
  return {
    claim(
      input: AnalysisRunClaimRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRunClaim> {
      const payload = parseClaim(input) as AnalysisRunClaimRequest;
      return client.request('/api/v1/agent/runs/claim', {
        ...init,
        method: 'POST',
        body: JSON.stringify(payload),
        decode: decodeClaim,
      });
    },

    claimShadow(
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRunClaim | null> {
      return client.request('/api/v1/agent/runs/claim-shadow', {
        ...init,
        method: 'POST',
        decode: decodeShadowClaim,
      }).then((result) => result.claim);
    },

    claimNext(
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRunClaim | null> {
      return client.request('/api/v1/agent/runs/claim-next', {
        ...init,
        method: 'POST',
        decode: decodeShadowClaim,
      }).then((result) => result.claim);
    },

    heartbeat(
      runId: string,
      runToken: string,
      input: AnalysisRunHeartbeatRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRun> {
      const payload = parseHeartbeat(input) as AnalysisRunHeartbeatRequest;
      return client.request(`/api/v1/agent/runs/${explicitRunId(runId)}/heartbeat`, {
        ...init,
        method: 'POST',
        headers: runAuthorization(runToken),
        body: JSON.stringify(payload),
        decode: decodeRun,
      });
    },

    complete(
      runId: string,
      runToken: string,
      input: AnalysisRunCompleteRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRun> {
      const payload = parseComplete(input) as AnalysisRunCompleteRequest;
      return client.request(`/api/v1/agent/runs/${explicitRunId(runId)}/complete`, {
        ...init,
        method: 'POST',
        headers: runAuthorization(runToken),
        body: JSON.stringify(payload),
        decode: decodeRun,
      });
    },

    fail(
      runId: string,
      runToken: string,
      input: AnalysisRunFailRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRun> {
      const payload = parseFail(input) as AnalysisRunFailRequest;
      return client.request(`/api/v1/agent/runs/${explicitRunId(runId)}/fail`, {
        ...init,
        method: 'POST',
        headers: runAuthorization(runToken),
        body: JSON.stringify(payload),
        decode: decodeRun,
      });
    },

    inspectLinks(
      runId: string,
      runToken: string,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRunLinks> {
      return client.request(`/api/v1/agent/runs/${explicitRunId(runId)}/links/inspect`, {
        ...init,
        method: 'POST',
        headers: runAuthorization(runToken),
        decode: decodeLinks,
      });
    },

    expire(
      runId: string,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AnalysisRun> {
      return client.request(`/api/v1/agent/runs/${explicitRunId(runId)}/expire`, {
        ...init,
        method: 'POST',
        decode: decodeRun,
      });
    },
  };
}
