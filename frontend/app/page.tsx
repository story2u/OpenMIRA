'use client'

import { AlertTriangle, ChevronLeft, ChevronRight, Inbox } from 'lucide-react'
import Link from 'next/link'
import { useEffect, useMemo, useState } from 'react'
import { FilterPanel } from '@/components/filter-panel'
import { OpportunityCard } from '@/components/opportunity-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAppStore } from '@/lib/app-store'
import { applyFilters, defaultFilters, type DashboardFilters, type SortKey } from '@/lib/dashboard-filters'
import type { Platform } from '@/lib/types'

const PAGE_SIZE = 12

const sortLabels: Record<SortKey, string> = {
  newest: '最新优先',
  oldest: '最早优先',
  confidence: '按商机相关度',
  trust: '按可信度',
}

export default function DashboardPage() {
  const { opportunities, newOpportunityId } = useAppStore()
  const [filters, setFilters] = useState<DashboardFilters>(defaultFilters)
  const [page, setPage] = useState(1)

  const filtered = useMemo(() => applyFilters(opportunities, filters), [opportunities, filters])
  const keywordOptions = useMemo(
    () => Array.from(new Set(opportunities.flatMap((o) => o.matchedKeywords))).sort(),
    [opportunities],
  )

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const safePage = Math.min(page, totalPages)
  const pageItems = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)

  // 筛选变化时回到第一页
  useEffect(() => {
    setPage(1)
  }, [filters])

  const pendingCount = opportunities.filter((o) => o.status === 'pending').length
  const attentionOpportunities = opportunities.filter((o) => o.attentionRequired && o.status === 'pending')

  return (
    <div className="mx-auto w-full max-w-5xl px-4 py-6 md:px-8">
      <header className="mb-6 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight md:text-2xl">商机看板</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            自动识别 Telegram 与企业微信中的潜在商机
          </p>
        </div>
        <Badge variant="secondary" className="gap-1.5 rounded-full px-3 py-1">
          <span className="size-1.5 rounded-full bg-warning" aria-hidden="true" />
          {pendingCount} 条待处理
        </Badge>
      </header>

      {attentionOpportunities.length > 0 && (
        <section
          role="alert"
          aria-label="重大商机提醒"
          className="mb-5 rounded-xl border border-warning/40 bg-warning/10 p-4"
        >
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-warning" />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold">pi Agent 发现 {attentionOpportunities.length} 条重大商机</p>
              <p className="mt-0.5 text-xs text-muted-foreground">请优先核对链接结论和后续行动建议，外部动作仍需人工批准。</p>
              <div className="mt-3 flex flex-wrap gap-2">
                {attentionOpportunities.slice(0, 3).map((opportunity) => (
                  <Button
                    key={opportunity.id}
                    nativeButton={false}
                    render={<Link href={`/opportunity/${opportunity.id}`} />}
                    variant="outline"
                    size="sm"
                    className="bg-background/70"
                  >
                    {opportunity.contactName} · 查看建议
                  </Button>
                ))}
              </div>
            </div>
          </div>
        </section>
      )}

      <div className="mb-3 flex flex-wrap items-center gap-2.5">
        <Tabs value={filters.status} onValueChange={(v) => setFilters({ ...filters, status: v as DashboardFilters['status'] })}>
          <TabsList>
            <TabsTrigger value="all">全部</TabsTrigger>
            <TabsTrigger value="pending">待处理</TabsTrigger>
            <TabsTrigger value="replied">已回复</TabsTrigger>
            <TabsTrigger value="ignored">已忽略</TabsTrigger>
          </TabsList>
        </Tabs>
        <Select
          items={{ all: '全部平台', telegram: 'Telegram', wecom: '企业微信' }}
          value={filters.platform}
          onValueChange={(v) => setFilters({ ...filters, platform: v as 'all' | Platform })}
        >
          <SelectTrigger className="h-8 w-32 text-xs" aria-label="按平台筛选">
            <SelectValue placeholder="平台" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部平台</SelectItem>
            <SelectItem value="telegram">Telegram</SelectItem>
            <SelectItem value="wecom">企业微信</SelectItem>
          </SelectContent>
        </Select>
        <FilterPanel filters={filters} onChange={setFilters} keywordOptions={keywordOptions} />
        <Select
          items={sortLabels}
          value={filters.sort}
          onValueChange={(v) => setFilters({ ...filters, sort: v as SortKey })}
        >
          <SelectTrigger className="h-8 w-36 text-xs" aria-label="排序方式">
            <SelectValue placeholder="排序" />
          </SelectTrigger>
          <SelectContent>
            {(Object.keys(sortLabels) as SortKey[]).map((key) => (
              <SelectItem key={key} value={key}>
                {sortLabels[key]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <p className="mb-4 text-xs text-muted-foreground" aria-live="polite">
        当前筛选下共 <span className="font-semibold text-foreground">{filtered.length}</span> 条商机
        {totalPages > 1 && `，第 ${safePage} / ${totalPages} 页`}
      </p>

      {pageItems.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed py-16 text-center">
          <span className="flex size-14 items-center justify-center rounded-full bg-muted">
            <Inbox className="size-7 text-muted-foreground" />
          </span>
          <div>
            <p className="text-sm font-medium">暂无匹配的商机</p>
            <p className="mt-1 text-xs text-muted-foreground">
              调整筛选条件，或等待系统从聊天中识别新的商机
            </p>
          </div>
        </div>
      ) : (
        <>
          <div className="grid gap-3 md:grid-cols-2">
            {pageItems.map((opportunity) => (
              <OpportunityCard
                key={opportunity.id}
                opportunity={opportunity}
                isNew={opportunity.id === newOpportunityId}
              />
            ))}
          </div>

          {totalPages > 1 && (
            <nav className="mt-6 flex items-center justify-center gap-1.5" aria-label="分页">
              <Button
                variant="outline"
                size="icon-sm"
                className="bg-transparent"
                disabled={safePage <= 1}
                onClick={() => setPage(safePage - 1)}
                aria-label="上一页"
              >
                <ChevronLeft className="size-4" />
              </Button>
              {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
                <Button
                  key={p}
                  variant={p === safePage ? 'default' : 'ghost'}
                  size="icon-sm"
                  onClick={() => setPage(p)}
                  aria-label={`第 ${p} 页`}
                  aria-current={p === safePage ? 'page' : undefined}
                >
                  {p}
                </Button>
              ))}
              <Button
                variant="outline"
                size="icon-sm"
                className="bg-transparent"
                disabled={safePage >= totalPages}
                onClick={() => setPage(safePage + 1)}
                aria-label="下一页"
              >
                <ChevronRight className="size-4" />
              </Button>
            </nav>
          )}
        </>
      )}
    </div>
  )
}
