'use client'

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import {
  archiveOpportunity as archiveOpportunityRequest,
  bulkArchiveOpportunities as bulkArchiveOpportunitiesRequest,
  claimOpportunity as claimOpportunityRequest,
  enqueueAgentAnalysis,
  fetchMessagePage,
  fetchOpportunities,
  fetchOpportunity,
  fetchReplyTemplates,
  generateAIDraft as generateAIDraftRequest,
  restoreOpportunity as restoreOpportunityRequest,
  sendManualReply,
  updateOpportunityStatus as updateOpportunityStatusRequest,
  updateFriendRequest,
} from './api'
import { useAuth } from './auth'
import type {
  ChatMessage,
  ExtractedContacts,
  InternalOpportunityStatus,
  Opportunity,
  ReplyTemplate,
} from './types'

export type WorkMode = 'work' | 'ai'

interface AppStore {
  opportunities: Opportunity[]
  messagesByOpportunity: Record<string, ChatMessage[]>
  messageTotalsByOpportunity: Record<string, number>
  templates: ReplyTemplate[]
  workMode: WorkMode
  newOpportunityId: string | null
  toggleWorkMode: () => void
  setOpportunityStatus: (id: string, status: InternalOpportunityStatus) => Promise<void>
  sendMessage: (opportunityId: string, content: string, idempotencyKey: string) => Promise<void>
  generateAIDraft: (opportunityId: string) => Promise<string>
  claimOpportunity: (opportunityId: string) => Promise<void>
  addTemplate: (template: Omit<ReplyTemplate, 'id'>) => void
  updateTemplate: (template: ReplyTemplate) => void
  updateOpportunity: (id: string, patch: Partial<Opportunity>) => void
  startLinkAnalysis: (id: string) => Promise<void>
  updateContacts: (id: string, contacts: Partial<ExtractedContacts>) => void
  setFriendRequestStatus: (
    id: string,
    status: Exclude<Opportunity['friendRequestStatus'], 'n/a'>,
  ) => Promise<void>
  overrideRiskAndContinue: (id: string) => void
  closeOpportunity: (id: string) => Promise<void>
  archiveOpportunity: (id: string) => Promise<void>
  restoreOpportunity: (id: string) => Promise<void>
  bulkArchiveOpportunities: (ids: string[]) => Promise<void>
  loadOpportunityDetail: (id: string, signal?: AbortSignal) => Promise<void>
  loadMoreMessages: (id: string, offset: number) => Promise<void>
}

const AppStoreContext = createContext<AppStore | null>(null)

export function AppStoreProvider({ children }: { children: React.ReactNode }) {
  const { token } = useAuth()
  const [opportunities, setOpportunities] = useState<Opportunity[]>([])
  const [messagesByOpportunity, setMessagesByOpportunity] = useState<Record<string, ChatMessage[]>>({})
  const [messageTotalsByOpportunity, setMessageTotalsByOpportunity] = useState<Record<string, number>>({})
  const [templates, setTemplates] = useState<ReplyTemplate[]>([])
  const [workMode, setWorkMode] = useState<WorkMode>('work')
  const [newOpportunityId] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setOpportunities([])
    setMessagesByOpportunity({})
    setMessageTotalsByOpportunity({})
    setTemplates([])
    async function loadBackendData() {
      if (!token) {
        return
      }
      try {
        const [backendOpportunities, backendTemplates] = await Promise.all([
          fetchOpportunities('all'),
          fetchReplyTemplates(),
        ])
        if (cancelled) return
        setOpportunities(backendOpportunities)
        setTemplates(backendTemplates)
      } catch (error) {
        console.warn('Failed to load backend data.', error)
        if (!cancelled) {
          setOpportunities([])
          setMessagesByOpportunity({})
        }
      }
    }

    loadBackendData()
    const interval = setInterval(loadBackendData, 30000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [token])

  const toggleWorkMode = useCallback(() => {
    setWorkMode((prev) => (prev === 'work' ? 'ai' : 'work'))
  }, [])

  const setOpportunityStatus = useCallback(async (
    id: string,
    status: InternalOpportunityStatus,
  ) => {
    const updated = await updateOpportunityStatusRequest(id, status)
    setOpportunities((prev) => prev.map((item) => (item.id === id ? updated : item)))
  }, [])

  const updateOpportunity = useCallback((id: string, patch: Partial<Opportunity>) => {
    setOpportunities((prev) => prev.map((o) => (o.id === id ? { ...o, ...patch } : o)))
  }, [])

  const startLinkAnalysis = useCallback(async (id: string) => {
    setOpportunities((prev) =>
      prev.map((o) =>
        o.id === id
          ? { ...o, sopStage: 'analyzing', linkVerification: { ...o.linkVerification, status: 'verifying' } }
          : o,
      ),
    )
    try {
      await enqueueAgentAnalysis(id)
      for (let attempt = 0; attempt < 30; attempt += 1) {
        await new Promise((resolve) => setTimeout(resolve, 2000))
        const updated = await fetchOpportunity(id)
        setOpportunities((prev) => prev.map((item) => (item.id === id ? updated : item)))
        if (
          updated.agentAnalysisStatus === 'completed' ||
          updated.agentAnalysisStatus === 'failed' ||
          updated.agentAnalysisStatus === 'quota_exceeded'
        ) {
          return
        }
      }
      throw new Error('Agent analysis did not finish within the polling window.')
    } catch (error) {
      console.warn('Failed to run pi agent analysis.', error)
      const quotaExceeded = error instanceof Error && error.message.includes('monthly pi agent quota exceeded')
      setOpportunities((prev) =>
        prev.map((item) =>
          item.id === id
            ? {
                ...item,
                agentAnalysisStatus: quotaExceeded ? 'quota_exceeded' : 'failed',
                agentAnalysisError: error instanceof Error ? error.message : 'Agent analysis failed.',
                linkVerification: { ...item.linkVerification, status: 'unverified' },
              }
            : item,
        ),
      )
    }
  }, [])

  const updateContacts = useCallback((id: string, contacts: Partial<ExtractedContacts>) => {
    setOpportunities((prev) =>
      prev.map((o) => {
        if (o.id !== id) return o
        const merged = { ...o.extractedContacts, ...contacts }
        const hasAny = Boolean(merged.phone || merged.email || merged.telegramHandle || merged.wecomId)
        const stageAdvance =
          hasAny && (o.sopStage === 'verified' || o.sopStage === 'detected' || o.sopStage === 'contact_extracted')
            ? o.sourceType === 'private'
              ? 'ready_to_chat'
              : 'contact_extracted'
            : o.sopStage
        return { ...o, extractedContacts: merged, sopStage: stageAdvance }
      }),
    )
  }, [])

  // 好友申请状态流转：真实持久化到后端；"已通过"由操作员确认回填，无任何定时伪造。
  const setFriendRequestStatus = useCallback(
    async (id: string, status: Exclude<Opportunity['friendRequestStatus'], 'n/a'>) => {
      const updated = await updateFriendRequest(id, status)
      setOpportunities((prev) => prev.map((o) => (o.id === id ? updated : o)))
    },
    [],
  )

  const overrideRiskAndContinue = useCallback((id: string) => {
    setOpportunities((prev) =>
      prev.map((o) =>
        o.id === id
          ? {
              ...o,
              sopStage: 'verified',
              linkVerification: {
                ...o.linkVerification,
                status: 'safe',
                resolvedInfo: '人工确认安全：已由操作人员手动核验并确认可以继续跟进（原 AI 判定为可疑）。',
              },
            }
          : o,
      ),
    )
  }, [])

  const closeOpportunity = useCallback(async (id: string) => {
    await setOpportunityStatus(id, 'closed')
  }, [setOpportunityStatus])

  const archiveOpportunity = useCallback(async (id: string) => {
    const updated = await archiveOpportunityRequest(id)
    setOpportunities((prev) => prev.map((item) => (item.id === id ? updated : item)))
  }, [])

  const restoreOpportunity = useCallback(async (id: string) => {
    const updated = await restoreOpportunityRequest(id)
    setOpportunities((prev) => prev.map((item) => (item.id === id ? updated : item)))
  }, [])

  const bulkArchiveOpportunities = useCallback(async (ids: string[]) => {
    const updated = await bulkArchiveOpportunitiesRequest(ids)
    const byId = new Map(updated.map((item) => [item.id, item]))
    setOpportunities((prev) => prev.map((item) => byId.get(item.id) ?? item))
  }, [])

  const loadOpportunityDetail = useCallback(async (id: string, signal?: AbortSignal) => {
    const [detail, messagePage] = await Promise.all([
      fetchOpportunity(id, signal),
      fetchMessagePage(id, { limit: 200, offset: 0, signal }),
    ])
    setOpportunities((prev) => (
      prev.some((item) => item.id === id)
        ? prev.map((item) => (item.id === id ? detail : item))
        : [detail, ...prev]
    ))
    setMessagesByOpportunity((prev) => ({ ...prev, [id]: messagePage.items }))
    setMessageTotalsByOpportunity((prev) => ({ ...prev, [id]: messagePage.total }))
  }, [])

  const loadMoreMessages = useCallback(async (id: string, offset: number) => {
    const messagePage = await fetchMessagePage(id, { limit: 200, offset })
    setMessagesByOpportunity((prev) => {
      const current = prev[id] ?? []
      if (current.length !== offset) return prev
      const currentIds = new Set(current.map((message) => message.id))
      return {
        ...prev,
        [id]: [
          ...current,
          ...messagePage.items.filter((message) => !currentIds.has(message.id)),
        ],
      }
    })
    setMessageTotalsByOpportunity((prev) => ({ ...prev, [id]: messagePage.total }))
  }, [])

  const sendMessage = useCallback(async (
    opportunityId: string,
    content: string,
    idempotencyKey: string,
  ) => {
    const result = await sendManualReply(opportunityId, content, idempotencyKey)
    setMessagesByOpportunity((prev) => ({
      ...prev,
      [opportunityId]: (prev[opportunityId] ?? []).some(
        (message) => message.id === result.message.id,
      )
        ? prev[opportunityId] ?? []
        : [...(prev[opportunityId] ?? []), result.message],
    }))
    setMessageTotalsByOpportunity((prev) => ({
      ...prev,
      [opportunityId]: result.messageTotal,
    }))
    setOpportunities((prev) => prev.map(
      (item) => (item.id === opportunityId ? result.opportunity : item),
    ))
  }, [])

  const generateAIDraft = useCallback(async (opportunityId: string) => {
    const draft = await generateAIDraftRequest(opportunityId)
    setOpportunities((prev) => prev.map(
      (item) => (item.id === opportunityId ? { ...item, aiReplyDraft: draft } : item),
    ))
    return draft
  }, [])

  const claimOpportunity = useCallback(async (opportunityId: string) => {
    const updated = await claimOpportunityRequest(opportunityId)
    setOpportunities((prev) => prev.map(
      (item) => (item.id === opportunityId ? updated : item),
    ))
  }, [])

  const addTemplate = useCallback((template: Omit<ReplyTemplate, 'id'>) => {
    setTemplates((prev) => [{ ...template, id: `tpl-${Date.now()}` }, ...prev])
  }, [])

  const updateTemplate = useCallback((template: ReplyTemplate) => {
    setTemplates((prev) => prev.map((t) => (t.id === template.id ? template : t)))
  }, [])

  const value = useMemo(
    () => ({
      opportunities,
      messagesByOpportunity,
      messageTotalsByOpportunity,
      templates,
      workMode,
      newOpportunityId,
      toggleWorkMode,
      setOpportunityStatus,
      sendMessage,
      generateAIDraft,
      claimOpportunity,
      addTemplate,
      updateTemplate,
      updateOpportunity,
      startLinkAnalysis,
      updateContacts,
      setFriendRequestStatus,
      overrideRiskAndContinue,
      closeOpportunity,
      archiveOpportunity,
      restoreOpportunity,
      bulkArchiveOpportunities,
      loadOpportunityDetail,
      loadMoreMessages,
    }),
    [
      opportunities,
      messagesByOpportunity,
      messageTotalsByOpportunity,
      templates,
      workMode,
      newOpportunityId,
      toggleWorkMode,
      setOpportunityStatus,
      sendMessage,
      generateAIDraft,
      claimOpportunity,
      addTemplate,
      updateTemplate,
      updateOpportunity,
      startLinkAnalysis,
      updateContacts,
      setFriendRequestStatus,
      overrideRiskAndContinue,
      closeOpportunity,
      archiveOpportunity,
      restoreOpportunity,
      bulkArchiveOpportunities,
      loadOpportunityDetail,
      loadMoreMessages,
    ],
  )

  return <AppStoreContext.Provider value={value}>{children}</AppStoreContext.Provider>
}

export function useAppStore() {
  const ctx = useContext(AppStoreContext)
  if (!ctx) {
    throw new Error('useAppStore must be used within AppStoreProvider')
  }
  return ctx
}
