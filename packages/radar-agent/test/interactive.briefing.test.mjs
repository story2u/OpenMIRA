import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  INTERACTIVE_AGENT_BRIEFING_SCHEMA_VERSION,
  INTERACTIVE_AGENT_SCHEMA_VERSION,
  INTERACTIVE_BRIEFING_ALL_TOOLS,
  INTERACTIVE_BRIEFING_TOOLS,
  interactiveAgentContractForSchema,
} from '../src/interactive.mjs';

test('v5 briefing contract exposes six briefing tools on top of v4', () => {
  assert.equal(INTERACTIVE_AGENT_BRIEFING_SCHEMA_VERSION, 5);
  assert.equal(INTERACTIVE_AGENT_SCHEMA_VERSION, 5);
  const contract = interactiveAgentContractForSchema(5);
  assert.equal(contract.policyVersion, 'interactive-briefing-v5');
  assert.equal(contract.tools, INTERACTIVE_BRIEFING_ALL_TOOLS);
  assert.deepEqual(
    INTERACTIVE_BRIEFING_TOOLS.map((tool) => tool.name),
    [
      'summarize_time_window',
      'get_attention_snapshot',
      'list_priority_items',
      'list_category_items',
      'get_quiet_summary',
      'update_brief_schedule',
    ],
  );
});

test('tool names stay unique across every contract version', () => {
  for (const version of [1, 2, 3, 4, 5]) {
    const names = interactiveAgentContractForSchema(version).tools.map((tool) => tool.name);
    assert.equal(new Set(names).size, names.length, `duplicate tool in v${version}`);
  }
});

test('v4 contract is unchanged by the v5 addition', () => {
  const v4 = interactiveAgentContractForSchema(4);
  assert.equal(v4.policyVersion, 'interactive-signal-appetite-v4');
  assert.equal(v4.tools.length, 22);
  assert.ok(!v4.tools.some((tool) => tool.name === 'summarize_time_window'));
});
