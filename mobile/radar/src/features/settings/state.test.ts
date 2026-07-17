import { describe, expect, it } from 'vitest';

import { initialSettingsState, settingsReducer } from './state';

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

describe('settings reducer', () => {
  it('preserves server truth when a write fails', () => {
    const loaded = settingsReducer(initialSettingsState, { type: 'load-succeeded', bundle });
    const saving = settingsReducer(loaded, { type: 'save-started', kind: 'detection' });
    const failed = settingsReducer(saving, {
      type: 'save-failed',
      kind: 'detection',
      error: '保存失败',
    });

    expect(failed.bundle).toEqual(bundle);
    expect(failed.busyAction).toBeNull();
    expect(failed.saveError).toBe('保存失败');
  });

  it('replaces only the saved slice with the strict server response', () => {
    const loaded = settingsReducer(initialSettingsState, { type: 'load-succeeded', bundle });
    const saving = settingsReducer(loaded, { type: 'save-started', kind: 'notifications' });
    const next = settingsReducer(saving, {
      type: 'notifications-succeeded',
      value: {
        newOpportunityEnabled: false,
        aiRepliedEnabled: false,
        dailyDigestEnabled: true,
        urgentOnly: true,
      },
    });

    expect(next.bundle?.detection).toEqual(bundle.detection);
    expect(next.bundle?.notifications.urgentOnly).toBe(true);
    expect(next.busyAction).toBeNull();
  });
});
