import type {
  InteractiveAgentApprovalDecision,
  InteractiveAgentApprovalDecisionRequest,
  InteractiveAgentApprovedSend,
  InteractiveAgentApprovedSendRequest,
  InteractiveAgentTurn,
  InteractiveAgentTurnClaim,
  InteractiveAgentTurnClaimRequest,
  InteractiveAgentTurnCompleteRequest,
  InteractiveAgentTurnFailRequest,
  InteractiveAgentTurnHeartbeatRequest,
} from '@story2u/radar-contracts/interactive-agent';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { ManualReplyResultSchema } from './opportunity-actions';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const idempotencyPattern = '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$';
const failureCodePattern = '^[a-z][a-z0-9_.-]{0,63}$';
const DateTimeSchema = Type.String({ minLength: 20, maxLength: 64 });
const NullableDateTimeSchema = Type.Union([DateTimeSchema, Type.Null()]);
const NullableStringSchema = Type.Union([
  Type.String({ minLength: 1, maxLength: 64 }),
  Type.Null(),
]);
const RequiredTextSchema = Type.String({ minLength: 1, maxLength: 4_000 });

const InteractiveAgentTurnStatusSchema = Type.Union([
  Type.Literal('claimed'),
  Type.Literal('running'),
  Type.Literal('completed'),
  Type.Literal('failed'),
  Type.Literal('expired'),
]);

export const InteractiveAgentTurnSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    localSessionId: Type.String({ pattern: uuidPattern }),
    deviceId: Type.String({ pattern: uuidPattern }),
    status: InteractiveAgentTurnStatusSchema,
    runtimeVersion: Type.String({ minLength: 1, maxLength: 64 }),
    schemaVersion: Type.Integer({ minimum: 1, maximum: 100 }),
    modelAlias: Type.String({ minLength: 1, maxLength: 64 }),
    policyVersion: Type.String({ minLength: 1, maxLength: 64 }),
    lockVersion: Type.Integer({ minimum: 1 }),
    requestCount: Type.Integer({ minimum: 0 }),
    leaseExpiresAt: DateTimeSchema,
    claimedAt: DateTimeSchema,
    heartbeatAt: NullableDateTimeSchema,
    completedAt: NullableDateTimeSchema,
    failedAt: NullableDateTimeSchema,
    expiredAt: NullableDateTimeSchema,
    failureCode: NullableStringSchema,
  },
  { additionalProperties: false },
);

export const InteractiveAgentTurnClaimSchema = Type.Object(
  {
    ...InteractiveAgentTurnSchema.properties,
    turnToken: Type.String({ minLength: 16, maxLength: 16_384 }),
  },
  { additionalProperties: false },
);

const InteractiveAgentTurnClaimRequestSchema = Type.Object(
  {
    localSessionId: Type.String({ pattern: uuidPattern }),
    idempotencyKey: Type.String({ pattern: idempotencyPattern }),
  },
  { additionalProperties: false },
);
const InteractiveAgentTurnHeartbeatRequestSchema = Type.Object(
  { expectedLockVersion: Type.Integer({ minimum: 1 }) },
  { additionalProperties: false },
);
const InteractiveAgentTurnCompleteRequestSchema = Type.Object(
  { expectedLockVersion: Type.Integer({ minimum: 1 }) },
  { additionalProperties: false },
);
const InteractiveAgentTurnFailRequestSchema = Type.Object(
  {
    expectedLockVersion: Type.Integer({ minimum: 1 }),
    failureCode: Type.String({ pattern: failureCodePattern }),
  },
  { additionalProperties: false },
);
const InteractiveAgentApprovalStatusSchema = Type.Union([
  Type.Literal('denied'),
  Type.Literal('granted'),
  Type.Literal('executing'),
  Type.Literal('consumed'),
  Type.Literal('failed'),
  Type.Literal('uncertain'),
  Type.Literal('expired'),
]);
const InteractiveAgentApprovalDecisionRequestSchema = Type.Object(
  {
    approved: Type.Boolean(),
    toolCallId: Type.String({ pattern: '^[A-Za-z0-9._:-]{1,128}$' }),
    opportunityId: Type.String({ pattern: uuidPattern }),
    expectedVersion: Type.Integer({ minimum: 1 }),
    idempotencyKey: Type.String({ pattern: idempotencyPattern, minLength: 8 }),
    text: RequiredTextSchema,
  },
  { additionalProperties: false },
);
const InteractiveAgentApprovalDecisionSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    status: InteractiveAgentApprovalStatusSchema,
    toolCallId: Type.String({ pattern: '^[A-Za-z0-9._:-]{1,128}$' }),
    opportunityId: Type.String({ pattern: uuidPattern }),
    expectedVersion: Type.Integer({ minimum: 1 }),
    expiresAt: NullableDateTimeSchema,
    approvalToken: Type.Union([
      Type.String({ minLength: 16, maxLength: 16_384 }),
      Type.Null(),
    ]),
  },
  { additionalProperties: false },
);
const InteractiveAgentApprovedSendRequestSchema = Type.Object(
  {
    opportunityId: Type.String({ pattern: uuidPattern }),
    expectedVersion: Type.Integer({ minimum: 1 }),
    idempotencyKey: Type.String({ pattern: idempotencyPattern, minLength: 8 }),
    text: RequiredTextSchema,
  },
  { additionalProperties: false },
);
const InteractiveAgentApprovedSendSchema = Type.Object(
  {
    approvalId: Type.String({ pattern: uuidPattern }),
    ...ManualReplyResultSchema.properties,
  },
  { additionalProperties: false },
);

const decodeTurn: ResponseDecoder<InteractiveAgentTurn> = typeboxDecoder(
  InteractiveAgentTurnSchema,
);
const decodeClaim: ResponseDecoder<InteractiveAgentTurnClaim> = typeboxDecoder(
  InteractiveAgentTurnClaimSchema,
);
const parseClaim = typeboxDecoder(InteractiveAgentTurnClaimRequestSchema);
const parseHeartbeat = typeboxDecoder(InteractiveAgentTurnHeartbeatRequestSchema);
const parseComplete = typeboxDecoder(InteractiveAgentTurnCompleteRequestSchema);
const parseFail = typeboxDecoder(InteractiveAgentTurnFailRequestSchema);
const parseApprovalDecision = typeboxDecoder(InteractiveAgentApprovalDecisionRequestSchema);
const decodeApprovalDecision: ResponseDecoder<InteractiveAgentApprovalDecision> = typeboxDecoder(
  InteractiveAgentApprovalDecisionSchema,
);
const parseApprovedSend = typeboxDecoder(InteractiveAgentApprovedSendRequestSchema);
const parseApprovedSendResponse = typeboxDecoder(InteractiveAgentApprovedSendSchema);
const decodeApprovedSend: ResponseDecoder<InteractiveAgentApprovedSend> = (value) => {
  const parsed = parseApprovedSendResponse(value);
  if (parsed.message.isFromContact) {
    throw new Error('Approved reply result contains an incoming message');
  }
  return parsed as InteractiveAgentApprovedSend;
};

function explicitTurnId(value: string) {
  if (!(new RegExp(uuidPattern)).test(value)) {
    throw new Error('Invalid interactive Agent turn ID');
  }
  return value;
}

function turnAuthorization(token: string) {
  if (token.length < 16 || token.length > 16_384) {
    throw new Error('Invalid interactive Agent turn token');
  }
  return { Authorization: `Bearer ${token}` };
}

export function createInteractiveAgentApi(client: RadarApiClient) {
  return {
    claim(
      input: InteractiveAgentTurnClaimRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<InteractiveAgentTurnClaim> {
      const payload = parseClaim(input) as InteractiveAgentTurnClaimRequest;
      return client.request('/api/v1/agent/interactive/turns', {
        ...init,
        method: 'POST',
        body: JSON.stringify(payload),
        decode: decodeClaim,
      });
    },

    heartbeat(
      turnId: string,
      turnToken: string,
      input: InteractiveAgentTurnHeartbeatRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<InteractiveAgentTurn> {
      const payload = parseHeartbeat(input) as InteractiveAgentTurnHeartbeatRequest;
      return client.request(
        `/api/v1/agent/interactive/turns/${explicitTurnId(turnId)}/heartbeat`,
        {
          ...init,
          method: 'POST',
          headers: turnAuthorization(turnToken),
          body: JSON.stringify(payload),
          decode: decodeTurn,
        },
      );
    },

    complete(
      turnId: string,
      turnToken: string,
      input: InteractiveAgentTurnCompleteRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<InteractiveAgentTurn> {
      const payload = parseComplete(input) as InteractiveAgentTurnCompleteRequest;
      return client.request(
        `/api/v1/agent/interactive/turns/${explicitTurnId(turnId)}/complete`,
        {
          ...init,
          method: 'POST',
          headers: turnAuthorization(turnToken),
          body: JSON.stringify(payload),
          decode: decodeTurn,
        },
      );
    },

    fail(
      turnId: string,
      turnToken: string,
      input: InteractiveAgentTurnFailRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<InteractiveAgentTurn> {
      const payload = parseFail(input) as InteractiveAgentTurnFailRequest;
      return client.request(
        `/api/v1/agent/interactive/turns/${explicitTurnId(turnId)}/fail`,
        {
          ...init,
          method: 'POST',
          headers: turnAuthorization(turnToken),
          body: JSON.stringify(payload),
          decode: decodeTurn,
        },
      );
    },

    expire(
      turnId: string,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<InteractiveAgentTurn> {
      return client.request(
        `/api/v1/agent/interactive/turns/${explicitTurnId(turnId)}/expire`,
        {
          ...init,
          method: 'POST',
          decode: decodeTurn,
        },
      );
    },

    decideAction(
      turnToken: string,
      input: InteractiveAgentApprovalDecisionRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<InteractiveAgentApprovalDecision> {
      const payload = parseApprovalDecision(input) as InteractiveAgentApprovalDecisionRequest;
      return client.request('/api/v1/agent/interactive/actions/approvals', {
        ...init,
        method: 'POST',
        headers: turnAuthorization(turnToken),
        body: JSON.stringify(payload),
        decode: decodeApprovalDecision,
      });
    },

    sendApprovedReply(
      approvalToken: string,
      input: InteractiveAgentApprovedSendRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<InteractiveAgentApprovedSend> {
      const payload = parseApprovedSend(input) as InteractiveAgentApprovedSendRequest;
      return client.request('/api/v1/agent/interactive/actions/send-reply', {
        ...init,
        method: 'POST',
        headers: turnAuthorization(approvalToken),
        body: JSON.stringify(payload),
        decode: decodeApprovedSend,
      });
    },
  };
}
