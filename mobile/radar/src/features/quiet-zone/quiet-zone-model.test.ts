import { describe, expect, it } from 'vitest';

import { confidenceBand } from './quiet-zone-model';

describe('confidenceBand', () => {
  it('keeps low-confidence suppression visible as uncertain', () => {
    expect(confidenceBand(0.8)).toBe('high');
    expect(confidenceBand(0.55)).toBe('medium');
    expect(confidenceBand(0.54)).toBe('low');
  });
});

