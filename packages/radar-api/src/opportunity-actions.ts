import type {
  AIDraft,
  InternalOpportunityStatus,
  ManualReplyInput,
  ManualReplyResult,
} from '@story2u/radar-contracts/opportunity-actions';
import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { idempotencyHeaders } from './client';
import { ChatMessageSchema } from './messages';
import {
  decodeOpportunityDetailResponse,
  OpportunityDetailSchema,
} from './opportunities';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const statuses = new Set<InternalOpportunityStatus>([
  'pending_human',
  'ai_auto_reply',
  'replied',
  'following',
  'ignored',
  'closed',
]);

export const ManualReplyResultSchema = Type.Object(
  {
    opportunity: OpportunityDetailSchema,
    message: ChatMessageSchema,
    messageTotal: Type.Integer({ minimum: 1 }),
  },
  { additionalProperties: false },
);

export const AIDraftSchema = Type.Object(
  {
    opportunity_id: Type.String({ pattern: uuidPattern }),
    draft: Type.String({ minLength: 1, maxLength: 4000 }),
  },
  { additionalProperties: false },
);

const parseManualReplyResult = typeboxDecoder(ManualReplyResultSchema);
const parseAIDraft = typeboxDecoder(AIDraftSchema);

export const decodeManualReplyResult: ResponseDecoder<ManualReplyResult> = (value) => {
  const parsed = parseManualReplyResult(value);
  if (parsed.message.isFromContact) {
    throw new Error('Manual reply result contains an incoming message');
  }
  return parsed as ManualReplyResult;
};

export const decodeAIDraft: ResponseDecoder<AIDraft> = (value) => parseAIDraft(value) as AIDraft;

function requireUuid(value: string) {
  if (!new RegExp(uuidPattern).test(value)) throw new Error('Invalid opportunity id');
}

function opportunityActionPath(opportunityId: string, action: string) {
  requireUuid(opportunityId);
  return `/api/v1/opportunities/${encodeURIComponent(opportunityId)}/${action}`;
}

function normalizeManualReply(input: ManualReplyInput) {
  const text = input.text.trim();
  if (text.length < 1 || text.length > 4000) {
    throw new Error('Manual reply text must contain 1 to 4000 characters');
  }
  return { text, mark_following: input.mark_following ?? true };
}

export interface StatusUpdateOptions extends Pick<RequestInit, 'signal'> {
  expectedVersion?: number;
  idempotencyKey?: string;
}

export function createOpportunityActionsApi(client: RadarApiClient) {
  return {
    manualReply(
      opportunityId: string,
      input: ManualReplyInput,
      idempotencyKey: string,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<ManualReplyResult> {
      return client.request(opportunityActionPath(opportunityId, 'manual-reply/result'), {
        ...init,
        method: 'POST',
        headers: idempotencyHeaders(idempotencyKey),
        body: JSON.stringify(normalizeManualReply(input)),
        decode: decodeManualReplyResult,
      });
    },

    generateAIDraft(
      opportunityId: string,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<AIDraft> {
      return client.request(opportunityActionPath(opportunityId, 'ai-draft'), {
        ...init,
        method: 'POST',
        decode: decodeAIDraft,
      });
    },

    updateStatus(
      opportunityId: string,
      status: InternalOpportunityStatus,
      options: StatusUpdateOptions = {},
    ): Promise<OpportunityDetail> {
      if (!statuses.has(status)) throw new Error('Invalid opportunity status');
      if ((options.expectedVersion === undefined) !== (options.idempotencyKey === undefined)) {
        throw new Error('Status expectedVersion and idempotencyKey must be provided together');
      }
      if (
        options.expectedVersion !== undefined
        && (!Number.isSafeInteger(options.expectedVersion) || options.expectedVersion < 1)
      ) {
        throw new Error('Invalid opportunity expected version');
      }
      const headers = options.idempotencyKey
        ? idempotencyHeaders(options.idempotencyKey)
        : undefined;
      return client.request(opportunityActionPath(opportunityId, 'status'), {
        signal: options.signal,
        method: 'PATCH',
        headers,
        body: JSON.stringify({
          status,
          ...(options.expectedVersion === undefined
            ? {}
            : { expectedVersion: options.expectedVersion }),
        }),
        decode: decodeOpportunityDetailResponse,
      });
    },

    claim(
      opportunityId: string,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<OpportunityDetail> {
      return client.request(opportunityActionPath(opportunityId, 'claim'), {
        ...init,
        method: 'POST',
        decode: decodeOpportunityDetailResponse,
      });
    },
  };
}
