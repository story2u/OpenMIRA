'use client'

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { fetchOpportunities, fetchReplyTemplates } from './api'
import { useAuth } from './auth'
import { mockMessages, mockOpportunities, mockTemplates } from './mock-data'
import type {
  ChatMessage,
  ExtractedContacts,
  Opportunity,
  OpportunityStatus,
  ReplyTemplate,
} from './types'

export type WorkMode = 'work' | 'ai'

interface AppStore {
  opportunities: Opportunity[]
  messagesByOpportunity: Record<string, ChatMessage[]>
  templates: ReplyTemplate[]
  workMode: WorkMode
  newOpportunityId: string | null
  toggleWorkMode: () => void
  setOpportunityStatus: (id: string, status: OpportunityStatus) => void
  sendMessage: (opportunityId: string, content: string, source: 'human' | 'ai') => void
  addTemplate: (template: Omit<ReplyTemplate, 'id'>) => void
  updateTemplate: (template: ReplyTemplate) => void
  updateOpportunity: (id: string, patch: Partial<Opportunity>) => void
  startLinkAnalysis: (id: string) => void
  updateContacts: (id: string, contacts: Partial<ExtractedContacts>) => void
  sendFriendRequest: (id: string) => void
  overrideRiskAndContinue: (id: string) => void
  closeOpportunity: (id: string) => void
}

const AppStoreContext = createContext<AppStore | null>(null)

export function AppStoreProvider({ children }: { children: React.ReactNode }) {
  const { token } = useAuth()
  const [opportunities, setOpportunities] = useState<Opportunity[]>(mockOpportunities)
  const [messagesByOpportunity, setMessagesByOpportunity] = useState<Record<string, ChatMessage[]>>(mockMessages)
  const [templates, setTemplates] = useState<ReplyTemplate[]>(mockTemplates)
  const [workMode, setWorkMode] = useState<WorkMode>('work')
  const [newOpportunityId] = useState<string | null>(null)
  const timersRef = useRef<ReturnType<typeof setTimeout>[]>([])

  useEffect(() => {
    let cancelled = false
    async function loadBackendData() {
      if (!token) {
        setOpportunities([])
        setMessagesByOpportunity({})
        return
      }
      try {
        const [backendOpportunities, backendTemplates] = await Promise.all([
          fetchOpportunities(),
          fetchReplyTemplates(),
        ])
        if (cancelled) return
        setOpportunities(backendOpportunities)
        setTemplates(backendTemplates.length > 0 ? backendTemplates : mockTemplates)
        setMessagesByOpportunity({})
      } catch (error) {
        console.warn('Failed to load backend data, using mock data.', error)
      }
    }

    loadBackendData()
    const interval = setInterval(loadBackendData, 30000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [token])

  useEffect(() => {
    const timers = timersRef.current
    return () => timers.forEach(clearTimeout)
  }, [])

  const toggleWorkMode = useCallback(() => {
    setWorkMode((prev) => (prev === 'work' ? 'ai' : 'work'))
  }, [])

  const setOpportunityStatus = useCallback((id: string, status: OpportunityStatus) => {
    setOpportunities((prev) => prev.map((o) => (o.id === id ? { ...o, status } : o)))
  }, [])

  const updateOpportunity = useCallback((id: string, patch: Partial<Opportunity>) => {
    setOpportunities((prev) => prev.map((o) => (o.id === id ? { ...o, ...patch } : o)))
  }, [])

  // 模拟链接安全分析：2.5 秒后返回结果
  const startLinkAnalysis = useCallback((id: string) => {
    setOpportunities((prev) =>
      prev.map((o) =>
        o.id === id
          ? { ...o, sopStage: 'analyzing', linkVerification: { ...o.linkVerification, status: 'verifying' } }
          : o,
      ),
    )
    const timer = setTimeout(() => {
      setOpportunities((prev) =>
        prev.map((o) => {
          if (o.id !== id) return o
          // opp-risk 演示分支：始终返回可疑
          if (o.id === 'opp-risk') {
            return {
              ...o,
              sopStage: 'analyzing',
              trustScore: 23,
              linkVerification: {
                status: 'suspicious',
                verifiedAt: new Date().toISOString(),
                riskReasons: [
                  '域名注册时间仅 11 天，无备案信息',
                  '页面要求填写手机号与验证码，疑似钓鱼站点',
                  '页面内容与"企业订单资源"描述不符，实际为推广注册页',
                ],
                resolvedInfo: null,
              },
            }
          }
          return {
            ...o,
            sopStage: 'verified',
            trustScore: Math.max(o.trustScore, 82),
            linkVerification: {
              status: 'safe',
              verifiedAt: new Date().toISOString(),
              riskReasons: [],
              resolvedInfo: '落地页为正规企业页面，内容与消息描述一致，未发现钓鱼或欺诈特征。',
            },
          }
        }),
      )
    }, 2500)
    timersRef.current.push(timer)
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

  // 模拟好友申请：发送后 4 秒自动通过（演示用）
  const sendFriendRequest = useCallback((id: string) => {
    setOpportunities((prev) =>
      prev.map((o) => (o.id === id ? { ...o, friendRequestStatus: 'pending', sopStage: 'friend_requested' } : o)),
    )
    const timer = setTimeout(() => {
      setOpportunities((prev) =>
        prev.map((o) =>
          o.id === id && o.friendRequestStatus === 'pending'
            ? { ...o, friendRequestStatus: 'accepted', sopStage: 'ready_to_chat' }
            : o,
        ),
      )
    }, 4000)
    timersRef.current.push(timer)
  }, [])

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

  const closeOpportunity = useCallback((id: string) => {
    setOpportunities((prev) =>
      prev.map((o) => (o.id === id ? { ...o, sopStage: 'closed', status: 'ignored' } : o)),
    )
  }, [])

  const sendMessage = useCallback((opportunityId: string, content: string, source: 'human' | 'ai') => {
    const message: ChatMessage = {
      id: `msg-${Date.now()}`,
      senderName: '商机助手',
      content,
      isFromContact: false,
      sentAt: new Date().toISOString(),
      source,
    }
    setMessagesByOpportunity((prev) => ({
      ...prev,
      [opportunityId]: [...(prev[opportunityId] ?? []), message],
    }))
    setOpportunities((prev) =>
      prev.map((o) =>
        o.id === opportunityId
          ? { ...o, status: 'replied' as const, sopStage: 'chatting' as const, lastMessagePreview: content }
          : o,
      ),
    )
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
      templates,
      workMode,
      newOpportunityId,
      toggleWorkMode,
      setOpportunityStatus,
      sendMessage,
      addTemplate,
      updateTemplate,
      updateOpportunity,
      startLinkAnalysis,
      updateContacts,
      sendFriendRequest,
      overrideRiskAndContinue,
      closeOpportunity,
    }),
    [
      opportunities,
      messagesByOpportunity,
      templates,
      workMode,
      newOpportunityId,
      toggleWorkMode,
      setOpportunityStatus,
      sendMessage,
      addTemplate,
      updateTemplate,
      updateOpportunity,
      startLinkAnalysis,
      updateContacts,
      sendFriendRequest,
      overrideRiskAndContinue,
      closeOpportunity,
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
