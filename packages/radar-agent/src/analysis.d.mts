import type { TSchema } from 'typebox';

export interface AnalysisResult {
  is_opportunity: boolean;
  confidence: number;
  title: string;
  summary: string;
  priority: 'low' | 'normal' | 'high' | 'urgent';
  trust_score: number;
  attention_required: boolean;
  link_status: 'unverified' | 'safe' | 'suspicious' | 'malicious';
  link_summary: string | null;
  risk_flags: string[];
  contacts: {
    email: string | null;
    phone: string | null;
    telegram_handle: string | null;
    wecom_id: string | null;
    extraction_source: 'message_text' | 'link_content' | null;
  };
  actions: Array<{
    action_type: 'send_email' | 'add_friend' | 'private_message' | 'notify_user';
    reason: string;
    target: string | null;
    draft: string | null;
    requires_approval: boolean;
  }>;
}

export const AnalysisSchema: TSchema;
export const ANALYSIS_SYSTEM_PROMPT: string;

export function createSubmitAnalysisTool(onSubmit: (result: AnalysisResult) => void): {
  name: 'submit_analysis';
  label: string;
  description: string;
  parameters: TSchema;
  executionMode: 'sequential';
  execute: (toolCallId: string, params: AnalysisResult) => Promise<{
    content: Array<{ type: 'text'; text: string }>;
    details: Record<string, never>;
    terminate: true;
  }>;
};

export function serializeUntrustedInput(input: unknown): string;
