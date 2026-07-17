import type { InteractiveToolName } from '@story2u/radar-agent/interactive';

export const AGENT_SESSION_SCHEMA_VERSION = 1;
export const AGENT_SESSION_RETENTION_DAYS = 30;
export const MAXIMUM_AGENT_SESSIONS_PER_OWNER = 100;
export const MAXIMUM_AGENT_ENTRIES_PER_SESSION = 500;
export const MAXIMUM_AGENT_ENTRY_BYTES = 65_536;

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const toolCallIdPattern = /^[A-Za-z0-9._:-]{1,128}$/;
const errorCodePattern = /^[a-z0-9_]{1,64}$/;
const toolNames = new Set<InteractiveToolName>([
  'search_opportunities',
  'get_opportunity',
  'get_messages',
  'draft_reply',
  'update_status',
  'claim_opportunity',
  'send_reply',
]);

type SqliteValue = string | number | null;

export interface AgentSessionStoreExecutor {
  getAllAsync<Row>(source: string, ...params: SqliteValue[]): Promise<Row[]>;
  getFirstAsync<Row>(source: string, ...params: SqliteValue[]): Promise<Row | null>;
  runAsync(source: string, ...params: SqliteValue[]): Promise<unknown>;
}

export interface AgentSessionStoreDatabase extends AgentSessionStoreExecutor {
  withExclusiveTransactionAsync(
    task: (transaction: AgentSessionStoreExecutor) => Promise<void>,
  ): Promise<void>;
}

export type AgentEntryContent =
  | { type: 'user'; text: string }
  | { type: 'assistant'; text: string }
  | {
    type: 'tool_call';
    toolCallId: string;
    toolName: InteractiveToolName;
    arguments: Record<string, unknown>;
  }
  | {
    type: 'tool_result';
    toolCallId: string;
    toolName: InteractiveToolName;
    result: Record<string, unknown>;
  }
  | { type: 'error'; code: string };

export interface LocalAgentSession {
  ownerId: string;
  id: string;
  opportunityId: string | null;
  schemaVersion: number;
  title: string;
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
}

export interface LocalAgentEntry {
  ownerId: string;
  sessionId: string;
  seq: number;
  content: AgentEntryContent;
  createdAt: string;
}

interface SessionRow {
  owner_id: string;
  id: string;
  opportunity_id: string | null;
  schema_version: number;
  title: string;
  created_at: string;
  updated_at: string;
  expires_at: string;
}

interface EntryRow {
  owner_id: string;
  session_id: string;
  seq: number;
  entry_type: AgentEntryContent['type'];
  content_json: string;
  created_at: string;
}

export class AgentSessionStoreError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'AgentSessionStoreError';
  }
}

function requireUuid(value: string, field: string) {
  if (!uuidPattern.test(value)) throw new AgentSessionStoreError(`invalid_${field}`);
  return value.toLowerCase();
}

function expiryFrom(now: Date) {
  return new Date(
    now.getTime() + AGENT_SESSION_RETENTION_DAYS * 24 * 60 * 60 * 1_000,
  ).toISOString();
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function boundedText(value: unknown, maximum: number) {
  return typeof value === 'string' && value.length >= 1 && value.length <= maximum;
}

function validPersistedSendReplyArguments(value: Record<string, unknown>) {
  return Object.keys(value).length === 2
    && typeof value.opportunity_id === 'string'
    && uuidPattern.test(value.opportunity_id)
    && boundedText(value.text, 4_000)
    && String(value.text).trim().length > 0;
}

function validPersistedSendReplyResult(value: Record<string, unknown>) {
  if (Object.keys(value).length === 1) {
    return value.error === 'tool_execution_failed';
  }
  return Object.keys(value).length === 5
    && value.state === 'sent'
    && value.sent === true
    && typeof value.approval_id === 'string'
    && uuidPattern.test(value.approval_id)
    && typeof value.message_id === 'string'
    && uuidPattern.test(value.message_id)
    && typeof value.opportunity_id === 'string'
    && uuidPattern.test(value.opportunity_id);
}

function validateEntryContent(value: unknown): AgentEntryContent {
  if (!isRecord(value) || typeof value.type !== 'string') {
    throw new AgentSessionStoreError('invalid_agent_entry');
  }
  switch (value.type) {
    case 'user':
      if (!boundedText(value.text, 8_000) || Object.keys(value).length !== 2) break;
      return value as unknown as AgentEntryContent;
    case 'assistant':
      if (!boundedText(value.text, 32_000) || Object.keys(value).length !== 2) break;
      return value as unknown as AgentEntryContent;
    case 'tool_call':
      if (
        !toolCallIdPattern.test(String(value.toolCallId ?? ''))
        || !toolNames.has(value.toolName as InteractiveToolName)
        || !isRecord(value.arguments)
        || (value.toolName === 'send_reply' && !validPersistedSendReplyArguments(value.arguments))
        || Object.keys(value).length !== 4
      ) break;
      return value as unknown as AgentEntryContent;
    case 'tool_result':
      if (
        !toolCallIdPattern.test(String(value.toolCallId ?? ''))
        || !toolNames.has(value.toolName as InteractiveToolName)
        || !isRecord(value.result)
        || (value.toolName === 'send_reply' && !validPersistedSendReplyResult(value.result))
        || Object.keys(value).length !== 4
      ) break;
      return value as unknown as AgentEntryContent;
    case 'error':
      if (
        typeof value.code !== 'string'
        || !errorCodePattern.test(value.code)
        || Object.keys(value).length !== 2
      ) break;
      return value as unknown as AgentEntryContent;
    default:
      break;
  }
  throw new AgentSessionStoreError('invalid_agent_entry');
}

function serializeEntry(content: AgentEntryContent) {
  const validated = validateEntryContent(content);
  let serialized: string;
  try {
    serialized = JSON.stringify(validated);
  } catch {
    throw new AgentSessionStoreError('invalid_agent_entry');
  }
  if (new TextEncoder().encode(serialized).byteLength > MAXIMUM_AGENT_ENTRY_BYTES) {
    throw new AgentSessionStoreError('agent_entry_too_large');
  }
  try {
    return {
      content: validateEntryContent(JSON.parse(serialized)),
      serialized,
    };
  } catch {
    throw new AgentSessionStoreError('invalid_agent_entry');
  }
}

function sessionFromRow(row: SessionRow): LocalAgentSession {
  return {
    ownerId: row.owner_id,
    id: row.id,
    opportunityId: row.opportunity_id,
    schemaVersion: row.schema_version,
    title: row.title,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
    expiresAt: row.expires_at,
  };
}

function entryFromRow(row: EntryRow): LocalAgentEntry {
  let content: AgentEntryContent;
  try {
    content = validateEntryContent(JSON.parse(row.content_json));
  } catch {
    throw new AgentSessionStoreError('agent_entry_corrupt');
  }
  if (content.type !== row.entry_type) {
    throw new AgentSessionStoreError('agent_entry_corrupt');
  }
  return {
    ownerId: row.owner_id,
    sessionId: row.session_id,
    seq: row.seq,
    content,
    createdAt: row.created_at,
  };
}

async function deleteExpired(
  executor: Pick<AgentSessionStoreExecutor, 'runAsync'>,
  ownerId: string,
  now: Date,
) {
  await executor.runAsync(
    'DELETE FROM agent_sessions WHERE owner_id = ? AND expires_at <= ?',
    ownerId,
    now.toISOString(),
  );
}

export async function purgeExpiredAgentSessions(
  database: AgentSessionStoreDatabase,
  ownerId: string,
  now: Date = new Date(),
) {
  const normalizedOwnerId = requireUuid(ownerId, 'owner_id');
  await database.withExclusiveTransactionAsync(
    (transaction) => deleteExpired(transaction, normalizedOwnerId, now),
  );
}

export async function createAgentSession(
  database: AgentSessionStoreDatabase,
  input: {
    ownerId: string;
    sessionId: string;
    opportunityId?: string | null;
    title: string;
  },
  now: Date = new Date(),
): Promise<LocalAgentSession> {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const sessionId = requireUuid(input.sessionId, 'session_id');
  const opportunityId = input.opportunityId
    ? requireUuid(input.opportunityId, 'opportunity_id')
    : null;
  const title = input.title.trim();
  if (!boundedText(title, 120)) throw new AgentSessionStoreError('invalid_session_title');
  const timestamp = now.toISOString();
  const expiresAt = expiryFrom(now);
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await deleteExpired(transaction, ownerId, now);
    const count = await transaction.getFirstAsync<{ total: number }>(
      'SELECT COUNT(*) AS total FROM agent_sessions WHERE owner_id = ?',
      ownerId,
    );
    if ((count?.total ?? 0) >= MAXIMUM_AGENT_SESSIONS_PER_OWNER) {
      throw new AgentSessionStoreError('agent_session_limit_reached');
    }
    await transaction.runAsync(
      `INSERT INTO agent_sessions (
        owner_id, id, opportunity_id, schema_version, title,
        created_at, updated_at, expires_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
      ownerId,
      sessionId,
      opportunityId,
      AGENT_SESSION_SCHEMA_VERSION,
      title,
      timestamp,
      timestamp,
      expiresAt,
    );
  });
  return {
    ownerId,
    id: sessionId,
    opportunityId,
    schemaVersion: AGENT_SESSION_SCHEMA_VERSION,
    title,
    createdAt: timestamp,
    updatedAt: timestamp,
    expiresAt,
  };
}

export async function listAgentSessions(
  database: AgentSessionStoreDatabase,
  ownerId: string,
  now: Date = new Date(),
): Promise<LocalAgentSession[]> {
  const normalizedOwnerId = requireUuid(ownerId, 'owner_id');
  await purgeExpiredAgentSessions(database, normalizedOwnerId, now);
  const rows = await database.getAllAsync<SessionRow>(
    `SELECT * FROM agent_sessions
     WHERE owner_id = ? ORDER BY updated_at DESC, id LIMIT ?`,
    normalizedOwnerId,
    MAXIMUM_AGENT_SESSIONS_PER_OWNER,
  );
  return rows.map(sessionFromRow);
}

export async function appendAgentEntry(
  database: AgentSessionStoreDatabase,
  input: {
    ownerId: string;
    sessionId: string;
    content: AgentEntryContent;
  },
  now: Date = new Date(),
): Promise<LocalAgentEntry> {
  const [entry] = await appendAgentEntries(
    database,
    {
      ownerId: input.ownerId,
      sessionId: input.sessionId,
      contents: [input.content],
    },
    now,
  );
  if (!entry) throw new AgentSessionStoreError('invalid_agent_entry');
  return entry;
}

export async function appendAgentEntries(
  database: AgentSessionStoreDatabase,
  input: {
    ownerId: string;
    sessionId: string;
    contents: AgentEntryContent[];
  },
  now: Date = new Date(),
): Promise<LocalAgentEntry[]> {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const sessionId = requireUuid(input.sessionId, 'session_id');
  if (input.contents.length < 1 || input.contents.length > 128) {
    throw new AgentSessionStoreError('invalid_agent_entry_batch');
  }
  const encoded = input.contents.map(serializeEntry);
  const timestamp = now.toISOString();
  const expiresAt = expiryFrom(now);
  let firstSeq = 0;
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await deleteExpired(transaction, ownerId, now);
    const session = await transaction.getFirstAsync<{ id: string }>(
      'SELECT id FROM agent_sessions WHERE owner_id = ? AND id = ?',
      ownerId,
      sessionId,
    );
    if (!session) throw new AgentSessionStoreError('agent_session_not_found');
    const sequence = await transaction.getFirstAsync<{ maximum: number | null }>(
      `SELECT MAX(seq) AS maximum FROM agent_entries
       WHERE owner_id = ? AND session_id = ?`,
      ownerId,
      sessionId,
    );
    firstSeq = (sequence?.maximum ?? 0) + 1;
    if (firstSeq + encoded.length - 1 > MAXIMUM_AGENT_ENTRIES_PER_SESSION) {
      throw new AgentSessionStoreError('agent_entry_limit_reached');
    }
    for (const [index, item] of encoded.entries()) {
      await transaction.runAsync(
        `INSERT INTO agent_entries (
          owner_id, session_id, seq, entry_type, content_json, created_at
        ) VALUES (?, ?, ?, ?, ?, ?)`,
        ownerId,
        sessionId,
        firstSeq + index,
        item.content.type,
        item.serialized,
        timestamp,
      );
    }
    await transaction.runAsync(
      `UPDATE agent_sessions SET updated_at = ?, expires_at = ?
       WHERE owner_id = ? AND id = ?`,
      timestamp,
      expiresAt,
      ownerId,
      sessionId,
    );
  });
  return encoded.map((item, index) => ({
    ownerId,
    sessionId,
    seq: firstSeq + index,
    content: item.content,
    createdAt: timestamp,
  }));
}

export async function readAgentEntries(
  database: AgentSessionStoreExecutor,
  ownerId: string,
  sessionId: string,
  options: { afterSeq?: number; limit?: number } = {},
): Promise<LocalAgentEntry[]> {
  const normalizedOwnerId = requireUuid(ownerId, 'owner_id');
  const normalizedSessionId = requireUuid(sessionId, 'session_id');
  const afterSeq = options.afterSeq ?? 0;
  const limit = options.limit ?? 100;
  if (!Number.isInteger(afterSeq) || afterSeq < 0) {
    throw new AgentSessionStoreError('invalid_entry_cursor');
  }
  if (!Number.isInteger(limit) || limit < 1 || limit > MAXIMUM_AGENT_ENTRIES_PER_SESSION) {
    throw new AgentSessionStoreError('invalid_entry_limit');
  }
  const rows = await database.getAllAsync<EntryRow>(
    `SELECT * FROM agent_entries
     WHERE owner_id = ? AND session_id = ? AND seq > ?
     ORDER BY seq LIMIT ?`,
    normalizedOwnerId,
    normalizedSessionId,
    afterSeq,
    limit,
  );
  return rows.map(entryFromRow);
}

export async function deleteAgentSession(
  database: AgentSessionStoreExecutor,
  ownerId: string,
  sessionId: string,
) {
  await database.runAsync(
    'DELETE FROM agent_sessions WHERE owner_id = ? AND id = ?',
    requireUuid(ownerId, 'owner_id'),
    requireUuid(sessionId, 'session_id'),
  );
}
