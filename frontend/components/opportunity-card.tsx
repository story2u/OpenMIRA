'use client'

import { AlertTriangle, Archive, ArchiveRestore, Loader2, MessageCircle, Users } from 'lucide-react'
import Link from 'next/link'
import { ConfidenceRing } from '@/components/confidence-ring'
import { PlatformIcon } from '@/components/platform-icon'
import { TrustBadge } from '@/components/trust-badge'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { MOCK_NOW } from '@/lib/dashboard-filters'
import { sopStageConfig } from '@/lib/sop'
import type { Opportunity, Priority } from '@/lib/types'
import { cn } from '@/lib/utils'

const priorityConfig: Record<Priority, { label: string; className: string }> = {
  urgent: { label: '紧急', className: 'bg-destructive/10 text-destructive border-destructive/30' },
  high: { label: '高', className: 'bg-warning/10 text-warning border-warning/30' },
  normal: { label: '中', className: 'bg-primary/10 text-primary border-primary/30' },
  low: { label: '低', className: 'bg-muted text-muted-foreground border-border' },
}

const statusConfig = {
  pending: { label: '待处理', className: 'text-warning' },
  replied: { label: '已回复', className: 'text-success' },
  ignored: { label: '已忽略', className: 'text-muted-foreground' },
}

function formatTime(iso: string) {
  const date = new Date(iso)
  const diffMs = MOCK_NOW.getTime() - date.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return '刚刚'
  if (diffMin < 60) return `${diffMin} 分钟前`
  const diffHour = Math.floor(diffMin / 60)
  if (diffHour < 24) return `${diffHour} 小时前`
  return `${Math.floor(diffHour / 24)} 天前`
}

export function OpportunityCard({
  opportunity,
  isNew,
  selected = false,
  actionPending = false,
  onSelectedChange,
  onArchive,
  onRestore,
}: {
  opportunity: Opportunity
  isNew?: boolean
  selected?: boolean
  actionPending?: boolean
  onSelectedChange?: (selected: boolean) => void
  onArchive?: () => void
  onRestore?: () => void
}) {
  const priority = priorityConfig[opportunity.priority]
  const status = statusConfig[opportunity.status]
  const stage = sopStageConfig[opportunity.sopStage]
  const hasUnverifiedLink =
    opportunity.rawMessageLinks.length > 0 &&
    (opportunity.linkVerification.status === 'unverified' || opportunity.linkVerification.status === 'verifying')

  return (
    <Card
      className={cn(
        'gap-0 overflow-hidden rounded-xl border p-0 shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md',
        isNew && 'animate-card-enter ring-2 ring-primary/40',
      )}
    >
      <Link href={`/opportunity/${opportunity.id}`} className="flex flex-col gap-3 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <div className="relative shrink-0">
              <Avatar className="size-10">
                <AvatarImage src={opportunity.contactAvatar || '/placeholder.svg'} alt="" />
                <AvatarFallback>{opportunity.contactName.slice(0, 1)}</AvatarFallback>
              </Avatar>
              <PlatformIcon
                platform={opportunity.platform}
                className="absolute -bottom-1 -right-1 size-5 border-2 border-card"
              />
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-1.5">
                <p className="truncate text-sm font-semibold">{opportunity.contactName}</p>
                <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px]', priority.className)}>
                  {priority.label}
                </Badge>
                <TrustBadge score={opportunity.trustScore} showScore={false} />
                {opportunity.archivedAt && <Badge variant="outline" className="h-5 px-1.5 text-[10px]">已归档</Badge>}
              </div>
              <p className="mt-0.5 flex items-center gap-1 text-[11px] text-muted-foreground">
                {opportunity.sourceType === 'group' ? (
                  <>
                    <Users className="size-3 shrink-0" />
                    <span className="truncate">{opportunity.groupName}</span>
                  </>
                ) : (
                  <>
                    <MessageCircle className="size-3 shrink-0" />
                    私聊
                  </>
                )}
                <span aria-hidden="true">·</span>
                {formatTime(opportunity.createdAt)}
                <span aria-hidden="true">·</span>
                <span className={status.className}>{status.label}</span>
              </p>
            </div>
          </div>
          <div className="flex shrink-0 flex-col items-center gap-0.5">
            <ConfidenceRing score={opportunity.confidenceScore} />
            <span className="text-[9px] text-muted-foreground">相关度</span>
          </div>
        </div>

        <p className="text-pretty text-sm leading-relaxed text-foreground">{opportunity.summary}</p>

        {hasUnverifiedLink && (
          <p className="flex items-center gap-1.5 rounded-md border border-warning/40 bg-warning/10 px-2 py-1.5 text-[11px] font-medium text-warning">
            <AlertTriangle className="size-3.5 shrink-0" />
            含未核验链接，请先完成安全分析
          </p>
        )}

        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="secondary" className="h-5 gap-1 rounded-md px-1.5 text-[10px] font-normal">
            <span className={cn('size-1.5 rounded-full', stage.dotClass)} aria-hidden="true" />
            {stage.label}
          </Badge>
          {opportunity.matchedKeywords.map((keyword) => (
            <Badge key={keyword} variant="secondary" className="h-5 rounded-md px-1.5 text-[10px] font-normal">
              {keyword}
            </Badge>
          ))}
        </div>
      </Link>
      <div className="flex min-h-11 items-center justify-between gap-3 border-t px-4 py-2">
        {onSelectedChange ? (
          <label className="flex cursor-pointer items-center gap-2 text-xs text-muted-foreground">
            <Checkbox checked={selected} onCheckedChange={(checked) => onSelectedChange(checked === true)} />
            选择
          </label>
        ) : <span />}
        {opportunity.archivedAt ? (
          <Button variant="ghost" size="sm" className="h-7 gap-1.5 text-xs" disabled={actionPending} onClick={onRestore}>
            {actionPending ? <Loader2 className="size-3.5 animate-spin" /> : <ArchiveRestore className="size-3.5" />}
            恢复
          </Button>
        ) : (
          <Button variant="ghost" size="sm" className="h-7 gap-1.5 text-xs text-muted-foreground" disabled={actionPending} onClick={onArchive}>
            {actionPending ? <Loader2 className="size-3.5 animate-spin" /> : <Archive className="size-3.5" />}
            归档
          </Button>
        )}
      </div>
    </Card>
  )
}
