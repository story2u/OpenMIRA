'use client'

import { ArrowLeft, Bot, Check, Loader2, MessagesSquare } from 'lucide-react'
import Link from 'next/link'
import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { fetchMySubscription, fetchSubscriptionPlans } from '@/lib/api'
import type { PlanCode, PlanEntitlements, SubscriptionUsage } from '@/lib/types'

const planNames: Record<PlanCode, string> = {
  free: 'Free',
  plus: 'Plus',
  pro: 'Pro',
  max: 'Max',
}

function UsageBar({ value, limit }: { value: number; limit: number }) {
  const percentage = limit > 0 ? Math.min((value / limit) * 100, 100) : 0
  return (
    <div
      className="h-2 overflow-hidden rounded-full bg-muted"
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={limit}
    >
      <div className="h-full rounded-full bg-primary transition-[width]" style={{ width: `${percentage}%` }} />
    </div>
  )
}

export default function SubscriptionPage() {
  const [usage, setUsage] = useState<SubscriptionUsage | null>(null)
  const [plans, setPlans] = useState<PlanEntitlements[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([fetchMySubscription(), fetchSubscriptionPlans()])
      .then(([nextUsage, nextPlans]) => {
        if (cancelled) return
        setUsage(nextUsage)
        setPlans(nextPlans)
      })
      .catch((reason: unknown) => {
        if (!cancelled) setError(reason instanceof Error ? reason.message : '无法加载订阅信息')
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (error) {
    return (
      <div className="mx-auto w-full max-w-4xl px-4 py-6 md:px-8">
        <Link href="/settings" className="mb-5 inline-flex items-center gap-1 text-sm text-muted-foreground">
          <ArrowLeft className="size-4" /> 返回设置
        </Link>
        <Card className="rounded-xl p-5 text-sm text-destructive">{error}</Card>
      </div>
    )
  }

  if (!usage) {
    return (
      <div className="flex min-h-64 items-center justify-center" aria-live="polite">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
        <span className="sr-only">正在加载订阅信息</span>
      </div>
    )
  }

  const allocated = usage.aiAnalysesConsumed + usage.aiAnalysesReserved
  const periodEnd = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium' }).format(
    new Date(usage.periodEnd),
  )

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 md:px-8">
      <Link href="/settings" className="mb-5 inline-flex items-center gap-1 text-sm text-muted-foreground">
        <ArrowLeft className="size-4" /> 返回设置
      </Link>
      <header className="mb-6">
        <div className="flex items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight md:text-2xl">套餐与用量</h1>
          <Badge variant="secondary">{planNames[usage.planCode]}</Badge>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">本周期额度截止到 {periodEnd}</p>
      </header>

      <section className="mb-8 grid gap-4 sm:grid-cols-2" aria-label="当前用量">
        <Card className="gap-4 rounded-xl p-5 shadow-sm">
          <div className="flex items-center gap-2">
            <Bot className="size-5 text-primary" />
            <h2 className="font-medium">AI 分析</h2>
          </div>
          <div>
            <p className="mb-2 text-2xl font-semibold">
              {allocated.toLocaleString()}
              <span className="text-sm font-normal text-muted-foreground">
                {' '}/ {usage.entitlements.piAgentAnalysisMonthlyLimit.toLocaleString()} 次
              </span>
            </p>
            <UsageBar value={allocated} limit={usage.entitlements.piAgentAnalysisMonthlyLimit} />
          </div>
          <p className="text-xs text-muted-foreground">
            已完成 {usage.aiAnalysesConsumed.toLocaleString()} 次，处理中 {usage.aiAnalysesReserved.toLocaleString()} 次
          </p>
        </Card>

        <Card className="gap-4 rounded-xl p-5 shadow-sm">
          <div className="flex items-center gap-2">
            <MessagesSquare className="size-5 text-primary" />
            <h2 className="font-medium">群监控</h2>
          </div>
          <div>
            <p className="mb-2 text-2xl font-semibold">
              {usage.combinedGroupsUsed}
              <span className="text-sm font-normal text-muted-foreground">
                {' '}/ {usage.entitlements.combinedGroupLimit} 个
              </span>
            </p>
            <UsageBar value={usage.combinedGroupsUsed} limit={usage.entitlements.combinedGroupLimit} />
          </div>
          <p className="text-xs text-muted-foreground">
            Telegram {usage.telegramGroupsUsed} 个 · 企微 {usage.wecomGroupsUsed} 个
          </p>
        </Card>
      </section>

      <section aria-labelledby="plans-heading">
        <h2 id="plans-heading" className="mb-3 text-base font-semibold">套餐额度</h2>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {plans.map((plan) => {
            const current = plan.planCode === usage.planCode
            return (
              <Card key={plan.planCode} className="gap-4 rounded-xl p-4 shadow-sm">
                <div className="flex items-center justify-between">
                  <h3 className="font-semibold">{planNames[plan.planCode]}</h3>
                  {current && <Badge>当前</Badge>}
                </div>
                <ul className="space-y-2 text-sm text-muted-foreground">
                  <li className="flex gap-2">
                    <Check className="mt-0.5 size-4 shrink-0 text-success" />
                    每月 {plan.piAgentAnalysisMonthlyLimit.toLocaleString()} 次 AI 分析
                  </li>
                  <li className="flex gap-2">
                    <Check className="mt-0.5 size-4 shrink-0 text-success" />
                    {plan.planCode === 'free'
                      ? '1 个 TG 群 + 1 个企微群'
                      : `合计 ${plan.combinedGroupLimit} 个群`}
                  </li>
                </ul>
                <Button variant={current ? 'secondary' : 'outline'} disabled className="mt-auto w-full">
                  {current ? '当前套餐' : '升级即将开放'}
                </Button>
              </Card>
            )
          })}
        </div>
      </section>
    </div>
  )
}
