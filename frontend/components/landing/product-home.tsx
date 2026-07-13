'use client'

import {
  ArrowRight, Bot, CheckCircle2, Clock3, GitBranch as Github, Globe2, Link2, LockKeyhole,
  MessageSquare, MonitorSmartphone, Radar, SearchCheck, Send, ShieldCheck,
  Smartphone, Sparkles, UserCheck,
} from 'lucide-react'
import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

const capabilities = [
  [MessageSquare, '多渠道消息接入', '连接 Telegram 群聊、私聊与企业微信消息入口。'],
  [Radar, '商机自动识别', '规则与语义模型结合，识别明确需求和上下文信号。'],
  [Bot, '上下文语义分析', 'Pi Agent 整理会话、可信度、联系人和行动建议。'],
  [SearchCheck, '链接风险核验', '受限读取公开链接，记录跳转与风险证据。'],
  [UserCheck, '联系方式提取', '从消息与公开页面提取可审核的联系线索。'],
  [Sparkles, '回复草稿', '生成可编辑草稿，人工确认后才执行外部动作。'],
  [CheckCircle2, 'SOP 行动建议', '按发现、核验、联系和跟进阶段组织工作。'],
  [Clock3, '工作时间模式', '白天人工审核，非工作时间按安全策略辅助处理。'],
  [MonitorSmartphone, '多端及时处理', 'Web 完整运营，iOS 与 Android 用于移动审核。'],
] as const

const steps = [
  [Send, '01', '连接消息渠道', '以 Bot、Webhook 或普通账号只读授权接入消息来源。'],
  [Radar, '02', '识别潜在商机', '规则快速判断，语义模型补足藏在上下文里的真实需求。'],
  [Bot, '03', 'Pi Agent 分析', '核验链接、提取联系人，并形成结构化跟进建议。'],
  [UserCheck, '04', '人工确认跟进', '运营人员审核草稿和动作，再决定是否对外发送。'],
] as const

function Brand() {
  return (
    <span className="flex items-center gap-2.5">
      <span className="grid size-9 place-items-center rounded-md bg-primary text-primary-foreground"><Radar className="size-5" /></span>
      <span className="leading-tight"><strong className="block text-sm">商机雷达</strong><span className="block text-[10px] text-muted-foreground">Opportunity Radar</span></span>
    </span>
  )
}

function PreviewCard({ title, copy, detail }: { title: string; copy: string; detail: string }) {
  return (
    <div className="rounded-md border bg-card p-3 shadow-sm">
      <div className="flex items-start gap-2.5">
        <span className="grid size-8 shrink-0 place-items-center rounded-md bg-primary/10 text-primary"><Radar className="size-4" /></span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5"><strong className="truncate text-xs">{title}</strong><Badge variant="outline" className="h-4 px-1 text-[9px]">已启用</Badge></div>
          <p className="mt-1 line-clamp-2 text-[11px] leading-4 text-muted-foreground">{copy}</p>
        </div>
        <span className="text-sm font-semibold text-primary">{detail}</span>
      </div>
      <div className="mt-2 text-[9px] text-muted-foreground">商机雷达工作台</div>
    </div>
  )
}

export function ProductHome() {
  return (
    <div className="min-h-svh bg-background text-foreground" data-testid="product-home">
      <header className="sticky top-0 z-40 border-b bg-background/92 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 lg:px-8">
          <Link href="/"><Brand /></Link>
          <nav className="hidden items-center gap-6 text-sm text-muted-foreground md:flex" aria-label="产品导航">
            <a href="#capabilities">产品能力</a><a href="#workflow">工作流程</a><a href="#apps">多端应用</a>
            <a href="https://github.com/story2u/IM" target="_blank" rel="noreferrer">GitHub</a>
          </nav>
          <div className="flex items-center gap-2"><Button nativeButton={false} render={<Link href="/login" />} variant="ghost" size="sm">登录</Button><Button nativeButton={false} render={<Link href="/login" />} size="sm" data-testid="start-experience">开始体验</Button></div>
        </div>
      </header>

      <main>
        <section className="border-b" data-testid="hero">
          <div className="mx-auto grid min-h-[calc(100svh-4rem)] max-w-7xl content-center gap-10 px-4 py-14 lg:grid-cols-[0.88fr_1.12fr] lg:px-8 lg:py-16">
            <div className="flex flex-col justify-center">
              <Badge variant="outline" className="mb-5 w-fit gap-2 px-3 py-1"><span className="size-1.5 rounded-full bg-success" />多 IM 商机识别与 AI 辅助跟进</Badge>
              <h1 className="max-w-2xl text-4xl font-semibold leading-[1.12] sm:text-5xl lg:text-6xl">不错过每一条<br className="hidden sm:block" />藏在聊天里的商机</h1>
              <p className="mt-6 max-w-xl text-base leading-7 text-muted-foreground sm:text-lg">连接 Telegram 与企业微信，自动识别潜在需求，由 Pi Agent 完成上下文分析、风险核验和行动建议，让团队更快完成审核、回复与跟进。</p>
              <div className="mt-8 flex flex-wrap gap-3"><Button size="lg" nativeButton={false} render={<Link href="/login" />} className="gap-2">开始体验<ArrowRight className="size-4" /></Button><Button size="lg" variant="outline" nativeButton={false} render={<a href="#capabilities" />} className="gap-2 bg-transparent">查看产品能力</Button></div>
              <div className="mt-7 flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground"><span className="flex items-center gap-1.5"><ShieldCheck className="size-3.5 text-success" />外部动作需人工审批</span><span className="flex items-center gap-1.5"><Send className="size-3.5 text-sky-500" />Telegram 原生授权</span><span className="flex items-center gap-1.5"><Smartphone className="size-3.5 text-primary" />Web / iOS / Android</span></div>
            </div>

            <div className="relative flex items-center" data-testid="dashboard-preview">
              <div className="w-full overflow-hidden rounded-md border bg-muted/35 shadow-xl">
                <div className="flex h-10 items-center gap-1.5 border-b bg-card px-4"><span className="size-2.5 rounded-full bg-destructive/70" /><span className="size-2.5 rounded-full bg-warning/70" /><span className="size-2.5 rounded-full bg-success/70" /><span className="ml-3 text-[10px] text-muted-foreground">商机雷达 · 看板预览</span></div>
                <div className="grid min-h-[500px] grid-cols-[116px_1fr] sm:grid-cols-[150px_1fr]">
                  <aside className="border-r bg-card p-3"><Brand /><div className="mt-7 space-y-2 text-[10px]"><div className="rounded-md bg-primary/10 px-2 py-2 font-medium text-primary">商机看板</div><div className="px-2 py-2 text-muted-foreground">回复模板</div><div className="px-2 py-2 text-muted-foreground">设置中心</div></div></aside>
                  <div className="min-w-0 p-3 sm:p-4">
                    <div className="mb-3 flex items-center justify-between"><div><strong className="text-sm">商机看板</strong><p className="text-[9px] text-muted-foreground">Telegram 与企业微信</p></div><Badge className="text-[9px]">6 条待处理</Badge></div>
                    <div className="mb-3 rounded-md border border-warning/40 bg-warning/10 p-2.5"><p className="flex items-center gap-1.5 text-[10px] font-semibold"><Sparkles className="size-3 text-warning" />AI 分析与人工审核</p><p className="mt-1 text-[9px] text-muted-foreground">从消息识别需求，核验风险并生成可编辑建议</p></div>
                    <div className="grid gap-2 xl:grid-cols-2"><PreviewCard title="多渠道消息接入" copy="统一查看 Telegram 与企业微信消息" detail="01" /><PreviewCard title="商机自动识别" copy="规则与语义模型结合判断真实需求" detail="02" /><PreviewCard title="链接风险核验" copy="记录公开链接的跳转与风险证据" detail="03" /><PreviewCard title="人工确认跟进" copy="外部动作始终需要人工批准" detail="04" /></div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section id="workflow" className="border-b py-20" data-testid="workflow"><div className="mx-auto max-w-7xl px-4 lg:px-8"><p className="text-sm font-semibold text-primary">工作流程</p><h2 className="mt-2 text-3xl font-semibold">从聊天消息到人工跟进，四步完成</h2><div className="mt-10 grid gap-px overflow-hidden rounded-md border bg-border md:grid-cols-4">{steps.map(([Icon, number, title, copy]) => <div key={number} className="bg-background p-6"><div className="flex items-center justify-between"><span className="grid size-10 place-items-center rounded-md bg-primary/10 text-primary"><Icon className="size-5" /></span><span className="text-xs text-muted-foreground">{number}</span></div><h3 className="mt-7 font-semibold">{title}</h3><p className="mt-2 text-sm leading-6 text-muted-foreground">{copy}</p></div>)}</div></div></section>

        <section id="capabilities" className="border-b bg-muted/30 py-20"><div className="mx-auto max-w-7xl px-4 lg:px-8"><p className="text-sm font-semibold text-primary">产品能力</p><h2 className="mt-2 text-3xl font-semibold">识别、核验、整理，再交给人决策</h2><div className="mt-10 grid gap-x-10 gap-y-9 sm:grid-cols-2 lg:grid-cols-3">{capabilities.map(([Icon, title, copy]) => <div key={title} className="flex gap-4"><span className="grid size-10 shrink-0 place-items-center rounded-md border bg-card"><Icon className="size-5 text-primary" /></span><div><h3 className="text-sm font-semibold">{title}</h3><p className="mt-1.5 text-sm leading-6 text-muted-foreground">{copy}</p></div></div>)}</div></div></section>

        <section id="apps" className="border-b bg-foreground py-20 text-background" data-testid="multi-platform"><div className="mx-auto max-w-7xl px-4 lg:px-8"><p className="text-sm font-semibold text-primary">多端应用</p><h2 className="mt-2 text-3xl font-semibold">完整运营与及时处理，在不同终端协同</h2><div className="mt-10 grid gap-4 md:grid-cols-3"><PlatformPanel icon={Globe2} title="Web 看板" status="可用" copy="完整筛选、详情分析、连接和套餐管理。" /><PlatformPanel icon={Smartphone} title="iOS App" status="Beta" copy="登录、商机查看、详情与订阅页面已实现。" /><PlatformPanel icon={Smartphone} title="Android App" status="Beta" copy="登录、看板、详情与订阅链路已实现。" /></div></div></section>

        <section className="border-b py-20"><div className="mx-auto max-w-7xl px-4 lg:px-8"><p className="text-sm font-semibold text-primary">安全与权限</p><div className="mt-7 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{[[LockKeyhole,'用户级数据隔离'],[ShieldCheck,'Session 服务端加密'],[UserCheck,'外部动作人工审批'],[Link2,'Webhook 验签与幂等'],[Send,'Telegram 原生授权'],[Bot,'不向 AI 发送 Telegram 凭据']].map(([Icon,title]) => { const I=Icon as typeof LockKeyhole; return <div key={title as string} className="flex items-center gap-3 border-b py-4"><I className="size-5 text-success" /><span className="text-sm font-medium">{title as string}</span></div>})}</div></div></section>

        <section className="py-20 text-center"><div className="mx-auto max-w-3xl px-4"><h2 className="text-3xl font-semibold sm:text-4xl">让每一条潜在需求，都有机会被及时看见</h2><div className="mt-8 flex flex-wrap justify-center gap-3"><Button size="lg" nativeButton={false} render={<Link href="/login" />}>登录商机雷达</Button><Button size="lg" variant="outline" nativeButton={false} render={<a href="https://github.com/story2u/IM" target="_blank" rel="noreferrer" />} className="gap-2 bg-transparent"><Github className="size-4" />查看 GitHub</Button></div></div></section>
      </main>
      <footer className="border-t py-8"><div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-4 text-xs text-muted-foreground lg:px-8"><Brand /><span>Web / iOS / Android 多端协同</span></div></footer>
    </div>
  )
}

function PlatformPanel({ icon: Icon, title, status, copy }: { icon: typeof Globe2; title: string; status: string; copy: string }) { return <div className="rounded-md border border-background/15 p-6"><div className="flex items-center justify-between"><Icon className="size-6 text-primary" /><Badge variant="outline" className="border-background/20 text-background">{status}</Badge></div><h3 className="mt-8 text-lg font-semibold">{title}</h3><p className="mt-2 text-sm leading-6 text-background/65">{copy}</p></div> }
