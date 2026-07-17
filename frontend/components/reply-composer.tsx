'use client'

import { Send, Sparkles } from 'lucide-react'
import { useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { generateOpportunityAiDraft } from '@/lib/api'
import { useAppStore } from '@/lib/app-store'
import type { Opportunity } from '@/lib/types'

function fillTemplate(content: string, opportunity: Opportunity) {
  return content
    .replaceAll('{{联系人姓名}}', opportunity.contactName)
    .replaceAll('{{团队规模}}', '200 人')
    .replaceAll('{{公司名称}}', '贵公司')
    .replaceAll('{{工作时间}}', '周一至周五 9:00-18:00')
}

export function ReplyComposer({ opportunity }: { opportunity: Opportunity }) {
  const { generateAIDraft, templates, sendMessage } = useAppStore()
  const [draft, setDraft] = useState(opportunity.aiReplyDraft ?? '')
  const [sending, setSending] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState('')
  const pendingRequest = useRef<{ content: string; key: string } | null>(null)

  const quickTemplates = templates.slice(0, 6)
  const replyDisabled = Boolean(opportunity.archivedAt) || ['closed', 'ignored'].includes(
    opportunity.internalStatus,
  )

  const handleGenerate = async () => {
    if (generating || replyDisabled) return
    setError('')
    setGenerating(true)
    try {
      const generated = await generateAIDraft(opportunity.id)
      pendingRequest.current = null
      setDraft(generated)
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : 'AI 草稿生成失败')
    } finally {
      setGenerating(false)
    }
  }

  const handleSend = async () => {
    const content = draft.trim()
    if (!content || sending) return
    setError('')
    setSending(true)
    const request = pendingRequest.current?.content === content
      ? pendingRequest.current
      : { content, key: crypto.randomUUID() }
    pendingRequest.current = request
    try {
      await sendMessage(opportunity.id, content, request.key)
      pendingRequest.current = null
      setDraft('')
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '消息发送失败')
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="flex flex-col gap-2.5 border-t bg-card p-3 md:p-4">
      <div className="flex gap-2 overflow-x-auto pb-1" role="list" aria-label="推荐模板">
        {quickTemplates.map((tpl) => (
          <button
            key={tpl.id}
            type="button"
            role="listitem"
            onClick={() => {
              pendingRequest.current = null
              setDraft(fillTemplate(tpl.content, opportunity))
            }}
            disabled={replyDisabled || sending || generating}
            className="shrink-0 rounded-full border bg-secondary px-3 py-1 text-xs text-secondary-foreground transition-colors hover:border-primary/40 hover:bg-accent hover:text-accent-foreground"
          >
            {tpl.title}
          </button>
        ))}
      </div>
      <Textarea
        value={draft}
        onChange={(e) => {
          if (pendingRequest.current?.content !== e.target.value.trim()) {
            pendingRequest.current = null
          }
          setDraft(e.target.value)
        }}
        onKeyDown={(e) => {
          if (
            e.key === 'Enter' &&
            !e.shiftKey &&
            !e.nativeEvent.isComposing &&
            e.keyCode !== 229
          ) {
            e.preventDefault()
            void handleSend()
          }
        }}
        placeholder="输入回复内容，或选择模板 / 使用 AI 生成草稿…"
        className="min-h-20 resize-none text-sm"
        aria-label="回复内容"
        disabled={replyDisabled || sending}
      />
      {replyDisabled && (
        <p className="text-xs text-muted-foreground">该商机当前状态不可回复，请先恢复为待处理。</p>
      )}
      {error && <p className="text-xs text-destructive">{error}</p>}
      <div className="flex items-center justify-between gap-2">
        <Button variant="outline" size="sm" onClick={() => void handleGenerate()} disabled={generating || sending || replyDisabled} className="gap-1.5 bg-transparent">
          <Sparkles className="size-3.5" />
          {generating ? '生成中…' : 'AI 生成草稿'}
        </Button>
        <Button size="sm" onClick={() => void handleSend()} disabled={!draft.trim() || sending || generating || replyDisabled} className="gap-1.5">
          <Send className="size-3.5" />
          {sending ? '发送中…' : '发送'}
        </Button>
      </div>
    </div>
  )
}
