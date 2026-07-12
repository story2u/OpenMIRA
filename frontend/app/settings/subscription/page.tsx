'use client'

import { AlertTriangle, ArrowLeft, Bot, Check, ExternalLink, Loader2, MessagesSquare } from 'lucide-react'
import Link from 'next/link'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAuth } from '@/lib/auth'
import { annualSavings } from '@/lib/billing/packages'
import { loadWebBilling, purchaseWebPackage } from '@/lib/billing/revenuecat-web'
import type { BillingAvailability, BillingPackage } from '@/lib/billing/types'
import {
  fetchMySubscription,
  fetchSubscriptionCatalog,
  fetchSubscriptionManagement,
  syncMySubscription,
} from '@/lib/api'
import type { BillingInterval, BillingStore, PlanCode, SubscriptionCatalogPlan, SubscriptionManagement, SubscriptionUsage } from '@/lib/types'

const planNames: Record<PlanCode, string> = { free: 'Free', plus: 'Plus', pro: 'Pro', max: 'Max' }
const storeNames: Record<BillingStore, string> = {
  app_store: 'Apple App Store', play_store: 'Google Play', paddle: 'Web / Paddle', test_store: '测试商店', unknown: '未知渠道',
}

function UsageBar({ value, limit }: { value: number; limit: number }) {
  const percentage = limit > 0 ? Math.min((value / limit) * 100, 100) : 0
  return <div className="h-2 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuenow={value} aria-valuemin={0} aria-valuemax={limit}>
    <div className="h-full rounded-full bg-primary transition-[width]" style={{ width: `${percentage}%` }} />
  </div>
}

function StatusNotice({ usage }: { usage: SubscriptionUsage }) {
  if (usage.multipleActiveSubscriptions) return <Card className="mb-4 flex-row items-start gap-3 border-warning p-4 text-sm"><AlertTriangle className="mt-0.5 size-5 shrink-0 text-warning" /><p>检测到多个渠道的有效订阅，你可能正在重复付费。请前往原购买渠道管理。</p></Card>
  if (usage.billingIssue) return <Card className="mb-4 flex-row items-start gap-3 border-destructive p-4 text-sm"><AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" /><p>当前订阅存在付款问题。宽限期结束后权益可能变化，请前往原购买渠道处理。</p></Card>
  if (usage.cancelAtPeriodEnd) return <Card className="mb-4 p-4 text-sm">续费已取消，当前权益将在 {usage.entitlementExpiresAt ? new Date(usage.entitlementExpiresAt).toLocaleDateString('zh-CN') : '当前周期结束时'} 到期。</Card>
  return null
}

export default function SubscriptionPage() {
  const { user } = useAuth()
  const [usage, setUsage] = useState<SubscriptionUsage | null>(null)
  const [catalog, setCatalog] = useState<SubscriptionCatalogPlan[]>([])
  const [management, setManagement] = useState<SubscriptionManagement | null>(null)
  const [billing, setBilling] = useState<BillingAvailability | null>(null)
  const [interval, setInterval] = useState<Exclude<BillingInterval, 'unknown'>>('monthly')
  const [busyPackage, setBusyPackage] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refreshBackend = useCallback(async () => {
    const [nextUsage, nextCatalog, nextManagement] = await Promise.all([
      fetchMySubscription(), fetchSubscriptionCatalog(), fetchSubscriptionManagement(),
    ])
    setUsage(nextUsage); setCatalog(nextCatalog); setManagement(nextManagement)
  }, [])

  useEffect(() => {
    let cancelled = false
    refreshBackend().catch((reason: unknown) => { if (!cancelled) setError(reason instanceof Error ? reason.message : '无法加载订阅信息') })
    return () => { cancelled = true }
  }, [refreshBackend])

  useEffect(() => {
    if (!user) return
    let cancelled = false
    loadWebBilling(user.id)
      .then((next) => { if (!cancelled) setBilling(next) })
      .catch((reason: unknown) => { if (!cancelled) setError(reason instanceof Error ? reason.message : '无法加载支付套餐') })
    return () => { cancelled = true }
  }, [user])

  const packagesById = useMemo(() => new Map(
    billing?.status === 'ready' ? billing.packages.map((item) => [item.identifier, item]) : [],
  ), [billing])

  async function purchase(item: BillingPackage) {
    if (!user || !usage) return
    setBusyPackage(item.identifier); setError(null); setMessage(null)
    try {
      const result = await purchaseWebPackage(user.id, item)
      if (result === 'cancelled') { setMessage('已取消购买，未产生套餐变更。'); return }
      setMessage('支付已完成，正在确认订阅权益。')
      const synced = await syncMySubscription()
      setUsage(synced)
      setManagement(await fetchSubscriptionManagement())
      setMessage(synced.planCode === item.planCode ? '订阅权益已生效。' : '支付已完成，正在确认订阅权益。')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '购买失败，请稍后重试')
    } finally { setBusyPackage(null) }
  }

  if (error && !usage) return <div className="mx-auto w-full max-w-5xl px-4 py-6 md:px-8"><Link href="/settings" className="mb-5 inline-flex items-center gap-1 text-sm text-muted-foreground"><ArrowLeft className="size-4" /> 返回设置</Link><Card className="p-5 text-sm text-destructive">{error}</Card></div>
  if (!usage) return <div className="flex min-h-64 items-center justify-center" aria-live="polite"><Loader2 className="size-5 animate-spin text-muted-foreground" /><span className="sr-only">正在加载订阅信息</span></div>

  const allocated = usage.aiAnalysesConsumed + usage.aiAnalysesReserved
  const paid = usage.planCode !== 'free'
  const periodEnd = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium' }).format(new Date(usage.usagePeriodEnd))

  return <div className="mx-auto w-full max-w-5xl px-4 py-6 md:px-8">
    <Link href="/settings" className="mb-5 inline-flex items-center gap-1 text-sm text-muted-foreground"><ArrowLeft className="size-4" /> 返回设置</Link>
    <header className="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div><div className="flex items-center gap-2"><h1 className="text-xl font-semibold md:text-2xl">套餐与用量</h1><Badge variant="secondary">{planNames[usage.planCode]}</Badge></div><p className="mt-1 text-sm text-muted-foreground">本月额度截止到 {periodEnd}{usage.effectiveStore ? ` · ${storeNames[usage.effectiveStore]}` : ''}</p></div>
      {paid && <Button variant="outline" disabled={!management?.canOpenInCurrentClient || !management.managementUrl} onClick={() => management?.managementUrl && window.open(management.managementUrl, '_blank', 'noopener,noreferrer')}>
        {management?.canOpenInCurrentClient && management.managementUrl ? <>管理订阅 <ExternalLink className="size-4" /></> : '请在原购买渠道管理'}
      </Button>}
    </header>
    <StatusNotice usage={usage} />
    {message && <Card className="mb-4 p-4 text-sm" aria-live="polite">{message}</Card>}
    {error && <Card className="mb-4 p-4 text-sm text-destructive" aria-live="assertive">{error}</Card>}

    <section className="mb-8 grid gap-4 sm:grid-cols-2" aria-label="当前用量">
      <Card className="gap-4 p-5"><div className="flex items-center gap-2"><Bot className="size-5 text-primary" /><h2 className="font-medium">AI 分析</h2></div><div><p className="mb-2 text-2xl font-semibold">{allocated.toLocaleString()} <span className="text-sm font-normal text-muted-foreground">/ {usage.entitlements.piAgentAnalysisMonthlyLimit.toLocaleString()} 次</span></p><UsageBar value={allocated} limit={usage.entitlements.piAgentAnalysisMonthlyLimit} /></div><p className="text-xs text-muted-foreground">已完成 {usage.aiAnalysesConsumed.toLocaleString()} 次，处理中 {usage.aiAnalysesReserved.toLocaleString()} 次</p></Card>
      <Card className="gap-4 p-5"><div className="flex items-center gap-2"><MessagesSquare className="size-5 text-primary" /><h2 className="font-medium">群监控</h2></div><div><p className="mb-2 text-2xl font-semibold">{usage.combinedGroupsUsed} <span className="text-sm font-normal text-muted-foreground">/ {usage.entitlements.combinedGroupLimit} 个</span></p><UsageBar value={usage.combinedGroupsUsed} limit={usage.entitlements.combinedGroupLimit} /></div><p className="text-xs text-muted-foreground">Telegram {usage.telegramGroupsUsed} 个 · 企微 {usage.wecomGroupsUsed} 个</p></Card>
    </section>

    <section aria-labelledby="plans-heading">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3"><h2 id="plans-heading" className="text-base font-semibold">套餐对比</h2><Tabs value={interval} onValueChange={(value) => setInterval(value as 'monthly' | 'annual')}><TabsList><TabsTrigger value="monthly">月付</TabsTrigger><TabsTrigger value="annual">年付</TabsTrigger></TabsList></Tabs></div>
      {billing?.status === 'unconfigured' && <Card className="mb-4 p-4 text-sm text-muted-foreground">支付尚未配置。当前套餐和用量不受影响。</Card>}
      {billing?.status === 'missing_offering' && <Card className="mb-4 p-4 text-sm text-muted-foreground">当前没有可购买的套餐，请稍后再试。</Card>}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {catalog.map((plan) => {
          const current = plan.planCode === usage.planCode
          const identifier = `${plan.planCode}_${interval}`
          const item = packagesById.get(identifier)
          const monthly = packagesById.get(`${plan.planCode}_monthly`)
          const annual = packagesById.get(`${plan.planCode}_annual`)
          const savings = annualSavings(monthly, annual)
          const isFree = plan.planCode === 'free'
          const blocked = paid && !current
          return <Card key={plan.planCode} className="gap-4 p-4">
            <div className="flex items-center justify-between"><h3 className="font-semibold">{plan.displayName}</h3>{current && <Badge>当前</Badge>}</div>
            <div className="min-h-12"><p className="text-xl font-semibold">{isFree ? '免费' : item?.formattedPrice ?? '价格暂不可用'}</p>{!isFree && interval === 'annual' && savings !== null && savings > 0 && <p className="text-xs text-success">相比月付节省 {savings}%</p>}</div>
            <ul className="space-y-2 text-sm text-muted-foreground"><li className="flex gap-2"><Check className="mt-0.5 size-4 shrink-0 text-success" />每月 {plan.entitlements.piAgentAnalysisMonthlyLimit.toLocaleString()} 次 AI 分析</li><li className="flex gap-2"><Check className="mt-0.5 size-4 shrink-0 text-success" />{isFree ? '1 个 TG 群 + 1 个企微群' : `合计 ${plan.entitlements.combinedGroupLimit} 个群`}</li></ul>
            <Button variant={current ? 'secondary' : 'default'} disabled={current || isFree || !item || blocked || busyPackage !== null} className="mt-auto w-full" onClick={() => item && purchase(item)}>{busyPackage === identifier ? <><Loader2 className="size-4 animate-spin" />处理中</> : current ? '当前套餐' : blocked ? '请先管理现有订阅' : item ? '购买' : '暂不可购买'}</Button>
          </Card>
        })}
      </div>
    </section>
  </div>
}
