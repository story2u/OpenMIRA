import { afterEach, describe, expect, it, vi } from 'vitest'
import { generateOpportunityAiDraft, updateTelegramConnectionSource } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
})

function mockJsonResponse(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('safe auto reply API', () => {
  it('updates only the current Telegram source authorization flag', async () => {
    const fetchMock = mockJsonResponse({ id: 'connection-1', sources: [] })

    await updateTelegramConnectionSource('source-1', true)

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/integrations/telegram/sources/source-1',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ autoReplyEnabled: true }),
      }),
    )
  })

  it('requests a server-side draft without submitting client entitlement or send state', async () => {
    const fetchMock = mockJsonResponse({ opportunity_id: 'opportunity-1', draft: '草稿' })

    await generateOpportunityAiDraft('opportunity-1')

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/opportunities/opportunity-1/ai-draft',
      expect.objectContaining({ method: 'POST' }),
    )
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(init.body).toBeUndefined()
  })
})
