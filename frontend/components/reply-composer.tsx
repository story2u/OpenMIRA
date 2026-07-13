'use client'

import { Send, Sparkles } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useAppStore } from '@/lib/app-store'
import type { Opportunity } from '@/lib/types'

function fillTemplate(content: string, opportunity: Opportunity) {
  return content
    .replaceAll('{{联系人姓名}}', opportunity.contactName)
    .replaceAll('{{团队规模}}', '200 人')
    .replaceAll('{{公司名称}}', '贵公司')
    .replaceAll('{{工作时间}}', '周一至周五 9:00-18:00')
}

function buildAiDraft(opportunity: Opportunity) {
  return `${opportunity.contactName} 您好！关于您提到的「${opportunity.matchedKeywords.join('、')}」需求，我们已经为类似规模的客户提供过成熟方案。我可以为您整理一份针对性的介绍资料，并安排一次 15 分钟的快速沟通，请问您明天上午方便吗？`
}

export function ReplyComposer({ opportunity }: { opportunity: Opportunity }) {
  const { templates, sendMessage } = useAppStore()
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState('')

  const quickTemplates = templates.slice(0, 6)

  const handleGenerate = () => {
    setGenerating(true)
    setDraft('')
    const full = buildAiDraft(opportunity)
    let i = 0
    const interval = setInterval(() => {
      i += 3
      setDraft(full.slice(0, i))
      if (i >= full.length) {
        clearInterval(interval)
        setGenerating(false)
      }
    }, 25)
  }

  const handleSend = async () => {
    const content = draft.trim()
    if (!content || sending) return
    setError('')
    setSending(true)
    try {
      await sendMessage(opportunity.id, content, 'human')
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
            onClick={() => setDraft(fillTemplate(tpl.content, opportunity))}
            className="shrink-0 rounded-full border bg-secondary px-3 py-1 text-xs text-secondary-foreground transition-colors hover:border-primary/40 hover:bg-accent hover:text-accent-foreground"
          >
            {tpl.title}
          </button>
        ))}
      </div>
      <Textarea
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
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
      />
      {error && <p className="text-xs text-destructive">{error}</p>}
      <div className="flex items-center justify-between gap-2">
        <Button variant="outline" size="sm" onClick={handleGenerate} disabled={generating} className="gap-1.5 bg-transparent">
          <Sparkles className="size-3.5" />
          {generating ? '生成中…' : 'AI 生成草稿'}
        </Button>
        <Button size="sm" onClick={() => void handleSend()} disabled={!draft.trim() || sending} className="gap-1.5">
          <Send className="size-3.5" />
          {sending ? '发送中…' : '发送'}
        </Button>
      </div>
    </div>
  )
}
