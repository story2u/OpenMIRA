'use client'

import { Archive, ArchiveRestore, ArrowLeft, Loader2, MessageCircle, MessageCircleOff, Users } from 'lucide-react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useState } from 'react'
import { ConfidenceRing } from '@/components/confidence-ring'
import { PlatformIcon, platformLabel } from '@/components/platform-icon'
import { SopStepper } from '@/components/sop-stepper'
import { TrustBadge } from '@/components/trust-badge'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useAppStore } from '@/lib/app-store'
import { sopStageConfig } from '@/lib/sop'

export default function OpportunityDetailPage() {
  const params = useParams<{ id: string }>()
  const { opportunities, setOpportunityStatus, archiveOpportunity, restoreOpportunity } = useAppStore()
  const [archivePending, setArchivePending] = useState(false)
  const [archiveError, setArchiveError] = useState<string | null>(null)
  const opportunity = opportunities.find((o) => o.id === params.id)

  if (!opportunity) {
    return (
      <div className="flex flex-col items-center gap-4 py-20 text-center">
        <MessageCircleOff className="size-10 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">未找到该商机，可能已被移除</p>
        <Button variant="outline" nativeButton={false} render={<Link href="/" />}>
          返回看板
        </Button>
      </div>
    )
  }

  const stage = sopStageConfig[opportunity.sopStage]

  async function toggleArchive(opportunityId: string, archived: boolean) {
    setArchivePending(true)
    setArchiveError(null)
    try {
      if (archived) await restoreOpportunity(opportunityId)
      else await archiveOpportunity(opportunityId)
    } catch (error) {
      setArchiveError(error instanceof Error ? error.message : '归档操作失败，请稍后重试。')
    } finally {
      setArchivePending(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-4 md:px-8 md:py-6">
      <div className="mb-4 flex items-center gap-2">
        <Button variant="ghost" size="icon" aria-label="返回看板" nativeButton={false} render={<Link href="/" />}>
          <ArrowLeft className="size-4" />
        </Button>
        <h1 className="text-base font-semibold md:text-lg">商机详情</h1>
        <Badge variant="secondary" className="ml-1 gap-1.5 rounded-full px-2.5 text-[11px]">
          <span className={`size-1.5 rounded-full ${stage.dotClass}`} aria-hidden="true" />
          {stage.label}
        </Badge>
        {opportunity.archivedAt && <Badge variant="outline">已归档</Badge>}
        <Button variant="outline" size="sm" className="ml-auto gap-1.5" disabled={archivePending} onClick={() => toggleArchive(opportunity.id, Boolean(opportunity.archivedAt))}>
          {archivePending ? <Loader2 className="size-3.5 animate-spin" /> : opportunity.archivedAt ? <ArchiveRestore className="size-3.5" /> : <Archive className="size-3.5" />}
          {opportunity.archivedAt ? '恢复' : '归档'}
        </Button>
      </div>

      {archiveError && (
        <p role="alert" className="mb-4 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {archiveError}
        </p>
      )}

      {opportunity.archivedAt && (
        <div className="mb-4 rounded-lg border bg-muted/40 px-4 py-3 text-sm">
          <p className="font-medium">该商机已归档</p>
          <p className="mt-1 text-xs text-muted-foreground">原状态和历史记录均已保留。恢复后才能继续分析、回复或修改状态。</p>
          {opportunity.archiveReason && <p className="mt-2 text-xs text-muted-foreground">归档原因：{opportunity.archiveReason}</p>}
        </div>
      )}

      {/* 联系人基础信息卡 */}
      <Card className="mb-4 gap-3 rounded-xl p-4 shadow-sm">
        <div className="flex flex-wrap items-center gap-3">
          <Avatar className="size-12">
            <AvatarImage src={opportunity.contactAvatar || '/placeholder.svg'} alt="" />
            <AvatarFallback>{opportunity.contactName.slice(0, 1)}</AvatarFallback>
          </Avatar>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-sm font-semibold">{opportunity.contactName}</p>
              <TrustBadge score={opportunity.trustScore} />
            </div>
            <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
              <span className="flex items-center gap-1">
                <PlatformIcon platform={opportunity.platform} className="size-3.5" />
                {platformLabel(opportunity.platform)}
              </span>
              <span aria-hidden="true">·</span>
              {opportunity.sourceType === 'group' ? (
                <span className="flex items-center gap-1">
                  <Users className="size-3.5" />
                  群消息 · {opportunity.groupName}
                </span>
              ) : (
                <span className="flex items-center gap-1">
                  <MessageCircle className="size-3.5" />
                  私聊消息
                </span>
              )}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex flex-col items-center gap-0.5">
              <ConfidenceRing score={opportunity.confidenceScore} />
              <span className="text-[10px] text-muted-foreground">商机相关度</span>
            </div>
          </div>
        </div>
        <p className="text-pretty text-sm leading-relaxed text-muted-foreground">{opportunity.summary}</p>
        <div className="flex flex-wrap items-center gap-1.5">
          {opportunity.matchedKeywords.map((keyword) => (
            <Badge key={keyword} variant="secondary" className="h-5 rounded-md px-1.5 text-[10px] font-normal">
              {keyword}
            </Badge>
          ))}
          {!opportunity.archivedAt && opportunity.status === 'pending' && opportunity.sopStage !== 'closed' && (
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto h-7 text-xs text-muted-foreground"
              onClick={() => setOpportunityStatus(opportunity.id, 'ignored')}
            >
              忽略此商机
            </Button>
          )}
        </div>
      </Card>

      {!opportunity.archivedAt && <SopStepper opportunity={opportunity} />}
    </div>
  )
}
