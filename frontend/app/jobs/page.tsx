'use client'

import { BriefcaseBusiness, ChevronLeft, ChevronRight, Filter, Loader2, Search, SlidersHorizontal } from 'lucide-react'
import Link from 'next/link'
import { useCallback, useEffect, useState } from 'react'
import { JobCard } from '@/components/job-card'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { fetchJobs, fetchJobSearchProfiles, type JobFilters } from '@/lib/api'
import type { JobSearchProfile, JobsPage } from '@/lib/types'

const PAGE_SIZE = 20

export default function JobsPage() {
  const [profiles, setProfiles] = useState<JobSearchProfile[]>([])
  const [data, setData] = useState<JobsPage | null>(null)
  const [filters, setFilters] = useState<JobFilters>({ sort: 'match', excludeExpired: true, limit: PAGE_SIZE, offset: 0 })
  const [draftQuery, setDraftQuery] = useState('')
  const [showFilters, setShowFilters] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true); setError(null)
    try { setData(await fetchJobs(filters)) }
    catch (cause) { setError(cause instanceof Error ? cause.message : '加载工作机会失败') }
    finally { setLoading(false) }
  }, [filters])

  useEffect(() => { fetchJobSearchProfiles().then(setProfiles).catch(() => setProfiles([])) }, [])
  useEffect(() => { void load() }, [load])

  function setFilter<K extends keyof JobFilters>(key: K, value: JobFilters[K]) {
    setFilters((current) => ({ ...current, [key]: value, offset: 0 }))
  }

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-6 md:px-8" data-testid="jobs-page">
      <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div><h1 className="text-xl font-semibold md:text-2xl">工作机会</h1><p className="mt-1 text-sm text-muted-foreground">从已授权群聊和私聊中筛选真实招聘信息</p></div>
        <Button variant="outline" nativeButton={false} render={<Link href="/settings/job-search" />} className="gap-2"><SlidersHorizontal className="size-4" />管理求职档案</Button>
      </header>

      <div className="mb-4 grid gap-2 md:grid-cols-[220px_1fr_auto_auto]">
        <Select value={filters.profileId || data?.profile?.id || ''} onValueChange={(value) => setFilter('profileId', value || undefined)}>
          <SelectTrigger aria-label="当前求职档案"><SelectValue placeholder="选择求职档案" /></SelectTrigger>
          <SelectContent>{profiles.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.name}{profile.isDefault ? '（默认）' : ''}</SelectItem>)}</SelectContent>
        </Select>
        <form className="flex gap-2" onSubmit={(event) => { event.preventDefault(); setFilter('query', draftQuery.trim() || undefined) }}><Input value={draftQuery} onChange={(event) => setDraftQuery(event.target.value)} placeholder="岗位、公司或技能" aria-label="搜索工作机会" /><Button type="submit" size="icon" aria-label="搜索"><Search className="size-4" /></Button></form>
        <Select value={filters.sort || 'match'} onValueChange={(value) => setFilter('sort', value as JobFilters['sort'])}><SelectTrigger className="min-w-32" aria-label="排序"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="match">匹配度</SelectItem><SelectItem value="newest">最新发布</SelectItem><SelectItem value="salary">薪资</SelectItem><SelectItem value="confidence">提取可信度</SelectItem><SelectItem value="source_reliability">来源可信度</SelectItem></SelectContent></Select>
        <Button variant={showFilters ? 'secondary' : 'outline'} onClick={() => setShowFilters((value) => !value)} className="gap-2"><Filter className="size-4" />筛选</Button>
      </div>

      {showFilters && <Card className="mb-5 grid gap-4 rounded-lg p-4 sm:grid-cols-2 lg:grid-cols-4" data-testid="job-filters">
        <div className="space-y-1.5"><Label htmlFor="job-source">来源</Label><Select value={filters.source || 'all'} onValueChange={(value) => setFilter('source', value === 'all' ? undefined : value as 'telegram' | 'wecom')}><SelectTrigger id="job-source"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部渠道</SelectItem><SelectItem value="telegram">Telegram</SelectItem><SelectItem value="wecom">企业微信</SelectItem></SelectContent></Select></div>
        <div className="space-y-1.5"><Label htmlFor="job-mode">工作模式</Label><Select value={filters.workMode || 'all'} onValueChange={(value) => { const next = String(value); setFilter('workMode', next === 'all' ? undefined : next) }}><SelectTrigger id="job-mode"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部</SelectItem><SelectItem value="remote">远程</SelectItem><SelectItem value="hybrid">混合</SelectItem><SelectItem value="on_site">现场</SelectItem><SelectItem value="flexible">灵活</SelectItem></SelectContent></Select></div>
        <div className="space-y-1.5"><Label htmlFor="job-city">城市</Label><Input id="job-city" value={filters.city || ''} onChange={(event) => setFilter('city', event.target.value || undefined)} placeholder="例如 Berlin" /></div>
        <div className="space-y-1.5"><Label htmlFor="job-score">最低匹配分</Label><Input id="job-score" type="number" min={0} max={100} value={filters.minimumMatchScore ?? ''} onChange={(event) => setFilter('minimumMatchScore', event.target.value ? Number(event.target.value) : undefined)} /></div>
        <div className="space-y-1.5"><Label htmlFor="job-salary">最低薪资</Label><Input id="job-salary" type="number" min={0} value={filters.salaryMin ?? ''} onChange={(event) => setFilter('salaryMin', event.target.value ? Number(event.target.value) : undefined)} /></div>
        <div className="space-y-1.5"><Label htmlFor="job-currency">币种</Label><Input id="job-currency" maxLength={3} value={filters.salaryCurrency || ''} onChange={(event) => setFilter('salaryCurrency', event.target.value.toUpperCase() || undefined)} placeholder="USD" /></div>
        <div className="space-y-1.5"><Label htmlFor="job-visa">签证支持</Label><Select value={filters.visaSponsorship === undefined ? 'all' : String(filters.visaSponsorship)} onValueChange={(value) => setFilter('visaSponsorship', value === 'all' ? undefined : value === 'true')}><SelectTrigger id="job-visa"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">不限/未说明</SelectItem><SelectItem value="true">明确支持</SelectItem><SelectItem value="false">明确不支持</SelectItem></SelectContent></Select></div>
        <div className="space-y-1.5"><Label htmlFor="job-age-limit">年龄限制</Label><Select value={filters.ageRequirementPresent === false ? 'without' : 'all'} onValueChange={(value) => setFilter('ageRequirementPresent', value === 'without' ? false : undefined)}><SelectTrigger id="job-age-limit"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部</SelectItem><SelectItem value="without">仅无明确年龄限制</SelectItem></SelectContent></Select></div>
      </Card>}

      {error && <p role="alert" className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</p>}
      <div className="mb-3 flex items-center justify-between text-sm text-muted-foreground"><span>共 {data?.total ?? 0} 个匹配职位</span>{data?.profile && <span>当前档案：{data.profile.name}</span>}</div>
      {loading ? <div className="grid min-h-64 place-items-center text-sm text-muted-foreground"><Loader2 className="mb-2 size-5 animate-spin" />正在筛选职位</div> : data?.items.length ? <div className="grid gap-3 lg:grid-cols-2">{data.items.map((job) => <JobCard key={job.opportunityId} job={job} />)}</div> : <div className="grid min-h-64 place-items-center rounded-lg border border-dashed text-center"><div><BriefcaseBusiness className="mx-auto size-9 text-muted-foreground" /><p className="mt-3 text-sm font-medium">暂无匹配的工作机会</p><p className="mt-1 text-xs text-muted-foreground">调整筛选或配置求职档案；普通聊天不会进入职位列表。</p></div></div>}
      {(data?.total || 0) > PAGE_SIZE && <div className="mt-5 flex justify-end gap-2"><Button variant="outline" size="sm" disabled={(filters.offset || 0) === 0} onClick={() => setFilters((current) => ({ ...current, offset: Math.max(0, (current.offset || 0) - PAGE_SIZE) }))}><ChevronLeft className="size-4" />上一页</Button><Button variant="outline" size="sm" disabled={(filters.offset || 0) + PAGE_SIZE >= (data?.total || 0)} onClick={() => setFilters((current) => ({ ...current, offset: (current.offset || 0) + PAGE_SIZE }))}>下一页<ChevronRight className="size-4" /></Button></div>}
    </div>
  )
}
