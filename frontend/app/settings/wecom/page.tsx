'use client'

import {
  ArrowLeft,
  Check,
  Copy,
  Database,
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
import { Textarea } from '@/components/ui/textarea'
import {
  createWeComArchiveConnection,
  createWeComConnection,
  deleteWeComArchiveConnection,
  deleteWeComConnection,
  fetchWeComArchiveConnections,
  fetchWeComConnections,
  syncWeComArchiveConnection,
  verifyWeComArchiveConnection,
  verifyWeComConnection,
} from '@/lib/api'
import type {
  WeComArchiveConnection,
  WeComArchiveConnectionCreate,
  WeComConnection,
  WeComConnectionCreate,
} from '@/lib/types'

const emptyForm: WeComConnectionCreate = {
  displayName: '企业微信自建应用',
  corpId: '',
  agentId: '',
  secret: '',
  token: '',
  encodingAesKey: '',
}

const emptyArchiveForm: WeComArchiveConnectionCreate = {
  displayName: '企业微信会话存档',
  corpId: '',
  archiveSecret: '',
  privateKeyPem: '',
  publicKeyVersion: 1,
  wecomUserId: '',
  memberDisplayName: '',
}

const statusLabel = {
  pending: '待验证',
  active: '已连接',
  disabled: '已停用',
  error: '需要处理',
}

export default function WeComSettingsPage() {
  const [connections, setConnections] = useState<WeComConnection[]>([])
  const [archiveConnections, setArchiveConnections] = useState<WeComArchiveConnection[]>([])
  const [form, setForm] = useState<WeComConnectionCreate>(emptyForm)
  const [archiveForm, setArchiveForm] = useState<WeComArchiveConnectionCreate>(emptyArchiveForm)
  const [loading, setLoading] = useState(true)
  const [action, setAction] = useState('')
  const [error, setError] = useState('')
  const [copied, setCopied] = useState('')
  const [notice, setNotice] = useState('')

  const load = useCallback(async () => {
    const [internalApps, archives] = await Promise.all([
      fetchWeComConnections(),
      fetchWeComArchiveConnections(),
    ])
    setConnections(internalApps)
    setArchiveConnections(archives)
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

  const createArchive = async (event: FormEvent) => {
    event.preventDefault()
    setAction('create-archive')
    setError('')
    setNotice('')
    try {
      const connection = await createWeComArchiveConnection(archiveForm)
      setArchiveConnections([connection])
      setArchiveForm(emptyArchiveForm)
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '创建会话存档连接失败')
    } finally {
      setAction('')
    }
  }

  const requestArchiveSync = async (connection: WeComArchiveConnection, verifying: boolean) => {
    setAction(`${verifying ? 'verify' : 'sync'}-archive-${connection.id}`)
    setError('')
    setNotice('')
    try {
      if (verifying) await verifyWeComArchiveConnection(connection.id)
      else await syncWeComArchiveConnection(connection.id)
      setNotice(verifying ? '验证任务已提交，完成后连接状态会自动更新。' : '同步任务已提交。')
      window.setTimeout(() => void load(), 1800)
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '提交会话存档任务失败')
    } finally {
      setAction('')
    }
  }

  const removeArchive = async (connection: WeComArchiveConnection) => {
    if (!window.confirm(`删除“${connection.displayName}”并停止拉取会话记录？`)) return
    setAction(`delete-archive-${connection.id}`)
    setError('')
    try {
      await deleteWeComArchiveConnection(connection.id)
      setArchiveConnections((items) => items.filter((item) => item.id !== connection.id))
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '删除会话存档连接失败')
    } finally {
      setAction('')
    }
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
      {notice && <p className="mb-4 rounded-md border border-emerald-500/30 bg-emerald-500/5 p-3 text-sm text-emerald-700">{notice}</p>}

      <section className="mb-6 border-y bg-muted/30 px-4 py-4">
        <div className="flex gap-3">
          <ShieldCheck className="mt-0.5 size-5 shrink-0 text-primary" />
          <div className="text-sm leading-6">
            <p className="font-medium">当前版本的能力边界</p>
            <p className="text-muted-foreground">自建应用只接收发给应用的消息。普通私聊和群聊必须由企业管理员开通会话内容存档，并把当前成员加入存档范围。存档消息只读，回复需回到企业微信人工处理。</p>
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

      <section className="mt-10 border-t pt-8" aria-labelledby="archive-heading">
        <div className="mb-5 flex gap-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-md bg-blue-500/15 text-blue-600"><Database className="size-5" /></span>
          <div>
            <h2 id="archive-heading" className="text-base font-semibold">会话内容存档</h2>
            <p className="mt-1 text-sm text-muted-foreground">企业级付费能力，可读取已授权成员参与的普通私聊和群聊。需要企业管理员配置 Finance SDK。</p>
          </div>
        </div>

        {archiveConnections.length > 0 ? (
          <div className="space-y-4">
            {archiveConnections.map((connection) => (
              <Card key={connection.id} className="gap-5 p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="font-medium">{connection.displayName}</p>
                    <p className="text-xs text-muted-foreground">CorpID {connection.corpId} · 成员 {connection.member.displayName} ({connection.member.wecomUserId})</p>
                  </div>
                  <Badge variant={connection.status === 'active' ? 'secondary' : 'outline'}>{statusLabel[connection.status]}</Badge>
                </div>
                {!connection.sdkConfigured && <p className="rounded-md border border-amber-500/30 bg-amber-500/5 p-3 text-sm text-amber-700">服务端尚未启用或挂载企业微信 Finance SDK，当前不会拉取消息。</p>}
                {connection.lastError && <p className="text-sm text-destructive">最近错误：{connection.lastError}</p>}
                <dl className="grid gap-3 text-sm sm:grid-cols-3">
                  <div><dt className="text-xs text-muted-foreground">最近序号</dt><dd>{connection.lastSequence}</dd></div>
                  <div><dt className="text-xs text-muted-foreground">已发现会话</dt><dd>{connection.sources.length}</dd></div>
                  <div><dt className="text-xs text-muted-foreground">最近同步</dt><dd>{connection.lastPolledAt ? new Date(connection.lastPolledAt).toLocaleString() : '尚未同步'}</dd></div>
                </dl>
                <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
                  <Button variant="outline" onClick={() => void requestArchiveSync(connection, true)} disabled={!connection.sdkConfigured || action === `verify-archive-${connection.id}`}><ShieldCheck className="size-4" />验证</Button>
                  <Button variant="outline" onClick={() => void requestArchiveSync(connection, false)} disabled={!connection.sdkConfigured || connection.status !== 'active' || action === `sync-archive-${connection.id}`}><RefreshCw className="size-4" />立即同步</Button>
                  <Button variant="ghost" size="icon" onClick={() => void removeArchive(connection)} disabled={action === `delete-archive-${connection.id}`} aria-label="删除会话存档连接"><Trash2 className="size-4" /></Button>
                </div>
              </Card>
            ))}
          </div>
        ) : (
          <form onSubmit={(event) => void createArchive(event)} className="space-y-5">
            <p className="text-sm text-muted-foreground">Secret 和 RSA 私钥仅加密保存在服务端，保存后不会再次显示。创建连接不代表企业微信后台已完成开通。</p>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="连接名称" value={archiveForm.displayName} onChange={(value) => setArchiveForm({ ...archiveForm, displayName: value })} />
              <Field label="CorpID" value={archiveForm.corpId} onChange={(value) => setArchiveForm({ ...archiveForm, corpId: value })} />
              <Field label="会话存档 Secret" value={archiveForm.archiveSecret} secret onChange={(value) => setArchiveForm({ ...archiveForm, archiveSecret: value })} />
              <Field label="公钥版本" value={String(archiveForm.publicKeyVersion)} onChange={(value) => setArchiveForm({ ...archiveForm, publicKeyVersion: Number(value) || 1 })} />
              <Field label="当前成员 UserID" value={archiveForm.wecomUserId} onChange={(value) => setArchiveForm({ ...archiveForm, wecomUserId: value })} />
              <Field label="成员显示名称" value={archiveForm.memberDisplayName} onChange={(value) => setArchiveForm({ ...archiveForm, memberDisplayName: value })} />
            </div>
            <div className="space-y-2"><Label>RSA 私钥 PEM</Label><Textarea required rows={8} value={archiveForm.privateKeyPem} autoComplete="off" spellCheck={false} onChange={(event) => setArchiveForm({ ...archiveForm, privateKeyPem: event.target.value })} /></div>
            <Button type="submit" disabled={action === 'create-archive'}>{action === 'create-archive' && <Loader2 className="size-4 animate-spin" />}创建存档连接</Button>
          </form>
        )}
      </section>
    </main>
  )
}

function Field({ label, value, onChange, secret = false }: { label: string; value: string; onChange: (value: string) => void; secret?: boolean }) {
  return <div className="space-y-2"><Label>{label}</Label><Input required type={secret ? 'password' : 'text'} value={value} autoComplete="off" onChange={(event) => onChange(event.target.value)} /></div>
}
