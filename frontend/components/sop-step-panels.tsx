'use client'

import {
  AlertTriangle,
  Bot,
  Check,
  ExternalLink,
  Link2,
  Loader2,
  Lock,
  Mail,
  MessageCircle,
  Phone,
  RefreshCw,
  Send,
  ShieldAlert,
  ShieldCheck,
  UserPlus,
  Users,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { ChatBubble } from '@/components/chat-bubble'
import { PlatformIcon, platformLabel } from '@/components/platform-icon'
import { ReplyComposer } from '@/components/reply-composer'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { useAppStore } from '@/lib/app-store'
import { formatDateTime, friendRequestConfig, linkStatusConfig } from '@/lib/sop'
import type { SopStep } from '@/lib/sop-steps'
import type { Opportunity } from '@/lib/types'
import { cn } from '@/lib/utils'

// ===== Step 1：商机发现 =====
export function StepDiscovery({ opportunity }: { opportunity: Opportunity }) {
  const { messagesByOpportunity } = useAppStore()
  const firstMessage = (messagesByOpportunity[opportunity.id] ?? []).find((m) => m.isFromContact)

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-2 text-sm sm:grid-cols-2">
        <InfoRow label="消息来源">
          <span className="flex items-center gap-1.5">
            {opportunity.sourceType === 'group' ? (
              <>
                <Users className="size-3.5 text-muted-foreground" />
                群消息 · {opportunity.groupName}
              </>
            ) : (
              <>
                <MessageCircle className="size-3.5 text-muted-foreground" />
                私聊消息
              </>
            )}
          </span>
        </InfoRow>
        <InfoRow label="平台">
          <span className="flex items-center gap-1.5">
            <PlatformIcon platform={opportunity.platform} className="size-4" />
            {platformLabel(opportunity.platform)}
          </span>
        </InfoRow>
        {opportunity.sourceType === 'group' && (
          <InfoRow label="发送者身份">
            {opportunity.groupMemberRole === 'member' ? '群成员（未添加好友）' : '身份未知（非好友、入群时间短）'}
          </InfoRow>
        )}
        <InfoRow label="发现时间">{formatDateTime(opportunity.createdAt)}</InfoRow>
      </div>

      <div className="rounded-lg border bg-muted/40 p-3">
        <p className="mb-1.5 text-xs font-medium text-muted-foreground">原始消息内容</p>
        <p className="text-pretty text-sm leading-relaxed">{firstMessage?.content ?? opportunity.lastMessagePreview}</p>
      </div>

      {opportunity.rawMessageLinks.length > 0 && (
        <div>
          <p className="mb-1.5 text-xs font-medium text-muted-foreground">消息中包含的链接</p>
          <div className="flex flex-col gap-1.5">
            {opportunity.rawMessageLinks.map((link) => (
              <span
                key={link}
                className="flex items-center gap-1.5 break-all rounded-md border bg-secondary px-2.5 py-1.5 font-mono text-xs text-secondary-foreground"
              >
                <Link2 className="size-3.5 shrink-0 text-muted-foreground" />
                {link}
              </span>
            ))}
          </div>
        </div>
      )}

      {opportunity.agentAnalysisStatus === 'failed' && (
        <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 p-3">
          <p className="text-sm font-medium text-destructive">pi Agent 分析失败</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {opportunity.agentAnalysisError ?? '本次分析没有产生有效结果，请稍后重新分析。'}
          </p>
        </div>
      )}

      {opportunity.agentActions.length > 0 && (
        <div className="rounded-lg border bg-primary/5 p-3">
          <p className="mb-2 text-xs font-medium text-muted-foreground">pi Agent 后续行动建议</p>
          <ul className="flex flex-col gap-2">
            {opportunity.agentActions.map((action, index) => (
              <li key={`${action.actionType}-${index}`} className="rounded-md bg-background/80 p-2.5 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">
                    {{
                      send_email: '主动发送邮件',
                      add_friend: '申请添加好友',
                      private_message: '主动私信',
                      notify_user: '提醒当前用户',
                    }[action.actionType]}
                  </span>
                  <Badge variant="outline" className="h-5 text-[10px]">
                    {action.requiresApproval ? '需人工批准' : '内部提醒'}
                  </Badge>
                </div>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{action.reason}</p>
                {action.target && <p className="mt-1 text-xs">目标：{action.target}</p>}
                {action.draft && <p className="mt-2 rounded border bg-muted/40 p-2 text-xs">建议草稿：{action.draft}</p>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

// ===== Step 2：链接安全分析 =====
export function StepLinkVerification({ opportunity, step }: { opportunity: Opportunity; step: SopStep }) {
  const { startLinkAnalysis, overrideRiskAndContinue, closeOpportunity } = useAppStore()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const lv = opportunity.linkVerification

  if (step.state === 'skipped') {
    return <SkippedNote text="该消息不包含任何链接，已自动跳过安全核验步骤。" />
  }

  const statusCfg = linkStatusConfig[lv.status]
  const isRisk = lv.status === 'suspicious' || lv.status === 'malicious'

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs text-muted-foreground">分析状态</span>
        <Badge variant="outline" className={cn('h-5 gap-1 px-1.5 text-[10px]', statusCfg.className)}>
          {lv.status === 'verifying' && <Loader2 className="size-3 animate-spin" />}
          {lv.status === 'safe' && <ShieldCheck className="size-3" />}
          {isRisk && <ShieldAlert className="size-3" />}
          {statusCfg.label}
        </Badge>
        {lv.verifiedAt && <span className="text-[11px] text-muted-foreground">核验于 {formatDateTime(lv.verifiedAt)}</span>}
      </div>

      {lv.status === 'unverified' && (
        <div className="flex flex-col items-start gap-3 rounded-lg border border-dashed p-4">
          <p className="text-sm text-muted-foreground">链接尚未核验。启动 AI/SOP 分析以检测钓鱼、欺诈风险并解析真实商机内容。</p>
          <Button size="sm" onClick={() => startLinkAnalysis(opportunity.id)} className="gap-1.5">
            <Bot className="size-3.5" />
            开始 AI 安全分析
          </Button>
        </div>
      )}

      {lv.status === 'verifying' && (
        <div className="flex items-center gap-3 rounded-lg border bg-primary/5 p-4">
          <Loader2 className="size-5 animate-spin text-primary" />
          <div>
            <p className="text-sm font-medium">AI 正在分析链接…</p>
            <p className="mt-0.5 text-xs text-muted-foreground">检测域名信誉、页面内容真实性与钓鱼特征</p>
          </div>
        </div>
      )}

      {lv.status === 'safe' && (
        <div className="rounded-lg border border-success/30 bg-success/5 p-4">
          <p className="flex items-center gap-1.5 text-sm font-medium text-success">
            <ShieldCheck className="size-4" />
            链接安全，可以继续推进
          </p>
          {lv.resolvedInfo && (
            <div className="mt-2.5 border-t border-success/20 pt-2.5">
              <p className="mb-1 text-xs font-medium text-muted-foreground">解析出的商机详情摘要</p>
              <p className="text-pretty text-sm leading-relaxed">{lv.resolvedInfo}</p>
            </div>
          )}
        </div>
      )}

      {isRisk && (
        <>
          <div
            role="alert"
            className="flex items-start gap-2.5 rounded-lg border border-destructive/40 bg-destructive/10 p-4"
          >
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
            <div>
              <p className="text-sm font-semibold text-destructive">该链接存在风险，建议忽略此商机</p>
              <p className="mt-0.5 text-xs leading-relaxed text-destructive/90">
                流程已在此步骤中断，后续步骤（联系方式提取、建立联系、对话）均已锁定。
              </p>
            </div>
          </div>
          <div>
            <p className="mb-1.5 text-xs font-medium text-muted-foreground">风险原因</p>
            <ul className="flex flex-col gap-1.5">
              {lv.riskReasons.map((reason) => (
                <li key={reason} className="flex items-start gap-2 text-sm leading-relaxed">
                  <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-destructive" aria-hidden="true" />
                  {reason}
                </li>
              ))}
            </ul>
          </div>
        </>
      )}

      {(lv.status === 'safe' || isRisk) && (
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => startLinkAnalysis(opportunity.id)} className="gap-1.5 bg-transparent">
            <RefreshCw className="size-3.5" />
            重新分析
          </Button>
          {isRisk && (
            <>
              <Button variant="outline" size="sm" onClick={() => closeOpportunity(opportunity.id)} className="bg-transparent">
                忽略此商机
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setConfirmOpen(true)}
                className="text-muted-foreground"
              >
                仍要继续（人工确认安全）
              </Button>
              <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogMedia className="bg-destructive/10 text-destructive">
                      <AlertTriangle />
                    </AlertDialogMedia>
                    <AlertDialogTitle>确认忽略风险警告？</AlertDialogTitle>
                    <AlertDialogDescription>
                      AI 已将该链接判定为{lv.status === 'malicious' ? '高风险（恶意站点）' : '可疑'}
                      。继续推进意味着您已人工核验并自行承担风险，该操作将被记录。
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>取消</AlertDialogCancel>
                    <Button
                      variant="destructive"
                      onClick={() => {
                        overrideRiskAndContinue(opportunity.id)
                        setConfirmOpen(false)
                      }}
                    >
                      我已人工确认，继续推进
                    </Button>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </>
          )}
        </div>
      )}
    </div>
  )
}

// ===== Step 3：联系方式提取 =====
const contactFields = [
  { key: 'phone', label: '手机号', icon: Phone },
  { key: 'email', label: '邮箱', icon: Mail },
  { key: 'telegramHandle', label: 'Telegram 账号', icon: Send },
  { key: 'wecomId', label: '企业微信号', icon: MessageCircle },
] as const

const extractionSourceLabel = {
  message_text: '消息文本',
  link_content: '链接内容',
  sop_manual: '人工补充',
} as const

export function StepContacts({ opportunity, step }: { opportunity: Opportunity; step: SopStep }) {
  const { updateContacts } = useAppStore()
  const [drafts, setDrafts] = useState<Record<string, string>>({})

  if (step.state === 'blocked') {
    return <BlockedNote text={step.blockReason ?? '需先完成前置步骤'} />
  }

  const contacts = opportunity.extractedContacts
  const hasAny = Boolean(contacts.phone || contacts.email || contacts.telegramHandle || contacts.wecomId)

  const handleSave = (key: (typeof contactFields)[number]['key']) => {
    const value = drafts[key]?.trim()
    if (!value) return
    updateContacts(opportunity.id, { [key]: value, extractionSource: 'sop_manual' })
    setDrafts((d) => ({ ...d, [key]: '' }))
  }

  return (
    <div className="flex flex-col gap-3">
      {!hasAny && (
        <div className="flex items-start gap-2.5 rounded-lg border border-warning/40 bg-warning/10 p-3">
          <AlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" />
          <p className="text-xs leading-relaxed">
            尚未获取到任何联系方式，此步骤处于<span className="font-semibold">待补充</span>状态，将阻塞后续步骤。
            请人工补充至少一项关键联系方式。
          </p>
        </div>
      )}
      <div className="grid gap-2.5 sm:grid-cols-2">
        {contactFields.map((field) => {
          const value = contacts[field.key]
          const Icon = field.icon
          return (
            <div key={field.key} className="rounded-lg border p-3">
              <p className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                <Icon className="size-3.5" />
                {field.label}
              </p>
              {value ? (
                <div className="flex flex-wrap items-center gap-2">
                  <p className="break-all font-mono text-sm">{value}</p>
                  {contacts.extractionSource && (
                    <Badge variant="secondary" className="h-4.5 px-1.5 text-[10px] font-normal">
                      来源：{extractionSourceLabel[contacts.extractionSource]}
                    </Badge>
                  )}
                </div>
              ) : (
                <div className="flex flex-col gap-1.5">
                  <p className="text-xs text-muted-foreground">未获取到</p>
                  <div className="flex gap-1.5">
                    <Input
                      value={drafts[field.key] ?? ''}
                      onChange={(e) => setDrafts((d) => ({ ...d, [field.key]: e.target.value }))}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && !e.nativeEvent.isComposing && e.keyCode !== 229) {
                          e.preventDefault()
                          handleSave(field.key)
                        }
                      }}
                      placeholder="手动补充…"
                      className="h-8 text-xs"
                      aria-label={`手动补充${field.label}`}
                    />
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 shrink-0 bg-transparent px-2.5"
                      disabled={!drafts[field.key]?.trim()}
                      onClick={() => handleSave(field.key)}
                    >
                      <Check className="size-3.5" />
                      <span className="sr-only">保存{field.label}</span>
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ===== Step 4：建立联系 =====
export function StepFriendRequest({ opportunity, step }: { opportunity: Opportunity; step: SopStep }) {
  const { templates, sendFriendRequest, updateOpportunity, closeOpportunity } = useAppStore()
  const [greeting, setGreeting] = useState(
    `您好 ${opportunity.contactName}，我在「${opportunity.groupName ?? '群聊'}」看到您发布的需求，我们正好提供相关解决方案，希望能加个好友详细沟通。`,
  )

  if (step.state === 'skipped') {
    return <SkippedNote text="该商机来自私聊，双方已可直接对话，无需发送好友申请。" />
  }
  if (step.state === 'blocked') {
    return <BlockedNote text={step.blockReason ?? '需先完成前置步骤'} />
  }

  const frStatus = friendRequestConfig[opportunity.friendRequestStatus]
  const greetingTemplates = templates.filter((t) => t.category === '开场白')

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground">申请状态</span>
        <Badge variant="outline" className={cn('h-5 gap-1 px-1.5 text-[10px]', frStatus.className)}>
          {opportunity.friendRequestStatus === 'pending' && <Loader2 className="size-3 animate-spin" />}
          {frStatus.label}
        </Badge>
      </div>

      {opportunity.friendRequestStatus === 'not_sent' && (
        <div className="flex flex-col gap-2.5">
          <div>
            <p className="mb-1.5 text-xs font-medium text-muted-foreground">打招呼语（可编辑）</p>
            <Textarea
              value={greeting}
              onChange={(e) => setGreeting(e.target.value)}
              className="min-h-20 resize-none text-sm"
              aria-label="好友申请打招呼语"
            />
          </div>
          <div className="flex gap-2 overflow-x-auto pb-1" role="list" aria-label="推荐打招呼模板">
            {greetingTemplates.map((tpl) => (
              <button
                key={tpl.id}
                type="button"
                role="listitem"
                onClick={() =>
                  setGreeting(
                    tpl.content
                      .replaceAll('{{联系人姓名}}', opportunity.contactName)
                      .replaceAll('{{群名称}}', opportunity.groupName ?? '群聊'),
                  )
                }
                className="shrink-0 rounded-full border bg-secondary px-3 py-1 text-xs text-secondary-foreground transition-colors hover:border-primary/40 hover:bg-accent hover:text-accent-foreground"
              >
                {tpl.title}
              </button>
            ))}
          </div>
          <Button size="sm" onClick={() => sendFriendRequest(opportunity.id)} className="w-fit gap-1.5">
            <UserPlus className="size-3.5" />
            发送好友申请
          </Button>
        </div>
      )}

      {opportunity.friendRequestStatus === 'pending' && (
        <div className="flex items-center gap-3 rounded-lg border bg-warning/5 p-4">
          <Loader2 className="size-5 animate-spin text-warning" />
          <div>
            <p className="text-sm font-medium">好友申请已发送，等待对方通过</p>
            <p className="mt-0.5 text-xs text-muted-foreground">通过后将自动解锁对话步骤（演示环境约 4 秒后自动通过）</p>
          </div>
        </div>
      )}

      {opportunity.friendRequestStatus === 'accepted' && (
        <div className="flex items-center gap-2.5 rounded-lg border border-success/30 bg-success/5 p-4">
          <ShieldCheck className="size-5 text-success" />
          <p className="text-sm font-medium text-success">好友申请已通过，可以开始对话</p>
        </div>
      )}

      {opportunity.friendRequestStatus === 'rejected' && (
        <div className="flex flex-col gap-3">
          <div className="flex items-start gap-2.5 rounded-lg border border-destructive/40 bg-destructive/10 p-3.5">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-destructive" />
            <p className="text-sm text-destructive">对方拒绝了好友申请。可尝试更换渠道联系，或关闭此商机。</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5 bg-transparent"
              onClick={() => updateOpportunity(opportunity.id, { friendRequestStatus: 'not_sent' })}
            >
              <RefreshCw className="size-3.5" />
              更换渠道重试
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground"
              onClick={() => closeOpportunity(opportunity.id)}
            >
              标记为无法触达，关闭此商机
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

// ===== Step 5：聊天与回复 =====
export function StepChat({ opportunity, step }: { opportunity: Opportunity; step: SopStep }) {
  const { messagesByOpportunity } = useAppStore()
  const messages = messagesByOpportunity[opportunity.id] ?? []
  const scrollRef = useRef<HTMLDivElement>(null)
  const unlocked = step.state !== 'locked'

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages.length])

  if (!unlocked) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed bg-muted/30 py-12 text-center">
        <span className="flex size-12 items-center justify-center rounded-full bg-muted">
          <Lock className="size-5 text-muted-foreground" />
        </span>
        <div className="px-6">
          <p className="text-sm font-medium text-muted-foreground">对话尚未解锁</p>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{step.blockReason}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col overflow-hidden rounded-lg border">
      <div className="border-b bg-muted/40 px-4 py-2.5">
        <p className="text-xs font-medium text-muted-foreground">聊天上下文</p>
      </div>
      <div ref={scrollRef} className="flex max-h-96 min-h-48 flex-col gap-4 overflow-y-auto p-4">
        {messages.map((message) => (
          <ChatBubble key={message.id} message={message} />
        ))}
      </div>
      <ReplyComposer opportunity={opportunity} />
    </div>
  )
}

// ===== 通用小组件 =====
function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-sm">{children}</span>
    </div>
  )
}

function SkippedNote({ text }: { text: string }) {
  return (
    <div className="flex items-center gap-2.5 rounded-lg border border-dashed bg-muted/30 p-3.5">
      <ExternalLink className="size-4 shrink-0 text-muted-foreground" />
      <p className="text-sm text-muted-foreground">{text}</p>
    </div>
  )
}

function BlockedNote({ text }: { text: string }) {
  return (
    <div className="flex items-center gap-2.5 rounded-lg border border-dashed bg-muted/30 p-3.5">
      <Lock className="size-4 shrink-0 text-muted-foreground" />
      <p className="text-sm text-muted-foreground">{text}</p>
    </div>
  )
}
