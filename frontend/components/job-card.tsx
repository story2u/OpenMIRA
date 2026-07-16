import { AlertTriangle, Building2, Clock3, MapPin, Radio, WalletCards } from 'lucide-react'
import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import type { JobOpportunity } from '@/lib/types'

const workModeLabels = { remote: '远程', hybrid: '混合', on_site: '现场', flexible: '灵活', unknown: '方式未说明' }
const employmentLabels = { full_time: '全职', part_time: '兼职', contract: '合同', internship: '实习', freelance: '自由职业', temporary: '临时', unknown: '类型未说明' }

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric' }).format(new Date(value))
}

export function JobCard({ job }: { job: JobOpportunity }) {
  const score = job.match?.matchScore
  const tags = [workModeLabels[job.workMode], ...job.requiredSkills].slice(0, 5)
  return (
    <Card className="gap-4 rounded-lg p-4 shadow-sm" data-testid={`job-card-${job.opportunityId}`}>
      <div className="flex items-start gap-3">
        <div className="grid size-10 shrink-0 place-items-center rounded-md border bg-muted/50"><Building2 className="size-5 text-muted-foreground" /></div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0"><h2 className="font-semibold leading-5">{job.jobTitle}</h2><p className="mt-1 truncate text-sm text-muted-foreground">{job.companyName || '公司未说明'}</p></div>
            {score !== undefined && <div className="shrink-0 text-right"><strong className="text-xl text-primary">{score}</strong><span className="text-xs text-muted-foreground"> / 100</span><p className="text-[10px] text-muted-foreground">档案匹配</p></div>}
          </div>
          <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1"><MapPin className="size-3.5" />{job.locationText || '地点未说明'}</span>
            <span className="inline-flex items-center gap-1"><WalletCards className="size-3.5" />{job.salaryRaw || '薪资未公开'}</span>
            <span className="inline-flex items-center gap-1"><Clock3 className="size-3.5" />{formatDate(job.postedAt)}</span>
          </div>
        </div>
      </div>
      <div className="flex flex-wrap gap-1.5">{tags.map((tag) => <Badge key={tag} variant="secondary" className="rounded-md font-normal">{tag}</Badge>)}<Badge variant="outline" className="rounded-md font-normal">{employmentLabels[job.employmentType]}</Badge></div>
      {(job.complianceFlags.length > 0 || job.conflictingSourceData) && <div className="flex items-center gap-1.5 text-xs text-warning"><AlertTriangle className="size-3.5" />{job.complianceFlags.length > 0 ? '招聘原文包含合规风险提示' : '多个来源的信息存在差异'}</div>}
      <div className="flex items-center justify-between gap-3 border-t pt-3 text-xs text-muted-foreground">
        <span className="inline-flex min-w-0 items-center gap-1"><Radio className="size-3.5 shrink-0" /><span className="truncate">{job.sourceChannel === 'telegram' ? 'Telegram' : '企业微信'} · {job.sourceChatName || '私聊来源'}{job.sourceCount > 1 ? ` · ${job.sourceCount} 个来源` : ''}</span></span>
        <Button size="sm" variant="outline" nativeButton={false} render={<Link href={`/jobs/${job.opportunityId}`} />}>查看详情</Button>
      </div>
    </Card>
  )
}
