'use client'

import { ArrowLeft, Check, Loader2, Pencil, Plus, Sparkles, Trash2 } from 'lucide-react'
import Link from 'next/link'
import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { createJobSearchProfile, deleteJobSearchProfile, fetchJobSearchProfiles, parseJobSearchProfile, updateJobSearchProfile } from '@/lib/api'
import type { JobEmploymentType, JobSearchProfile, JobSearchProfileInput, JobSeniority, JobWorkMode, SalaryPeriod } from '@/lib/types'

const emptyProfile: JobSearchProfileInput = {
  name: '', isDefault: false, enabled: true, targetRoles: [], excludedRoles: [], targetIndustries: [],
  preferredSeniority: [], candidateSkills: [], yearsExperience: null, educationLevel: null,
  englishLevel: null, otherLanguages: [], preferredCountries: [], preferredCities: [],
  preferredTimezones: [], workModes: [], employmentTypes: [], minimumSalary: null,
  salaryCurrency: null, salaryPeriod: null, visaSponsorshipRequired: null,
  relocationAcceptable: null, requiredKeywords: [], preferredKeywords: [], excludedKeywords: [],
  requireSalaryDisclosed: false, minimumMatchScore: 0, notificationEnabled: false,
}
const split = (value: string) => value.split(/[,，\n]/).map((item) => item.trim()).filter(Boolean)
const join = (value: string[]) => value.join('，')

export default function JobSearchSettingsPage() {
  const [profiles, setProfiles] = useState<JobSearchProfile[]>([])
  const [editingId, setEditingId] = useState<string | null>(null)
  const [draft, setDraft] = useState<JobSearchProfileInput>({ ...emptyProfile })
  const [naturalText, setNaturalText] = useState('')
  const [parsedPreview, setParsedPreview] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [parsing, setParsing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function reload() { setLoading(true); try { setProfiles(await fetchJobSearchProfiles()) } catch (cause) { setError(cause instanceof Error ? cause.message : '加载求职档案失败') } finally { setLoading(false) } }
  useEffect(() => { void reload() }, [])
  function edit(profile?: JobSearchProfile) { setEditingId(profile?.id || null); setDraft(profile ? { ...profile } : { ...emptyProfile, isDefault: profiles.length === 0 }); setParsedPreview(false); setConfirmed(false); setError(null) }
  async function parse() { if (naturalText.trim().length < 5) return; setParsing(true); setError(null); try { const preview = await parseJobSearchProfile(naturalText.trim()); const { requiresConfirmation, ...input } = preview; void requiresConfirmation; setDraft(input); setEditingId(null); setParsedPreview(true); setConfirmed(false) } catch (cause) { setError(cause instanceof Error ? cause.message : 'Pi Agent 解析失败') } finally { setParsing(false) } }
  async function save() { if (!draft.name.trim() || (parsedPreview && !confirmed)) return; setSaving(true); setError(null); try { if (editingId) await updateJobSearchProfile(editingId, draft); else await createJobSearchProfile(draft); setDraft({ ...emptyProfile }); setEditingId(null); setParsedPreview(false); setConfirmed(false); await reload() } catch (cause) { setError(cause instanceof Error ? cause.message : '保存求职档案失败') } finally { setSaving(false) } }
  async function remove(id: string) { setError(null); try { await deleteJobSearchProfile(id); if (editingId === id) edit(); await reload() } catch (cause) { setError(cause instanceof Error ? cause.message : '删除求职档案失败') } }
  const set = <K extends keyof JobSearchProfileInput>(key: K, value: JobSearchProfileInput[K]) => setDraft((current) => ({ ...current, [key]: value }))

  return <div className="mx-auto w-full max-w-5xl px-4 py-6 md:px-8" data-testid="job-search-settings">
    <div className="mb-5 flex items-center gap-2"><Button variant="ghost" size="icon" aria-label="返回设置" nativeButton={false} render={<Link href="/settings" />}><ArrowLeft className="size-4" /></Button><div><h1 className="text-xl font-semibold">求职档案</h1><p className="text-sm text-muted-foreground">职位匹配只使用你明确填写的职业偏好</p></div><Button className="ml-auto gap-2" onClick={() => edit()}><Plus className="size-4" />新建档案</Button></div>
    {error && <p role="alert" className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
    <section className="mb-8"><h2 className="mb-3 text-sm font-semibold text-muted-foreground">现有档案</h2>{loading ? <Loader2 className="size-5 animate-spin text-muted-foreground" /> : profiles.length ? <div className="grid gap-3 sm:grid-cols-2">{profiles.map((profile) => <Card key={profile.id} className="rounded-lg p-4"><div className="flex items-start gap-2"><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">{profile.name}</h3>{profile.isDefault && <Badge>默认</Badge>}{!profile.enabled && <Badge variant="outline">已停用</Badge>}</div><p className="mt-2 text-sm text-muted-foreground">{profile.targetRoles.join('、') || '未设置目标岗位'}</p><div className="mt-3 flex flex-wrap gap-1">{profile.candidateSkills.slice(0, 5).map((skill) => <Badge key={skill} variant="secondary">{skill}</Badge>)}</div></div><Button variant="ghost" size="icon" aria-label={`编辑${profile.name}`} onClick={() => edit(profile)}><Pencil className="size-4" /></Button><Button variant="ghost" size="icon" aria-label={`删除${profile.name}`} onClick={() => remove(profile.id)}><Trash2 className="size-4 text-destructive" /></Button></div></Card>)}</div> : <p className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">尚未创建求职档案</p>}</section>
    <section className="mb-6 border-y py-6"><div className="flex items-center gap-2"><Sparkles className="size-4 text-primary" /><h2 className="font-semibold">自然语言生成预览</h2></div><div className="mt-3 flex flex-col gap-2 sm:flex-row"><Textarea value={naturalText} onChange={(event) => setNaturalText(event.target.value)} className="min-h-20 flex-1" placeholder="例如：远程 Python 后端，欧洲时区，年薪至少 8 万美元，需要签证支持" /><Button onClick={parse} disabled={parsing || naturalText.trim().length < 5} className="gap-2 sm:self-end">{parsing ? <Loader2 className="size-4 animate-spin" /> : <Sparkles className="size-4" />}生成预览</Button></div></section>
    <Card className="rounded-lg p-5"><div className="flex flex-wrap items-center justify-between gap-2"><h2 className="font-semibold">{editingId ? '编辑档案' : parsedPreview ? '确认解析结果' : '新建档案'}</h2>{editingId && <Button variant="ghost" size="sm" onClick={() => edit()}>取消编辑</Button>}</div>
      <div className="mt-5 grid gap-4 sm:grid-cols-2">
        <Field label="档案名称"><Input value={draft.name} onChange={(event) => set('name', event.target.value)} placeholder="例如 远程后端" /></Field>
        <Field label="目标岗位"><Input value={join(draft.targetRoles)} onChange={(event) => set('targetRoles', split(event.target.value))} placeholder="Python 后端，平台工程师" /></Field>
        <Field label="技能"><Input value={join(draft.candidateSkills)} onChange={(event) => set('candidateSkills', split(event.target.value))} placeholder="Python，FastAPI，PostgreSQL" /></Field>
        <Field label="排除岗位"><Input value={join(draft.excludedRoles)} onChange={(event) => set('excludedRoles', split(event.target.value))} /></Field>
        <Field label="偏好国家"><Input value={join(draft.preferredCountries)} onChange={(event) => set('preferredCountries', split(event.target.value))} placeholder="DE，NL" /></Field>
        <Field label="偏好城市"><Input value={join(draft.preferredCities)} onChange={(event) => set('preferredCities', split(event.target.value))} /></Field>
        <Field label="偏好时区"><Input value={join(draft.preferredTimezones)} onChange={(event) => set('preferredTimezones', split(event.target.value))} placeholder="Europe/Berlin" /></Field>
        <Field label="工作模式"><Select value={draft.workModes[0] || 'any'} onValueChange={(value) => { const next = String(value); set('workModes', next === 'any' ? [] : [next as JobWorkMode]) }}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="any">不限</SelectItem><SelectItem value="remote">远程</SelectItem><SelectItem value="hybrid">混合</SelectItem><SelectItem value="on_site">现场</SelectItem><SelectItem value="flexible">灵活</SelectItem></SelectContent></Select></Field>
        <Field label="雇佣类型"><Select value={draft.employmentTypes[0] || 'any'} onValueChange={(value) => { const next = String(value); set('employmentTypes', next === 'any' ? [] : [next as JobEmploymentType]) }}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="any">不限</SelectItem><SelectItem value="full_time">全职</SelectItem><SelectItem value="part_time">兼职</SelectItem><SelectItem value="contract">合同</SelectItem><SelectItem value="internship">实习</SelectItem><SelectItem value="freelance">自由职业</SelectItem></SelectContent></Select></Field>
        <Field label="偏好资历"><Select value={draft.preferredSeniority[0] || 'any'} onValueChange={(value) => { const next = String(value); set('preferredSeniority', next === 'any' ? [] : [next as JobSeniority]) }}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="any">不限</SelectItem><SelectItem value="junior">初级</SelectItem><SelectItem value="mid">中级</SelectItem><SelectItem value="senior">高级</SelectItem><SelectItem value="lead">负责人</SelectItem><SelectItem value="manager">经理</SelectItem></SelectContent></Select></Field>
        <Field label="经验年限"><Input type="number" min={0} value={draft.yearsExperience ?? ''} onChange={(event) => set('yearsExperience', event.target.value ? Number(event.target.value) : null)} /></Field>
        <Field label="最低薪资"><div className="grid grid-cols-[1fr_76px_110px] gap-2"><Input type="number" min={0} value={draft.minimumSalary ?? ''} onChange={(event) => set('minimumSalary', event.target.value ? Number(event.target.value) : null)} /><Input maxLength={3} value={draft.salaryCurrency || ''} onChange={(event) => set('salaryCurrency', event.target.value.toUpperCase() || null)} placeholder="USD" /><Select value={draft.salaryPeriod || 'none'} onValueChange={(value) => set('salaryPeriod', value === 'none' ? null : value as SalaryPeriod)}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="none">周期</SelectItem><SelectItem value="monthly">月薪</SelectItem><SelectItem value="annual">年薪</SelectItem><SelectItem value="hourly">时薪</SelectItem></SelectContent></Select></div></Field>
        <Field label="学历"><Input value={draft.educationLevel || ''} onChange={(event) => set('educationLevel', event.target.value || null)} /></Field>
        <Field label="英语要求"><Input value={draft.englishLevel || ''} onChange={(event) => set('englishLevel', event.target.value || null)} /></Field>
        <Field label="签证支持"><Select value={draft.visaSponsorshipRequired === null ? 'unknown' : String(draft.visaSponsorshipRequired)} onValueChange={(value) => set('visaSponsorshipRequired', value === 'unknown' ? null : value === 'true')}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="unknown">不作为必要条件</SelectItem><SelectItem value="true">必须支持</SelectItem><SelectItem value="false">不需要</SelectItem></SelectContent></Select></Field>
        <Field label="最低匹配分"><Input type="number" min={0} max={100} value={draft.minimumMatchScore} onChange={(event) => set('minimumMatchScore', Number(event.target.value))} /></Field>
        <Field label="偏好关键词"><Input value={join(draft.preferredKeywords)} onChange={(event) => set('preferredKeywords', split(event.target.value))} /></Field>
        <Field label="排除关键词"><Input value={join(draft.excludedKeywords)} onChange={(event) => set('excludedKeywords', split(event.target.value))} /></Field>
      </div>
      <div className="mt-5 flex flex-wrap gap-5 border-t pt-4"><Toggle label="设为默认" checked={draft.isDefault} onChange={(value) => set('isDefault', value)} /><Toggle label="启用档案" checked={draft.enabled} onChange={(value) => set('enabled', value)} /><Toggle label="要求公开薪资" checked={draft.requireSalaryDisclosed} onChange={(value) => set('requireSalaryDisclosed', value)} /><Toggle label="匹配通知" checked={draft.notificationEnabled} onChange={(value) => set('notificationEnabled', value)} /></div>
      {parsedPreview && <label className="mt-5 flex items-start gap-2 rounded-md border bg-muted/30 p-3 text-sm"><Checkbox checked={confirmed} onCheckedChange={(value) => setConfirmed(value === true)} /><span>我已确认目标岗位、技能、地点、薪资、签证需求和排除项。保存后才会参与职位匹配。</span></label>}
      <div className="mt-5 flex justify-end"><Button onClick={save} disabled={saving || !draft.name.trim() || (parsedPreview && !confirmed)} className="gap-2">{saving ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}{editingId ? '保存修改' : '保存档案'}</Button></div>
    </Card>
  </div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-1.5"><Label>{label}</Label>{children}</div> }
function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) { return <div className="flex items-center gap-2"><Switch checked={checked} onCheckedChange={onChange} /><span className="text-sm">{label}</span></div> }
