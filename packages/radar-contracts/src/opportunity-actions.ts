import type { components } from './openapi';

type ManualReplyRequest = components['schemas']['ManualReplyRequest'];

export type ManualReplyInput = Pick<ManualReplyRequest, 'text'> &
  Partial<Pick<ManualReplyRequest, 'mark_following'>>;
export type ManualReplyResult = components['schemas']['ManualReplyResponse'];
export type AIDraft = components['schemas']['AIDraftResponse'];
export type OpportunityStatusUpdate = components['schemas']['OpportunityStatusUpdate'];
export type InternalOpportunityStatus = OpportunityStatusUpdate['status'];
