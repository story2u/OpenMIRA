'use client'

import { Bell, BriefcaseBusiness, ChevronRight, Clock, CreditCard, KeyRound, MessageSquare, Send, Tags, X } from 'lucide-react'
import Link from 'next/link'
import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  fetchSettings,
  updateDetectionSettings,
  updateNotificationSettings,
} from '@/lib/api'
import type { NotificationSettings } from '@/lib/types'

const emptyNotifications: NotificationSettings = {
  newOpportunityEnabled: true,
  aiRepliedEnabled: true,
  dailyDigestEnabled: false,
  urgentOnly: false,
}

export default function SettingsPage() {
  const [keywords, setKeywords] = useState<string[]>([])
  const [newKeyword, setNewKeyword] = useState('')
  const [aiSemantics, setAiSemantics] = useState(true)
  const [notifications, setNotifications] = useState<NotificationSettings>(emptyNotifications)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [wecomBindingAvailable, setWecomBindingAvailable] = useState(false)

  // 从后端加载真实设置（与 iOS/Android 同一数据源）；失败不用默认值冒充服务端值。
  useEffect(() => {
    let active = true
    fetchSettings()
      .then((bundle) => {
        if (!active) return
        setKeywords(bundle.detection.keywords)
        setAiSemantics(bundle.detection.aiSemanticsEnabled)
        setNotifications(bundle.notifications)
        setWecomBindingAvailable(bundle.capabilities.wecomUserBindingAvailable)
        setLoaded(true)
      })
      .catch((e) => active && setError(e instanceof Error ? e.message : '加载设置失败'))
    return () => {
      active = false
    }
  }, [])

  // 关键词/AI 语义：本地先改，写回后端；失败回滚。
  const persistDetection = async (nextKeywords: string[], nextAi: boolean) => {
    const prevKeywords = keywords
    const prevAi = aiSemantics
    setKeywords(nextKeywords)
    setAiSemantics(nextAi)
    setError(null)
    try {
      const saved = await updateDetectionSettings({ keywords: nextKeywords, aiSemanticsEnabled: nextAi })
      setKeywords(saved.keywords)
      setAiSemantics(saved.aiSemanticsEnabled)
    } catch (e) {
      setKeywords(prevKeywords)
      setAiSemantics(prevAi)
      setError(e instanceof Error ? e.message : '保存失败')
    }
  }

  const persistNotifications = async (next: NotificationSettings) => {
    const prev = notifications
    setNotifications(next)
    setError(null)
    try {
      setNotifications(await updateNotificationSettings(next))
    } catch (e) {
      setNotifications(prev)
      setError(e instanceof Error ? e.message : '保存失败')
    }
  }

  const addKeyword = () => {
    const kw = newKeyword.trim()
    if (kw && !keywords.includes(kw)) {
      void persistDetection([...keywords, kw], aiSemantics)
    }
    setNewKeyword('')
  }

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 md:px-8">
      <header className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight md:text-2xl">设置中心</h1>
        <p className="mt-1 text-sm text-muted-foreground">管理平台绑定、识别规则与通知偏好</p>
        {error && <p className="mt-2 text-sm text-destructive">{error}</p>}
      </header>

      <div className="flex flex-col gap-5">
        <section aria-labelledby="account-heading">
          <h2 id="account-heading" className="mb-2.5 text-sm font-semibold text-muted-foreground">账户</h2>
          <Link href="/settings/security" className="block">
            <Card className="flex-row items-center gap-3 rounded-xl p-4 shadow-sm transition-shadow hover:shadow-md">
              <span className="flex size-10 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"><KeyRound className="size-5" /></span>
              <div className="min-w-0 flex-1"><p className="text-sm font-medium">账户安全</p><p className="text-xs text-muted-foreground">修改或设置登录密码</p></div>
              <ChevronRight className="size-4 text-muted-foreground" />
            </Card>
          </Link>
        </section>

        <section aria-labelledby="job-search-heading">
          <h2 id="job-search-heading" className="mb-2.5 text-sm font-semibold text-muted-foreground">工作机会</h2>
          <Link href="/settings/job-search" className="block">
            <Card className="flex-row items-center gap-3 rounded-xl p-4 shadow-sm transition-shadow hover:shadow-md">
              <span className="flex size-10 items-center justify-center rounded-lg bg-sky-500/15 text-sky-600 dark:text-sky-400"><BriefcaseBusiness className="size-5" /></span>
              <div className="min-w-0 flex-1"><p className="text-sm font-medium">求职档案</p><p className="text-xs text-muted-foreground">管理目标岗位、技能、地点与薪资偏好</p></div>
              <ChevronRight className="size-4 text-muted-foreground" />
            </Card>
          </Link>
        </section>

        <section aria-labelledby="subscription-heading">
          <h2 id="subscription-heading" className="mb-2.5 text-sm font-semibold text-muted-foreground">
            订阅
          </h2>
          <Link href="/settings/subscription" className="block">
            <Card className="flex-row items-center gap-3 rounded-xl p-4 shadow-sm transition-shadow hover:shadow-md">
              <span className="flex size-10 items-center justify-center rounded-lg bg-violet-500/15 text-violet-600 dark:text-violet-400">
                <CreditCard className="size-5" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">套餐与用量</p>
                <p className="text-xs text-muted-foreground">查看群监控和 AI 分析额度</p>
              </div>
              <ChevronRight className="size-4 text-muted-foreground" />
            </Card>
          </Link>
        </section>

        {/* 平台绑定 */}
        <section aria-labelledby="platform-heading">
          <h2 id="platform-heading" className="mb-2.5 text-sm font-semibold text-muted-foreground">
            平台绑定
          </h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <Link href="/settings/telegram" className="block">
              <Card className="flex-row items-center gap-3 rounded-xl p-4 shadow-sm transition-shadow hover:shadow-md">
                <span className="flex size-10 items-center justify-center rounded-lg bg-sky-500/15 text-sky-600 dark:text-sky-400">
                  <Send className="size-5" />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">Telegram 连接</p>
                  <p className="text-xs text-muted-foreground">群组、Business 与监听来源</p>
                </div>
                <ChevronRight className="size-4 text-muted-foreground" />
              </Card>
            </Link>
            <Link href="/settings/wecom" className="block">
            <Card className="flex-row items-center gap-3 rounded-xl p-4 shadow-sm transition-shadow hover:shadow-md">
              <span className="flex size-10 items-center justify-center rounded-lg bg-success/15 text-success">
                <MessageSquare className="size-5" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">企业微信</p>
                <p className="text-xs text-muted-foreground">
                  {wecomBindingAvailable ? '管理个人企业微信绑定' : '由管理员统一配置'}
                </p>
              </div>
              <Badge variant="secondary" className="border-transparent">
                {wecomBindingAvailable ? '可管理' : '仅管理员'}
              </Badge>
            </Card>
            </Link>
          </div>
        </section>

        {/* 商机识别规则 */}
        <section aria-labelledby="rules-heading">
          <h2 id="rules-heading" className="mb-2.5 text-sm font-semibold text-muted-foreground">
            商机识别规则
          </h2>
          <Card className="gap-5 rounded-xl p-4 shadow-sm md:p-5">
            <div className="flex flex-col gap-2.5">
              <div className="flex items-center gap-2">
                <Tags className="size-4 text-muted-foreground" />
                <Label className="text-sm font-medium">关键词标签</Label>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {keywords.map((keyword) => (
                  <Badge key={keyword} variant="secondary" className="gap-1 rounded-md pr-1 font-normal">
                    {keyword}
                    <button
                      type="button"
                      onClick={() => void persistDetection(keywords.filter((k) => k !== keyword), aiSemantics)}
                      className="rounded-sm p-0.5 hover:bg-foreground/10"
                      aria-label={`删除关键词 ${keyword}`}
                    >
                      <X className="size-3" />
                    </button>
                  </Badge>
                ))}
              </div>
              <div className="flex gap-2">
                <Input
                  value={newKeyword}
                  onChange={(e) => setNewKeyword(e.target.value)}
                  disabled={!loaded}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.nativeEvent.isComposing && e.keyCode !== 229) {
                      e.preventDefault()
                      addKeyword()
                    }
                  }}
                  placeholder="添加新关键词…"
                  className="h-9 max-w-52 text-sm"
                  aria-label="新关键词"
                />
                <Button variant="outline" size="sm" onClick={addKeyword} className="h-9 bg-transparent">
                  添加
                </Button>
              </div>
            </div>
            <div className="flex items-center justify-between gap-4 border-t pt-4">
              <div>
                <Label htmlFor="ai-semantics" className="text-sm font-medium">
                  AI 语义识别
                </Label>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  除关键词外，用大模型理解上下文语义识别潜在商机
                </p>
              </div>
              <Switch id="ai-semantics" checked={aiSemantics} onCheckedChange={(checked) => void persistDetection(keywords, checked)} />
            </div>
          </Card>
        </section>

        {/* 工作时间 */}
        <section aria-labelledby="hours-heading">
          <h2 id="hours-heading" className="mb-2.5 text-sm font-semibold text-muted-foreground">
            工作时间
          </h2>
          <Link href="/settings/working-hours" className="block">
            <Card className="flex-row items-center gap-3 rounded-xl p-4 shadow-sm transition-shadow hover:shadow-md">
              <span className="flex size-10 items-center justify-center rounded-lg bg-warning/15 text-warning">
                <Clock className="size-5" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">工作时间设置</p>
                <p className="text-xs text-muted-foreground">周一至周五 9:00-18:00 · Asia/Shanghai</p>
              </div>
              <ChevronRight className="size-4 text-muted-foreground" />
            </Card>
          </Link>
        </section>

        {/* 通知设置 */}
        <section aria-labelledby="notify-heading">
          <h2 id="notify-heading" className="mb-2.5 text-sm font-semibold text-muted-foreground">
            通知设置
          </h2>
          <Card className="gap-0 rounded-xl p-0 shadow-sm">
            {(
              [
                { key: 'newOpportunityEnabled', label: '新商机提醒', desc: '识别到新商机时立即推送通知' },
                { key: 'aiRepliedEnabled', label: 'AI 代回复通知', desc: '夜间 AI 自动回复后同步告知' },
                { key: 'dailyDigestEnabled', label: '每日商机摘要', desc: '每天早上 9 点汇总前一天商机' },
                { key: 'urgentOnly', label: '仅紧急商机', desc: '开启后仅推送紧急优先级商机' },
              ] as const
            ).map((item, index) => (
              <div
                key={item.key}
                className={`flex items-center justify-between gap-4 px-4 py-3.5 md:px-5 ${index > 0 ? 'border-t' : ''}`}
              >
                <div className="flex items-start gap-3">
                  <Bell className="mt-0.5 size-4 text-muted-foreground" />
                  <div>
                    <Label htmlFor={`notify-${item.key}`} className="text-sm font-medium">
                      {item.label}
                    </Label>
                    <p className="mt-0.5 text-xs text-muted-foreground">{item.desc}</p>
                  </div>
                </div>
                <Switch
                  id={`notify-${item.key}`}
                  checked={notifications[item.key]}
                  onCheckedChange={(checked) => void persistNotifications({ ...notifications, [item.key]: checked })}
                />
              </div>
            ))}
          </Card>
        </section>
      </div>
    </div>
  )
}
