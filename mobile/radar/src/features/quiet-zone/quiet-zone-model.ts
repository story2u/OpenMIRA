import type { DecisionEvaluator, DeliveryMode, MessageDecisionEvidence } from '@story2u/radar-core/attention/model';

export interface QuietZoneItem {
  messageId: string;
  senderName: string;
  body: string;
  sentAt: string;
  decision: DeliveryMode;
  confidence: number;
  reasonSummary: string;
  evidence: readonly MessageDecisionEvidence[];
  evaluator: DecisionEvaluator;
  decidedAt: string;
}

export type ConfidenceBand = 'high' | 'medium' | 'low';

export function confidenceBand(confidence: number): ConfidenceBand {
  if (confidence >= 0.8) return 'high';
  if (confidence >= 0.55) return 'medium';
  return 'low';
}

