import type { MessageDecisionEvidence } from '@story2u/radar-core/attention/model';

import type { SignalAppetiteStoreExecutor } from '../../attention/signalAppetiteStore';
import type { QuietZoneItem } from './quiet-zone-model';

interface QuietZoneRow {
  message_id: string;
  confidence: number;
  reason_summary: string;
  evidence_json: string;
  evaluator: QuietZoneItem['evaluator'];
  decided_at: string;
  payload_json: string;
}

function readMessage(serialized: string) {
  const parsed: unknown = JSON.parse(serialized);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
  const value = parsed as Record<string, unknown>;
  if (typeof value.senderName !== 'string' || typeof value.content !== 'string' || typeof value.sentAt !== 'string') return null;
  return { senderName: value.senderName, body: value.content, sentAt: value.sentAt };
}

export async function readQuietZone(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  limit = 100,
): Promise<QuietZoneItem[]> {
  const rows = await database.getAllAsync<QuietZoneRow>(
    `SELECT decision.message_id, decision.confidence, decision.reason_summary,
      decision.evidence_json, decision.evaluator, decision.decided_at, message.payload_json
     FROM message_filter_decisions AS decision
     JOIN message_projection AS message
       ON message.owner_id = decision.owner_id AND message.id = decision.message_id
     WHERE decision.owner_id = ? AND decision.decision = 'suppress'
       AND message.deleted_at IS NULL
     ORDER BY decision.decided_at DESC, decision.message_id DESC LIMIT ?`,
    ownerId,
    Math.max(1, Math.min(limit, 200)),
  );
  return rows.flatMap((row) => {
    const message = readMessage(row.payload_json);
    if (!message) return [];
    return [{
      messageId: row.message_id,
      ...message,
      decision: 'suppress' as const,
      confidence: row.confidence,
      reasonSummary: row.reason_summary,
      evidence: JSON.parse(row.evidence_json) as MessageDecisionEvidence[],
      evaluator: row.evaluator,
      decidedAt: row.decided_at,
    }];
  });
}

