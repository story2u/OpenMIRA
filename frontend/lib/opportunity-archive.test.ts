import { describe, expect, it } from 'vitest'
import { toOpportunity } from './api'

const baseOpportunity = {
  id: 'opportunity-1',
  platform: 'telegram' as const,
  contactName: '演示联系人',
  summary: '测试商机',
  matchedKeywords: [],
  confidenceScore: 0.8,
  status: 'replied' as const,
  priority: 'normal' as const,
  lastMessagePreview: '测试消息',
  createdAt: '2026-07-13T00:00:00Z',
}

describe('opportunity archive mapping', () => {
  it('treats old API responses without archive fields as active', () => {
    const opportunity = toOpportunity(baseOpportunity)

    expect(opportunity.archivedAt).toBeNull()
    expect(opportunity.archivedByUserId).toBeNull()
    expect(opportunity.archiveReason).toBeNull()
  })

  it('preserves archive metadata without changing the business status', () => {
    const opportunity = toOpportunity({
      ...baseOpportunity,
      archivedAt: '2026-07-13T01:00:00Z',
      archivedByUserId: '12a2937f-0183-46e7-8f24-928ffb19a20d',
      archiveReason: '已完成跟进',
    })

    expect(opportunity.status).toBe('replied')
    expect(opportunity.archivedAt).toBe('2026-07-13T01:00:00Z')
    expect(opportunity.archiveReason).toBe('已完成跟进')
  })
})
