import assert from 'node:assert/strict';
import test from 'node:test';
import { Value } from 'typebox/value';

import {
  ClaimOpportunityParameters,
  DraftReplyParameters,
  GetMessagesParameters,
  GetOpportunityParameters,
  INTERACTIVE_AGENT_APPROVED_SEND_POLICY_VERSION,
  INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION,
  INTERACTIVE_AGENT_INTERNAL_POLICY_VERSION,
  INTERACTIVE_AGENT_INTERNAL_TOOLS_SCHEMA_VERSION,
  INTERACTIVE_AGENT_READ_ONLY_POLICY_VERSION,
  INTERACTIVE_AGENT_SYSTEM_PROMPT,
  INTERACTIVE_INTERNAL_TOOLS,
  INTERACTIVE_APPROVED_SEND_TOOLS,
  INTERACTIVE_READ_ONLY_TOOLS,
  SearchOpportunitiesParameters,
  SendReplyParameters,
  UpdateStatusParameters,
  interactiveAgentContractForSchema,
} from '../src/interactive.mjs';

test('interactive v1 exposes only the three reviewed read-only tools', () => {
  assert.deepEqual(
    INTERACTIVE_READ_ONLY_TOOLS.map((tool) => tool.name),
    ['search_opportunities', 'get_opportunity', 'get_messages'],
  );
  assert.doesNotMatch(
    INTERACTIVE_READ_ONLY_TOOLS.map((tool) => tool.name).join(','),
    /send|update|claim|remember|http|file|shell/,
  );
  assert.match(INTERACTIVE_AGENT_SYSTEM_PROMPT, /read-only tools/);
  assert.match(INTERACTIVE_AGENT_SYSTEM_PROMPT, /untrusted data/);
});

test('interactive v2 adds only reviewed local/internal tools and preserves v1', () => {
  const v1 = interactiveAgentContractForSchema(1);
  const v2 = interactiveAgentContractForSchema(INTERACTIVE_AGENT_INTERNAL_TOOLS_SCHEMA_VERSION);
  assert.equal(v1.policyVersion, INTERACTIVE_AGENT_READ_ONLY_POLICY_VERSION);
  assert.equal(v2.policyVersion, INTERACTIVE_AGENT_INTERNAL_POLICY_VERSION);
  assert.deepEqual(v1.tools, INTERACTIVE_READ_ONLY_TOOLS);
  assert.deepEqual(v2.tools, INTERACTIVE_INTERNAL_TOOLS);
  assert.deepEqual(
    v2.tools.map((tool) => tool.name),
    [
      'search_opportunities',
      'get_opportunity',
      'get_messages',
      'draft_reply',
      'update_status',
      'claim_opportunity',
    ],
  );
  assert.doesNotMatch(v2.tools.map((tool) => tool.name).join(','), /send|friend|email|notify/);
  assert.match(v2.systemPrompt, /current\s+request explicitly asks/);
  assert.match(v2.systemPrompt, /never sent/i);
});

test('interactive v3 adds only explicitly approved send without changing older contracts', () => {
  const v1 = interactiveAgentContractForSchema(1);
  const v2 = interactiveAgentContractForSchema(2);
  const v3 = interactiveAgentContractForSchema(INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION);
  assert.equal(v3.policyVersion, INTERACTIVE_AGENT_APPROVED_SEND_POLICY_VERSION);
  assert.deepEqual(v1.tools, INTERACTIVE_READ_ONLY_TOOLS);
  assert.deepEqual(v2.tools, INTERACTIVE_INTERNAL_TOOLS);
  assert.deepEqual(v3.tools, INTERACTIVE_APPROVED_SEND_TOOLS);
  assert.deepEqual(v3.tools.slice(-1).map((tool) => tool.name), ['send_reply']);
  assert.match(v3.systemPrompt, /one-time approval/);
  assert.match(v3.systemPrompt, /never ask for, invent, expose, or reuse an approval credential/);
  assert.throws(() => interactiveAgentContractForSchema(4), /unsupported/);
});

test('interactive tool parameters are strict and bounded', () => {
  const opportunityId = '11234567-89ab-cdef-0123-456789abcdef';
  assert.equal(Value.Check(SearchOpportunitiesParameters, { query: 'quote', limit: 10 }), true);
  assert.equal(Value.Check(SearchOpportunitiesParameters, { query: '', limit: 10 }), false);
  assert.equal(Value.Check(SearchOpportunitiesParameters, { query: 'quote', limit: 21 }), false);
  assert.equal(Value.Check(
    GetOpportunityParameters,
    { opportunity_id: opportunityId },
  ), true);
  assert.equal(Value.Check(
    GetOpportunityParameters,
    { opportunity_id: opportunityId, owner_id: opportunityId },
  ), false);
  assert.equal(Value.Check(
    GetMessagesParameters,
    { opportunity_id: opportunityId, limit: 20, offset: 0 },
  ), true);
  assert.equal(Value.Check(
    GetMessagesParameters,
    { opportunity_id: opportunityId, limit: 20, offset: -1 },
  ), false);
  assert.equal(Value.Check(
    DraftReplyParameters,
    { opportunity_id: opportunityId, text: 'A local draft' },
  ), true);
  assert.equal(Value.Check(
    DraftReplyParameters,
    { opportunity_id: opportunityId, text: '', send: true },
  ), false);
  assert.equal(Value.Check(
    UpdateStatusParameters,
    { opportunity_id: opportunityId, status: 'following' },
  ), true);
  assert.equal(Value.Check(
    UpdateStatusParameters,
    { opportunity_id: opportunityId, status: 'deleted' },
  ), false);
  assert.equal(Value.Check(
    ClaimOpportunityParameters,
    { opportunity_id: opportunityId },
  ), true);
  assert.equal(Value.Check(
    ClaimOpportunityParameters,
    { opportunity_id: opportunityId, owner_id: opportunityId },
  ), false);
  assert.equal(Value.Check(
    SendReplyParameters,
    { opportunity_id: opportunityId, text: 'Send this exact reply' },
  ), true);
  assert.equal(Value.Check(
    SendReplyParameters,
    {
      opportunity_id: opportunityId,
      text: 'Send this exact reply',
      approval_token: 'model-controlled-token',
    },
  ), false);
});
