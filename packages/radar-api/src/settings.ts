import type {
  DetectionSettings,
  DetectionSettingsUpdate,
  NotificationSettings,
  NotificationSettingsUpdate,
  SettingsBundle,
  WorkSchedule,
  WorkScheduleSlot,
  WorkScheduleUpdate,
} from '@story2u/radar-contracts/settings';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const timePattern = '^([01]\\d|2[0-3]):[0-5]\\d$';

export const DetectionSettingsSchema = Type.Object(
  {
    keywords: Type.Array(Type.String({ minLength: 1, maxLength: 64 }), { maxItems: 200 }),
    aiSemanticsEnabled: Type.Boolean(),
  },
  { additionalProperties: false },
);

export const WorkScheduleSlotSchema = Type.Object(
  {
    weekday: Type.Integer({ minimum: 1, maximum: 7 }),
    start: Type.String({ pattern: timePattern }),
    end: Type.String({ pattern: timePattern }),
  },
  { additionalProperties: false },
);

export const WorkScheduleSchema = Type.Object(
  {
    timezone: Type.String({ minLength: 1, maxLength: 64 }),
    slots: Type.Array(WorkScheduleSlotSchema, { maxItems: 168 }),
    autoReplyOutsideHours: Type.Boolean(),
    isDefault: Type.Boolean(),
  },
  { additionalProperties: false },
);

export const NotificationSettingsSchema = Type.Object(
  {
    newOpportunityEnabled: Type.Boolean(),
    aiRepliedEnabled: Type.Boolean(),
    dailyDigestEnabled: Type.Boolean(),
    urgentOnly: Type.Boolean(),
  },
  { additionalProperties: false },
);

export const SettingsBundleSchema = Type.Object(
  {
    detection: DetectionSettingsSchema,
    workSchedule: WorkScheduleSchema,
    notifications: NotificationSettingsSchema,
    capabilities: Type.Object(
      {
        pushAvailable: Type.Boolean(),
        wecomUserBindingAvailable: Type.Boolean(),
      },
      { additionalProperties: false },
    ),
  },
  { additionalProperties: false },
);

const parseDetection = typeboxDecoder(DetectionSettingsSchema);
const parseWorkSchedule = typeboxDecoder(WorkScheduleSchema);
const parseNotifications = typeboxDecoder(NotificationSettingsSchema);
const parseSettingsBundle = typeboxDecoder(SettingsBundleSchema);

function requireIanaTimezone(timezone: string) {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: timezone }).format();
  } catch {
    throw new Error('Invalid IANA timezone');
  }
}

function normalizeKeywords(keywords: string[]) {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const item of keywords) {
    const keyword = item.trim();
    if (!keyword) continue;
    if (keyword.length > 64) throw new Error('Detection keyword must not exceed 64 characters');
    const key = keyword.toLocaleLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(keyword);
  }
  if (result.length > 200) throw new Error('Detection keywords must not exceed 200 items');
  return result;
}

function normalizeSlot(slot: WorkScheduleSlot): WorkScheduleSlot {
  const parsed = typeboxDecoder(WorkScheduleSlotSchema)(slot);
  if (parsed.end <= parsed.start) throw new Error('Work schedule end must be after start');
  return parsed as WorkScheduleSlot;
}

export function normalizeDetectionSettings(input: DetectionSettingsUpdate): DetectionSettingsUpdate {
  return {
    keywords: normalizeKeywords(input.keywords),
    aiSemanticsEnabled: input.aiSemanticsEnabled,
  };
}

export function normalizeWorkSchedule(input: WorkScheduleUpdate): WorkScheduleUpdate {
  const timezone = input.timezone.trim();
  requireIanaTimezone(timezone);
  if (input.slots.length > 168) throw new Error('Work schedule must not exceed 168 slots');
  return {
    timezone,
    slots: input.slots.map(normalizeSlot),
    autoReplyOutsideHours: input.autoReplyOutsideHours,
  };
}

export const decodeDetectionSettings: ResponseDecoder<DetectionSettings> = (value) =>
  parseDetection(value) as DetectionSettings;
export const decodeWorkSchedule: ResponseDecoder<WorkSchedule> = (value) => {
  const parsed = parseWorkSchedule(value) as WorkSchedule;
  requireIanaTimezone(parsed.timezone);
  parsed.slots.forEach(normalizeSlot);
  return parsed;
};
export const decodeNotificationSettings: ResponseDecoder<NotificationSettings> = (value) =>
  parseNotifications(value) as NotificationSettings;
export const decodeSettingsBundle: ResponseDecoder<SettingsBundle> = (value) => {
  const parsed = parseSettingsBundle(value) as SettingsBundle;
  requireIanaTimezone(parsed.workSchedule.timezone);
  parsed.workSchedule.slots.forEach(normalizeSlot);
  return parsed;
};

export function createSettingsApi(client: RadarApiClient) {
  return {
    get(init: Pick<RequestInit, 'signal'> = {}): Promise<SettingsBundle> {
      return client.request('/api/v1/settings/me', { ...init, decode: decodeSettingsBundle });
    },

    updateDetection(
      input: DetectionSettingsUpdate,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<DetectionSettings> {
      return client.request('/api/v1/settings/detection', {
        ...init,
        method: 'PATCH',
        body: JSON.stringify(normalizeDetectionSettings(input)),
        decode: decodeDetectionSettings,
      });
    },

    updateWorkSchedule(
      input: WorkScheduleUpdate,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<WorkSchedule> {
      return client.request('/api/v1/settings/work-schedule', {
        ...init,
        method: 'PATCH',
        body: JSON.stringify(normalizeWorkSchedule(input)),
        decode: decodeWorkSchedule,
      });
    },

    updateNotifications(
      input: NotificationSettingsUpdate,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<NotificationSettings> {
      return client.request('/api/v1/settings/notifications', {
        ...init,
        method: 'PATCH',
        body: JSON.stringify(input),
        decode: decodeNotificationSettings,
      });
    },
  };
}
