'use client'

import {
  ArrowLeft,
  Bot,
  Building2,
  CheckCircle2,
  ExternalLink,
  Loader2,
  QrCode,
  Radio,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Unplug,
} from 'lucide-react'
import Link from 'next/link'
import { QRCodeSVG } from 'qrcode.react'
import { useCallback, useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import {
  cancelTelegramConnectionAttempt,
  addTelegramMtprotoSource,
  deleteTelegramConnection,
  deleteTelegramConnectionSource,
  fetchTelegramConnectionAttempt,
  fetchTelegramConnectionHealth,
  fetchTelegramConnections,
  fetchTelegramMtprotoDialogs,
  startTelegramBotChatConnection,
  startTelegramBusinessConnection,
  startTelegramMtprotoQrConnection,
  updateTelegramConnection,
} from '@/lib/api'
import type {
  TelegramConnection,
  TelegramConnectionAttempt,
  TelegramConnectionHealth,
  TelegramMtprotoDialog,
  TelegramConnectionStatus,
} from '@/lib/types'

const STATUS_LABELS: Record<TelegramConnectionStatus, string> = {
  pending: '等待连接',
  connected: '已连接',
  disabled: '已停用',
  error: '需要处理',
  expired: '已过期',
}

function statusVariant(status: TelegramConnectionStatus) {
  if (status === 'connected') return 'secondary' as const
  if (status === 'error') return 'destructive' as const
  return 'outline' as const
}

function attemptStatusLabel(attempt: TelegramConnectionAttempt) {
  if (attempt.localMock) return '本地 mock 已完成'
  if (attempt.status === 'pending') return '等待你在 Telegram 中完成操作'
  if (attempt.status === 'completed') return '连接已完成'
  if (attempt.status === 'expired') return '连接已过期，请重新开始'
  if (attempt.status === 'cancelled') return '连接已取消'
  return attempt.error || '连接失败，请重试'
}

export default function TelegramSettingsPage() {
  const [health, setHealth] = useState<TelegramConnectionHealth | null>(null)
  const [connections, setConnections] = useState<TelegramConnection[]>([])
  const [attempt, setAttempt] = useState<TelegramConnectionAttempt | null>(null)
  const [loading, setLoading] = useState(true)
  const [dialogs, setDialogs] = useState<Record<string, TelegramMtprotoDialog[]>>({})
  const [action, setAction] = useState<'bot' | 'business' | 'qr' | 'refresh' | string | null>(null)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    const [nextHealth, nextConnections] = await Promise.all([
      fetchTelegramConnectionHealth(),
      fetchTelegramConnections(),
    ])
    setHealth(nextHealth)
    setConnections(nextConnections)
  }, [])

  useEffect(() => {
    let active = true
    load()
      .catch((exc) => {
        if (active) setError(exc instanceof Error ? exc.message : '无法加载 Telegram 连接')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [load])

  useEffect(() => {
    if (!attempt || attempt.status !== 'pending' || attempt.localMock) return
    let active = true
    const poll = async () => {
      try {
        const nextAttempt = await fetchTelegramConnectionAttempt(attempt.id)
        if (!active) return
        setAttempt((current) => current ? {
          ...nextAttempt,
          telegramUrl: nextAttempt.telegramUrl ?? current.telegramUrl,
          qrCodeUrl: nextAttempt.qrCodeUrl ?? current.qrCodeUrl,
          instructions: current.instructions,
          localMock: current.localMock,
        } : nextAttempt)
        if (nextAttempt.status === 'completed') {
          await load()
        }
      } catch (exc) {
        if (active) setError(exc instanceof Error ? exc.message : '无法更新连接状态')
      }
    }
    const timer = window.setInterval(poll, 2_000)
    void poll()
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [attempt, load])

  const refresh = useCallback(async () => {
    setError('')
    setAction('refresh')
    try {
      await load()
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '刷新失败')
    } finally {
      setAction(null)
    }
  }, [load])

  const startBotChat = useCallback(async () => {
    setError('')
    setAction('bot')
    try {
      const nextAttempt = await startTelegramBotChatConnection()
      setAttempt(nextAttempt)
      if (nextAttempt.status === 'completed') await load()
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '无法开始群组连接')
    } finally {
      setAction(null)
    }
  }, [load])

  const startBusiness = useCallback(async () => {
    setError('')
    setAction('business')
    try {
      setAttempt(await startTelegramBusinessConnection())
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '无法开始 Business 连接')
    } finally {
      setAction(null)
    }
  }, [])

  const startMtprotoQr = useCallback(async () => {
    setError('')
    setAction('qr')
    try {
      setAttempt(await startTelegramMtprotoQrConnection())
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '无法启动二维码登录')
    } finally {
      setAction(null)
    }
  }, [])

  const loadMtprotoDialogs = useCallback(async (connectionId: string) => {
    setError('')
    setAction(`dialogs-${connectionId}`)
    try {
      const nextDialogs = await fetchTelegramMtprotoDialogs(connectionId)
      setDialogs((current) => ({ ...current, [connectionId]: nextDialogs }))
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '无法读取可监听群组')
    } finally {
      setAction(null)
    }
  }, [])

  const addMtprotoSource = useCallback(async (connectionId: string, chatId: string) => {
    setError('')
    setAction(`dialog-${chatId}`)
    try {
      const updated = await addTelegramMtprotoSource(connectionId, chatId)
      setConnections((current) => current.map((item) => item.id === updated.id ? updated : item))
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '添加监听来源失败')
    } finally {
      setAction(null)
    }
  }, [])

  const cancelAttempt = useCallback(async () => {
    if (!attempt) return
    setError('')
    setAction(`cancel-${attempt.id}`)
    try {
      setAttempt(await cancelTelegramConnectionAttempt(attempt.id))
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '取消连接失败')
    } finally {
      setAction(null)
    }
  }, [attempt])

  const toggleConnection = useCallback(async (connection: TelegramConnection, enabled: boolean) => {
    setError('')
    setAction(`connection-${connection.id}`)
    try {
      const updated = await updateTelegramConnection(connection.id, enabled)
      setConnections((current) => current.map((item) => (item.id === updated.id ? updated : item)))
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '更新连接状态失败')
    } finally {
      setAction(null)
    }
  }, [])

  const removeConnection = useCallback(
    async (connection: TelegramConnection) => {
      if (!window.confirm(`移除“${connection.label}”及其全部监听来源？`)) return
      setError('')
      setAction(`connection-${connection.id}`)
      try {
        await deleteTelegramConnection(connection.id)
        setConnections((current) => current.filter((item) => item.id !== connection.id))
      } catch (exc) {
        setError(exc instanceof Error ? exc.message : '移除连接失败')
      } finally {
        setAction(null)
      }
    },
    [],
  )

  const removeSource = useCallback(async (sourceId: string) => {
    if (!window.confirm('停止监听并移除此来源？')) return
    setError('')
    setAction(`source-${sourceId}`)
    try {
      await deleteTelegramConnectionSource(sourceId)
      setConnections((current) =>
        current.map((connection) => ({
          ...connection,
          sources: connection.sources.filter((source) => source.id !== sourceId),
        })),
      )
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '移除来源失败')
    } finally {
      setAction(null)
    }
  }, [])

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 md:px-8">
      <header className="mb-6 flex items-start gap-2">
        <Button
          variant="ghost"
          size="icon"
          aria-label="返回设置中心"
          nativeButton={false}
          render={<Link href="/settings" />}
        >
          <ArrowLeft className="size-4" />
        </Button>
        <div className="min-w-0 flex-1">
          <h1 className="text-xl font-semibold tracking-tight md:text-2xl">Telegram 连接</h1>
          <p className="mt-1 text-sm text-muted-foreground">选择连接方式并管理正在监听的来源</p>
        </div>
        <Button variant="outline" size="sm" onClick={refresh} disabled={action === 'refresh'}>
          {action === 'refresh' ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}
          刷新
        </Button>
      </header>

      {health?.mode === 'mock' ? (
        <div className="mb-5 flex gap-3 rounded-xl border border-warning/35 bg-warning/5 p-4 text-sm">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-warning" />
          <p>当前为本地 mock adapter。它只验证连接流程，不会连接、读取或发送真实 Telegram 消息。</p>
        </div>
      ) : null}

      {health?.legacyMonitoringActive ? (
        <div className="mb-5 flex gap-3 rounded-xl border border-border bg-muted/45 p-4 text-sm">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <p>检测到 {health.legacyActiveSourceCount} 个旧 MTProto 监听仍在运行。为保护既有授权，系统不会迁移或展示其凭据；它们仍会计入套餐群额度。</p>
        </div>
      ) : null}

      {health?.message ? <p className="mb-5 text-xs text-muted-foreground">{health.message}</p> : null}
      {error ? <p className="mb-5 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive" role="alert">{error}</p> : null}

      <section aria-labelledby="connection-methods" className="mb-7">
        <div className="mb-3 flex items-center justify-between gap-3">
          <div>
            <h2 id="connection-methods" className="text-base font-semibold">选择连接方式</h2>
            <p className="mt-0.5 text-sm text-muted-foreground">不会在网页中输入 Telegram API Hash、手机号、验证码或 Session。</p>
          </div>
          <Badge variant="outline">{health?.listenerMode || '加载中'}</Badge>
        </div>
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="gap-4 border-sky-500/35 bg-sky-500/[0.03] p-5 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <span className="flex size-10 items-center justify-center rounded-xl bg-sky-500/12 text-sky-600 dark:text-sky-400"><Bot className="size-5" /></span>
              <Badge className="bg-sky-500 text-white">推荐 · P0</Badge>
            </div>
            <div>
              <h3 className="font-semibold">群组 / 频道</h3>
              <p className="mt-1 text-sm leading-5 text-muted-foreground">通过平台机器人选择已添加机器人的群或频道，验证后开始监听。</p>
            </div>
            <div className="mt-auto">
              <Button className="w-full" onClick={startBotChat} disabled={loading || action === 'bot' || !health}>
                {action === 'bot' ? <Loader2 className="size-4 animate-spin" /> : <Radio className="size-4" />}
                连接群组或频道
              </Button>
            </div>
          </Card>

          <Card className="gap-4 p-5 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <span className="flex size-10 items-center justify-center rounded-xl bg-violet-500/12 text-violet-600 dark:text-violet-400"><Building2 className="size-5" /></span>
              <Badge variant="outline">P1</Badge>
            </div>
            <div>
              <h3 className="font-semibold">Business 私聊</h3>
              <p className="mt-1 text-sm leading-5 text-muted-foreground">在 Telegram Business 设置中绑定机器人，用于接收授权范围内的私聊。</p>
            </div>
            <div className="mt-auto">
              <Button className="w-full" variant="outline" onClick={startBusiness} disabled={!health?.businessAvailable || action === 'business'}>
                {action === 'business' ? <Loader2 className="size-4 animate-spin" /> : <Building2 className="size-4" />}
                {health?.businessAvailable ? '开始 Business 连接' : '管理员尚未配置'}
              </Button>
            </div>
          </Card>

          <Card className="gap-4 p-5 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <span className="flex size-10 items-center justify-center rounded-xl bg-muted text-muted-foreground"><QrCode className="size-5" /></span>
              <Badge variant="outline">P2 · QR</Badge>
            </div>
            <div>
              <h3 className="font-semibold">普通账号 QR</h3>
              <p className="mt-1 text-sm leading-5 text-muted-foreground">使用平台统一配置扫码登录普通账号，再选择你已加入的群组或频道监听。</p>
            </div>
            <Button className="mt-auto w-full" variant="outline" onClick={startMtprotoQr} disabled={!health?.mtprotoQrAvailable || action === 'qr'}>
              {action === 'qr' ? <Loader2 className="size-4 animate-spin" /> : <QrCode className="size-4" />}
              {health?.mtprotoQrAvailable ? '显示登录二维码' : '管理员尚未配置'}
            </Button>
          </Card>
        </div>
      </section>

      {attempt ? (
        <Card className="mb-7 gap-4 border-primary/30 bg-primary/[0.035] p-5 shadow-sm" aria-live="polite">
          <div className="flex items-start gap-3">
            {attempt.status === 'completed' ? <CheckCircle2 className="mt-0.5 size-5 text-success" /> : <Radio className="mt-0.5 size-5 text-primary" />}
            <div className="min-w-0 flex-1">
              <h2 className="font-semibold">连接进度</h2>
              <p className="mt-1 text-sm text-muted-foreground">{attemptStatusLabel(attempt)}</p>
            </div>
            <Badge variant={attempt.status === 'completed' ? 'secondary' : 'outline'}>{attempt.status}</Badge>
          </div>
          {attempt.instructions.length > 0 ? (
            <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
              {attempt.instructions.map((instruction) => <li key={instruction}>{instruction}</li>)}
            </ol>
          ) : null}
          {attempt.qrCodeUrl && attempt.status === 'pending' ? (
            <div className="mx-auto rounded-xl bg-white p-3 w-fit" aria-label="Telegram 登录二维码">
              <QRCodeSVG value={attempt.qrCodeUrl} size={192} level="M" includeMargin />
            </div>
          ) : null}
          <div className="flex flex-wrap gap-2">
            {attempt.telegramUrl ? (
              <Button nativeButton={false} render={<a href={attempt.telegramUrl} target="_blank" rel="noreferrer" />}>
                <ExternalLink className="size-4" />
                在 Telegram 中打开
              </Button>
            ) : null}
            {attempt.status === 'pending' ? (
              <Button variant="outline" onClick={cancelAttempt} disabled={action === `cancel-${attempt.id}`}>
                <Unplug className="size-4" />
                取消连接
              </Button>
            ) : null}
            {attempt.status !== 'pending' ? <Button variant="ghost" onClick={() => setAttempt(null)}>关闭</Button> : null}
          </div>
        </Card>
      ) : null}

      <section aria-labelledby="current-connections">
        <div className="mb-3">
          <h2 id="current-connections" className="text-base font-semibold">当前连接</h2>
          <p className="mt-0.5 text-sm text-muted-foreground">停用不会删除来源；移除连接会停止并删除其来源。</p>
        </div>
        {loading ? (
          <Card className="items-center p-8 text-sm text-muted-foreground"><Loader2 className="size-5 animate-spin" />正在加载连接…</Card>
        ) : connections.length === 0 ? (
          <Card className="items-center gap-2 p-8 text-center shadow-sm">
            <Bot className="size-6 text-muted-foreground" />
            <p className="font-medium">还没有 Telegram 连接</p>
            <p className="text-sm text-muted-foreground">从上方选择一种方式开始；群组/频道是当前可用的推荐路径。</p>
          </Card>
        ) : (
          <div className="flex flex-col gap-3">
            {connections.map((connection) => (
              <Card key={connection.id} className="gap-4 p-4 shadow-sm md:p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-sky-500/12 text-sky-600 dark:text-sky-400"><Radio className="size-4" /></span>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">{connection.label}</h3><Badge variant={statusVariant(connection.status)}>{STATUS_LABELS[connection.status]}</Badge></div>
                      <p className="mt-1 text-xs text-muted-foreground">{connection.connectionType === 'bot_chat' ? '机器人群组/频道' : connection.connectionType === 'business' ? 'Telegram Business 私聊' : '普通账号 QR'}</p>
                      {connection.lastError ? <p className="mt-1 text-xs text-destructive">{connection.lastError}</p> : null}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch
                      checked={connection.enabled}
                      disabled={action === `connection-${connection.id}`}
                      onCheckedChange={(enabled) => void toggleConnection(connection, enabled)}
                      aria-label={`${connection.enabled ? '停用' : '启用'} ${connection.label}`}
                    />
                    <Button variant="ghost" size="icon-sm" onClick={() => void removeConnection(connection)} disabled={action === `connection-${connection.id}`} aria-label={`移除 ${connection.label}`}><Trash2 className="size-4 text-destructive" /></Button>
                  </div>
                </div>
                {connection.sources.length > 0 ? (
                  <div className="grid gap-2 border-t pt-3">
                    {connection.sources.map((source) => (
                      <div key={source.id} className="flex items-center gap-3 rounded-lg bg-muted/45 px-3 py-2.5">
                        <Radio className="size-3.5 shrink-0 text-muted-foreground" />
                        <div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{source.displayName}</p><p className="truncate text-xs text-muted-foreground">{source.sourceType === 'channel' ? '频道' : source.sourceType === 'group' ? '群组' : '私聊'}{source.username ? ` · @${source.username}` : ''}</p></div>
                        {source.quotaPaused ? <Badge variant="outline">套餐暂停</Badge> : <Badge variant="secondary">监听中</Badge>}
                        <Button variant="ghost" size="icon-xs" onClick={() => void removeSource(source.id)} disabled={action === `source-${source.id}`} aria-label={`移除 ${source.displayName}`}><Trash2 className="size-3.5 text-destructive" /></Button>
                      </div>
                    ))}
                  </div>
                ) : <p className="border-t pt-3 text-sm text-muted-foreground">尚未添加来源。</p>}
                {connection.connectionType === 'mtproto_qr' ? (
                  <div className="border-t pt-3">
                    <Button variant="outline" size="sm" onClick={() => void loadMtprotoDialogs(connection.id)} disabled={action === `dialogs-${connection.id}`}>
                      {action === `dialogs-${connection.id}` ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}
                      选择要监听的群
                    </Button>
                    {dialogs[connection.id]?.length ? (
                      <div className="mt-3 grid gap-2">
                        {dialogs[connection.id].map((dialog) => (
                          <div key={dialog.id} className="flex items-center justify-between gap-3 rounded-lg bg-muted/45 px-3 py-2">
                            <span className="min-w-0 truncate text-sm">{dialog.displayName}{dialog.username ? ` · @${dialog.username}` : ''}</span>
                            <Button size="sm" variant="ghost" onClick={() => void addMtprotoSource(connection.id, dialog.id)} disabled={action === `dialog-${dialog.id}`}>监听</Button>
                          </div>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </Card>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
