import type { ReplyTemplate } from '@story2u/radar-contracts/templates';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';

export const ReplyTemplateSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    title: Type.String({ minLength: 1, maxLength: 128 }),
    content: Type.String({ minLength: 1, maxLength: 4000 }),
    category: Type.String({ maxLength: 64 }),
  },
  { additionalProperties: false },
);

const parseTemplates = typeboxDecoder(Type.Array(ReplyTemplateSchema, { maxItems: 200 }));

export const decodeReplyTemplates: ResponseDecoder<ReplyTemplate[]> = (value) => {
  const parsed = parseTemplates(value);
  const ids = new Set(parsed.map((template) => template.id));
  if (ids.size !== parsed.length) throw new Error('Reply templates contain duplicate ids');
  return parsed as ReplyTemplate[];
};

export function createTemplatesApi(client: RadarApiClient) {
  return {
    list(init: Pick<RequestInit, 'signal'> = {}): Promise<ReplyTemplate[]> {
      return client.request('/api/v1/templates', {
        ...init,
        decode: decodeReplyTemplates,
      });
    },
  };
}
