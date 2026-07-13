'use client'

import {
  ArrowLeft,
  Check,
  Copy,
  Loader2,
  MessageSquare,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
import Link from 'next/link'
import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  createWeComConnection,
  deleteWeComConnection,
  fetchWeComConnections,
  verifyWeComConnection,
} from '@/lib/api'
import type { WeComConnection, WeComConnectionCreate } from '@/lib/types'

const emptyForm: WeComConnectionCreate = {
  displayName: '企业微信自建应用',
  corpId: '',
  agentId: '',
  secret: '',
  token: '',
  encodingAesKey: '',
}

const statusLabel = {
  pending: '待验证',
  active: '已连接',
  disabled: '已停用',
  error: '需要处理',
}

export default function WeComSettingsPage() {
  const [connections, setConnections] = useState<WeComConnection[]>([])
  const [form, setForm] = useState<WeComConnectionCreate>(emptyForm)
  const [loading, setLoading] = useState(true)
  const [action, setAction] = useState('')
  const [error, setError] = useState('')
  const [copied, setCopied] = useState('')

  const load = useCallback(async () => {
    setConnections(await fetchWeComConnections())
  }, [])

  useEffect(() => {
    load()
      .catch((exc) => setError(exc instanceof Error ? exc.message : '无法加载企业微信连接'))
      .finally(() => setLoading(false))
  }, [load])

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setAction('create')
    setError('')
    try {
      const connection = await createWeComConnection(form)
      setConnections([connection])
      setForm(emptyForm)
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '创建连接失败')
    } finally {
      setAction('')
    }
  }

  const verify = async (connection: WeComConnection) => {
    setAction(`verify-${connection.id}`)
    setError('')
    try {
      const updated = await verifyWeComConnection(connection.id)
      setConnections((items) => items.map((item) => item.id === updated.id ? updated : item))
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '验证失败')
    } finally {
      setAction('')
    }
  }

  const remove = async (connection: WeComConnection) => {
    if (!window.confirm(`删除“${connection.displayName}”并停止接收消息？`)) return
    setAction(`delete-${connection.id}`)
    setError('')
    try {
      await deleteWeComConnection(connection.id)
      setConnections((items) => items.filter((item) => item.id !== connection.id))
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '删除失败')
    } finally {
      setAction('')
    }
  }

  const copy = async (value: string) => {
    await navigator.clipboard.writeText(value)
    setCopied(value)
    window.setTimeout(() => setCopied(''), 1500)
  }

  return (
    <main className="mx-auto w-full max-w-4xl px-4 py-6 md:px-8">
      <header className="mb-6 flex items-start gap-3">
        <Button variant="ghost" size="icon" nativeButton={false} render={<Link href="/settings" />} aria-label="返回设置">
          <ArrowLeft className="size-4" />
        </Button>
        <div>
          <h1 className="text-xl font-semibold md:text-2xl">企业微信连接</h1>
          <p className="mt-1 text-sm text-muted-foreground">通过企业内部自建应用接收成员私聊消息并人工回复</p>
        </div>
      </header>

      {error && <p className="mb-4 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{error}</p>}

      <section className="mb-6 border-y bg-muted/30 px-4 py-4">
        <div className="flex gap-3">
          <ShieldCheck className="mt-0.5 size-5 shrink-0 text-primary" />
          <div className="text-sm leading-6">
            <p className="font-medium">当前版本的能力边界</p>
            <p className="text-muted-foreground">支持成员向自建应用发送的文本消息。不支持监听普通内部群或客户群；群聊需后续接入会话内容存档。企业微信回复始终需要人工确认。</p>
          </div>
        </div>
      </section>

      {loading ? (
        <div className="flex h-36 items-center justify-center"><Loader2 className="size-5 animate-spin" /></div>
      ) : connections.length > 0 ? (
        <section className="space-y-4" aria-label="已配置的企业微信连接">
          {connections.map((connection) => (
            <Card key={connection.id} className="gap-5 p-5">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex gap-3">
                  <span className="grid size-10 place-items-center rounded-md bg-emerald-500/15 text-emerald-600"><MessageSquare className="size-5" /></span>
                  <div><p className="font-medium">{connection.displayName}</p><p className="text-xs text-muted-foreground">CorpID {connection.corpId} · AgentID {connection.agentId}</p></div>
                </div>
                <Badge variant={connection.status === 'active' ? 'secondary' : 'outline'}>{statusLabel[connection.status]}</Badge>
              </div>

              <div>
                <Label className="mb-2 text-xs text-muted-foreground">回调 URL</Label>
                <div className="flex gap-2"><Input readOnly value={connection.callbackUrl} /><Button type="button" variant="outline" size="icon" onClick={() => void copy(connection.callbackUrl)} aria-label="复制回调 URL">{copied === connection.callbackUrl ? <Check className="size-4" /> : <Copy className="size-4" />}</Button></div>
                <p className="mt-2 text-xs text-muted-foreground">将此 URL 与创建时的 Token、EncodingAESKey 填入企业微信自建应用的“接收消息”配置。</p>
              </div>

              <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
                <p className="text-xs text-muted-foreground">已发现 {connection.sources.length} 个成员会话</p>
                <div className="flex gap-2"><Button variant="outline" onClick={() => void verify(connection)} disabled={action === `verify-${connection.id}`}><RefreshCw className="size-4" />验证凭据</Button><Button variant="ghost" size="icon" onClick={() => void remove(connection)} disabled={action === `delete-${connection.id}`} aria-label="删除连接"><Trash2 className="size-4" /></Button></div>
              </div>
            </Card>
          ))}
        </section>
      ) : (
        <form onSubmit={(event) => void create(event)} className="space-y-5">
          <div><h2 className="text-base font-semibold">添加自建应用</h2><p className="mt-1 text-sm text-muted-foreground">密钥经服务端加密后保存，不会在页面再次显示。</p></div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="连接名称" value={form.displayName} onChange={(value) => setForm({ ...form, displayName: value })} />
            <Field label="CorpID" value={form.corpId} onChange={(value) => setForm({ ...form, corpId: value })} />
            <Field label="AgentID" value={form.agentId} onChange={(value) => setForm({ ...form, agentId: value })} />
            <Field label="Secret" value={form.secret} secret onChange={(value) => setForm({ ...form, secret: value })} />
            <Field label="Token" value={form.token} secret onChange={(value) => setForm({ ...form, token: value })} />
            <Field label="EncodingAESKey" value={form.encodingAesKey} secret onChange={(value) => setForm({ ...form, encodingAesKey: value })} />
          </div>
          <Button type="submit" disabled={action === 'create'}>{action === 'create' && <Loader2 className="size-4 animate-spin" />}创建连接</Button>
        </form>
      )}
    </main>
  )
}

function Field({ label, value, onChange, secret = false }: { label: string; value: string; onChange: (value: string) => void; secret?: boolean }) {
  return <div className="space-y-2"><Label>{label}</Label><Input required type={secret ? 'password' : 'text'} value={value} autoComplete="off" onChange={(event) => onChange(event.target.value)} /></div>
}
