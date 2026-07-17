import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import {
  createSettingsApi,
  decodeSettingsBundle,
  normalizeDetectionSettings,
  normalizeWorkSchedule,
} from './settings';

const bundle = {
  detection: { keywords: ['报价'], aiSemanticsEnabled: true },
  workSchedule: {
    timezone: 'Asia/Shanghai',
    slots: [{ weekday: 1, start: '09:00', end: '18:00' }],
    autoReplyOutsideHours: true,
    isDefault: false,
  },
  notifications: {
    newOpportunityEnabled: true,
    aiRepliedEnabled: true,
    dailyDigestEnabled: false,
    urgentOnly: false,
  },
  capabilities: { pushAvailable: false, wecomUserBindingAvailable: true },
};

describe('settings API', () => {
  it('strictly decodes the owner settings bundle', () => {
    expect(decodeSettingsBundle(bundle)).toEqual(bundle);
    expect(() => decodeSettingsBundle({ ...bundle, secret: 'must-not-pass' })).toThrow();
    expect(() => decodeSettingsBundle({
      ...bundle,
      workSchedule: {
        ...bundle.workSchedule,
        slots: [{ weekday: 1, start: '18:00', end: '09:00' }],
      },
    })).toThrow('after start');
  });

  it('normalizes keyword writes exactly once at the shared boundary', () => {
    expect(normalizeDetectionSettings({
      keywords: [' 报价 ', '报价', 'API', 'api', ''],
      aiSemanticsEnabled: false,
    })).toEqual({ keywords: ['报价', 'API'], aiSemanticsEnabled: false });
    expect(() => normalizeDetectionSettings({
      keywords: ['x'.repeat(65)],
      aiSemanticsEnabled: true,
    })).toThrow('64');
  });

  it('rejects invalid timezones, cross-midnight slots and oversized schedules', () => {
    expect(normalizeWorkSchedule({
      timezone: 'UTC',
      slots: [],
      autoReplyOutsideHours: true,
    })).toEqual({ timezone: 'UTC', slots: [], autoReplyOutsideHours: true });
    expect(() => normalizeWorkSchedule({
      timezone: 'Not/AZone',
      slots: [],
      autoReplyOutsideHours: true,
    })).toThrow('IANA');
    expect(() => normalizeWorkSchedule({
      timezone: 'Asia/Shanghai',
      slots: [{ weekday: 1, start: '18:00', end: '09:00' }],
      autoReplyOutsideHours: true,
    })).toThrow('after start');
  });

  it('uses PATCH with normalized server-shaped payloads', async () => {
    const fetch = vi.fn(async (_input: string, init?: RequestInit) => Response.json({
      keywords: JSON.parse(String(init?.body)).keywords,
      aiSemanticsEnabled: false,
    }));
    const api = createSettingsApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'access-token',
    }));

    await expect(api.updateDetection({
      keywords: [' 报价 ', '报价'],
      aiSemanticsEnabled: false,
    })).resolves.toEqual({ keywords: ['报价'], aiSemanticsEnabled: false });
    expect(fetch.mock.calls[0][0]).toBe('https://api.example.test/api/v1/settings/detection');
    expect(fetch.mock.calls[0][1]?.method).toBe('PATCH');
    expect(fetch.mock.calls[0][1]?.body).toBe(JSON.stringify({
      keywords: ['报价'],
      aiSemanticsEnabled: false,
    }));
  });
});
