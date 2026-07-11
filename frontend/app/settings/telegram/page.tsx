'use client'

import { ArrowLeft, Check, KeyRound, ListChecks, PauseCircle, RefreshCw, Send, ShieldCheck } from 'lucide-react'
import Link from 'next/link'
import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  fetchTelegramDialogs,
  fetchTelegramUserConfig,
  sendTelegramCode,
  updateTelegramMonitorRetention,
  updateTelegramUserConfig,
  verifyTelegramCode,
} from '@/lib/api'
import type { TelegramDialog, TelegramUserConfig } from '@/lib/types'

function chatsToText(chats: Array<string | number>) {
  return chats.map(String).join('\n')
}

function parseChats(value: string): Array<string | number> {
  const seen = new Set<string>()
  return value
    .split(/\n|,/)
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => (/^-?\d+$/.test(item) ? Number(item) : item))
    .filter((item) => {
      const key = String(item)
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
}

export default function TelegramSettingsPage() {
  const [config, setConfig] = useState<TelegramUserConfig | null>(null)
  const [enabled, setEnabled] = useState(false)
  const [apiId, setApiId] = useState('')
  const [apiHash, setApiHash] = useState('')
  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [sessionString, setSessionString] = useState('')
  const [chatsText, setChatsText] = useState('')
  const [backfillLimit, setBackfillLimit] = useState('50')
  const [loginId, setLoginId] = useState('')
  const [dialogs, setDialogs] = useState<TelegramDialog[]>([])
  const [statusText, setStatusText] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [retentionSaving, setRetentionSaving] = useState(false)
  const [retainedMonitorIds, setRetainedMonitorIds] = useState<Set<string>>(() => new Set())

  const selectedChats = useMemo(() => new Set(parseChats(chatsText).map(String)), [chatsText])
  const monitorLimit = config?.monitorLimit ?? 1
  const monitorErrors = useMemo(
    () =>
      config?.monitors
        .map((monitor) => monitor.lastError)
        .filter((item): item is string => Boolean(item)) ?? [],
    [config],
  )
  const enabledMonitors = useMemo(
    () => config?.monitors.filter((monitor) => monitor.enabled) ?? [],
    [config],
  )
  const hasOverQuotaMonitors = enabledMonitors.length > monitorLimit

  function applyConfig(nextConfig: TelegramUserConfig) {
    const firstMonitor = nextConfig.monitors[0]
    const enabledActiveMonitors = nextConfig.monitors.filter(
      (monitor) => monitor.enabled && !monitor.quotaPaused,
    )
    const editableMonitors = nextConfig.monitors.some((monitor) => monitor.enabled)
      ? enabledActiveMonitors
      : nextConfig.monitors
    setConfig(nextConfig)
    setEnabled(nextConfig.monitors.some((monitor) => monitor.enabled))
    setApiId(nextConfig.apiId ? String(nextConfig.apiId) : '')
    setChatsText(chatsToText(editableMonitors.map((monitor) => monitor.chatId)))
    setRetainedMonitorIds(new Set(enabledActiveMonitors.map((monitor) => monitor.id)))
    setBackfillLimit(String(firstMonitor?.backfillLimit || 50))
  }

  useEffect(() => {
    let cancelled = false
    async function loadConfig() {
      try {
        const nextConfig = await fetchTelegramUserConfig()
        if (cancelled) return
        applyConfig(nextConfig)
      } catch (exc) {
        if (!cancelled) setError(exc instanceof Error ? exc.message : '加载配置失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    loadConfig()
    return () => {
      cancelled = true
    }
  }, [])

  function addChat(chatId: number | string) {
    const next = parseChats(chatsText)
    if (next.map(String).includes(String(chatId))) {
      return
    }
    if (next.length >= monitorLimit) {
      setError(`当前套餐最多监听 ${monitorLimit} 个群/频道`)
      return
    }
    setError('')
    setChatsText(chatsToText([...next, chatId]))
  }

  async function handleSendCode() {
    setError('')
    setStatusText('')
    const numericApiId = Number(apiId)
    if (!numericApiId || !apiHash || !phone) {
      setError('请填写 API ID、API Hash 和手机号')
      return
    }
    try {
      const result = await sendTelegramCode(numericApiId, apiHash, phone)
      setLoginId(result.loginId)
      setStatusText('验证码已发送')
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '发送验证码失败')
    }
  }

  async function handleVerifyCode() {
    setError('')
    setStatusText('')
    if (!loginId || !code) {
      setError('请先发送验证码并填写验证码')
      return
    }
    try {
      const result = await verifyTelegramCode(loginId, code, password)
      if (result.status === 'password_required') {
        setStatusText('需要二步验证密码')
        return
      }
      if (result.config) {
        applyConfig(result.config)
        setStatusText('Telegram 账号已连接')
      }
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '验证码确认失败')
    }
  }

  async function handleLoadDialogs() {
    setError('')
    setStatusText('')
    try {
      const items = await fetchTelegramDialogs()
      setDialogs(items)
      setStatusText(`已加载 ${items.length} 个会话`)
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '加载会话失败')
    }
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    setStatusText('')
    setSaving(true)
    try {
      const chats = parseChats(chatsText)
      if (chats.length > monitorLimit) {
        setError(`当前套餐最多监听 ${monitorLimit} 个群/频道`)
        return
      }
      const nextConfig = await updateTelegramUserConfig({
        enabled,
        apiId: apiId ? Number(apiId) : null,
        apiHash: apiHash || undefined,
        sessionString: sessionString || undefined,
        chats,
        backfillLimit: Number(backfillLimit) || 30,
      })
      applyConfig(nextConfig)
      setApiHash('')
      setSessionString('')
      setStatusText('配置已保存')
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  function toggleRetainedMonitor(monitorId: string, checked: boolean) {
    setError('')
    setRetainedMonitorIds((current) => {
      const next = new Set(current)
      if (checked) {
        if (next.size >= monitorLimit) {
          setError(`当前套餐只能保留 ${monitorLimit} 个群/频道`)
          return current
        }
        next.add(monitorId)
      } else {
        next.delete(monitorId)
      }
      return next
    })
  }

  async function handleSaveRetention() {
    setError('')
    setStatusText('')
    if (retainedMonitorIds.size !== monitorLimit) {
      setError(`请选择恰好 ${monitorLimit} 个要继续监听的群/频道`)
      return
    }
    setRetentionSaving(true)
    try {
      const nextConfig = await updateTelegramMonitorRetention([...retainedMonitorIds])
      applyConfig(nextConfig)
      setStatusText('已保存降级后的保留群，其余群保持暂停')
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '保存保留群失败')
    } finally {
      setRetentionSaving(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 md:px-8">
      <div className="mb-6 flex items-center gap-2">
        <Button
          variant="ghost"
          size="icon"
          aria-label="返回设置中心"
          nativeButton={false}
          render={<Link href="/settings" />}
        >
          <ArrowLeft className="size-4" />
        </Button>
        <div>
          <h1 className="text-lg font-semibold tracking-tight md:text-xl">Telegram 普通账号</h1>
          <p className="text-xs text-muted-foreground">每个登录用户独立保存一套监听配置</p>
        </div>
      </div>

      {hasOverQuotaMonitors ? (
        <Card className="mb-5 gap-4 border-warning/40 bg-warning/5 p-4 shadow-sm md:p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex items-start gap-3">
              <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-warning/15 text-warning">
                <PauseCircle className="size-5" />
              </span>
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-sm font-semibold">选择降级后继续监听的群</h2>
                  <Badge variant={config?.retentionSelectionRequired ? 'destructive' : 'secondary'}>
                    {config?.retentionSelectionRequired ? '需要确认' : '已选择'}
                  </Badge>
                </div>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  当前保存了 {enabledMonitors.length} 个群，本套餐可继续监听 {monitorLimit} 个。未选中的群只会暂停，配置不会被删除。
                </p>
              </div>
            </div>
            <span className="text-sm font-medium">
              已选择 {retainedMonitorIds.size} / {monitorLimit}
            </span>
          </div>

          <div className="grid gap-2 sm:grid-cols-2">
            {enabledMonitors.map((monitor) => {
              const checked = retainedMonitorIds.has(monitor.id)
              const disabled = !checked && retainedMonitorIds.size >= monitorLimit
              return (
                <label
                  key={monitor.id}
                  className="flex cursor-pointer items-center gap-3 rounded-lg border bg-background px-3 py-3 has-disabled:cursor-not-allowed has-disabled:opacity-60"
                >
                  <Checkbox
                    checked={checked}
                    disabled={disabled}
                    onCheckedChange={(nextChecked) => toggleRetainedMonitor(monitor.id, nextChecked)}
                    aria-label={`保留 ${monitor.chatTitle || monitor.name || monitor.chatId}`}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">
                      {monitor.chatTitle || monitor.name || monitor.chatId}
                    </span>
                    <span className="block truncate font-mono text-[11px] text-muted-foreground">
                      {monitor.chatId}
                    </span>
                  </span>
                  <Badge variant={checked ? 'secondary' : 'outline'}>
                    {checked ? '继续监听' : '暂停'}
                  </Badge>
                </label>
              )
            })}
          </div>

          <div className="flex justify-end">
            <Button type="button" onClick={handleSaveRetention} disabled={retentionSaving}>
              {retentionSaving ? '保存中' : '保存保留选择'}
            </Button>
          </div>
        </Card>
      ) : null}

      <form className="grid gap-5 lg:grid-cols-[1fr_320px]" onSubmit={handleSave}>
        <div className="flex flex-col gap-5">
          <Card className="gap-4 p-4 shadow-sm md:p-5">
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-2">
                <Send className="size-4 text-sky-600" />
                <Label htmlFor="telegram-enabled" className="text-sm font-medium">
                  启用监听
                </Label>
              </div>
              <Switch id="telegram-enabled" checked={enabled} onCheckedChange={setEnabled} />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="api-id">API ID</Label>
                <Input id="api-id" value={apiId} onChange={(event) => setApiId(event.target.value)} inputMode="numeric" />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="api-hash">API Hash</Label>
                <Input
                  id="api-hash"
                  type="password"
                  value={apiHash}
                  onChange={(event) => setApiHash(event.target.value)}
                  placeholder={config?.apiHashConfigured ? '已配置，留空不修改' : ''}
                />
              </div>
            </div>
          </Card>

          <Card className="gap-4 p-4 shadow-sm md:p-5">
            <div className="flex items-center gap-2">
              <KeyRound className="size-4 text-muted-foreground" />
              <h2 className="text-sm font-medium">账号登录</h2>
            </div>
            <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
              <div className="grid gap-1.5">
                <Label htmlFor="phone">手机号</Label>
                <Input id="phone" value={phone} onChange={(event) => setPhone(event.target.value)} placeholder="+8613800000000" />
              </div>
              <div className="flex items-end">
                <Button type="button" variant="outline" onClick={handleSendCode} disabled={loading}>
                  发送验证码
                </Button>
              </div>
            </div>
            <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto]">
              <div className="grid gap-1.5">
                <Label htmlFor="code">验证码</Label>
                <Input id="code" value={code} onChange={(event) => setCode(event.target.value)} />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="twofa">二步验证密码</Label>
                <Input id="twofa" type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
              </div>
              <div className="flex items-end">
                <Button type="button" variant="outline" onClick={handleVerifyCode}>
                  确认
                </Button>
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="session">Session String</Label>
              <Textarea
                id="session"
                value={sessionString}
                onChange={(event) => setSessionString(event.target.value)}
                placeholder={config?.sessionConfigured ? '已配置，留空不修改' : '可粘贴脚本生成的 session'}
                className="min-h-20 font-mono text-xs"
              />
            </div>
          </Card>

          <Card className="gap-4 p-4 shadow-sm md:p-5">
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <ListChecks className="size-4 text-muted-foreground" />
                <h2 className="text-sm font-medium">监听群/频道</h2>
              </div>
              <Button type="button" variant="outline" size="sm" onClick={handleLoadDialogs}>
                <RefreshCw className="size-4" />
                加载会话
              </Button>
            </div>
            <Textarea
              value={chatsText}
              onChange={(event) => setChatsText(event.target.value)}
              className="min-h-32 font-mono text-xs"
              placeholder={'-1001234567890\npublic_jobs_channel'}
            />
            {dialogs.length > 0 && (
              <div className="max-h-72 overflow-auto rounded-md border">
                {dialogs.map((dialog) => (
                  <div key={dialog.id} className="flex items-center justify-between gap-3 border-b px-3 py-2 last:border-b-0">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{dialog.name}</p>
                      <p className="truncate font-mono text-[11px] text-muted-foreground">
                        {dialog.id}
                        {dialog.username ? ` · @${dialog.username}` : ''}
                      </p>
                    </div>
                    <Button
                      type="button"
                      variant={selectedChats.has(String(dialog.id)) ? 'secondary' : 'outline'}
                      size="sm"
                      disabled={!selectedChats.has(String(dialog.id)) && selectedChats.size >= monitorLimit}
                      onClick={() => addChat(dialog.id)}
                    >
                      {selectedChats.has(String(dialog.id)) ? <Check className="size-4" /> : '添加'}
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </Card>
        </div>

        <aside className="flex flex-col gap-5">
          <Card className="gap-4 p-4 shadow-sm">
            <div className="flex items-center gap-2">
              <ShieldCheck className="size-4 text-muted-foreground" />
              <h2 className="text-sm font-medium">状态</h2>
            </div>
            <div className="grid gap-2 text-sm">
              <p>API Hash：{config?.apiHashConfigured ? '已保存' : '未配置'}</p>
              <p>Session：{config?.sessionConfigured ? '已保存' : '未配置'}</p>
              <p>
                正在监听：{config?.activeMonitorCount ?? parseChats(chatsText).length} / {monitorLimit}
              </p>
              {monitorErrors.map((item) => (
                <p key={item} className="text-destructive">
                  错误：{item}
                </p>
              ))}
            </div>
          </Card>

          <Card className="gap-3 p-4 shadow-sm">
            <div className="grid gap-1.5">
              <Label htmlFor="backfill">历史回填条数</Label>
              <Input
                id="backfill"
                type="number"
                min={0}
                max={500}
                value={backfillLimit}
                onChange={(event) => setBackfillLimit(event.target.value)}
              />
            </div>
            {statusText && <p className="rounded-md bg-success/10 px-3 py-2 text-xs text-success">{statusText}</p>}
            {error && <p className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</p>}
            <Button type="submit" disabled={loading || saving}>
              {saving ? '保存中' : '保存配置'}
            </Button>
          </Card>
        </aside>
      </form>
    </div>
  )
}
