'use client'

import { AlertTriangle, ArrowLeft, Building2, CheckCircle2, CircleHelp, ExternalLink, Flag, Loader2, MapPin, Radio, WalletCards, XCircle } from 'lucide-react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { fetchJob, submitJobFeedback } from '@/lib/api'
import type { JobFeedbackType, JobOpportunityDetail } from '@/lib/types'

const feedback: Array<[JobFeedbackType, string]> = [['relevant', '适合我'], ['not_relevant', '不适合'], ['not_a_job', '不是招聘'], ['expired', '已过期'], ['duplicate', '重复'], ['scam', '疑似诈骗'], ['wrong_extraction', '提取错误']]

function value(value: string | number | null | undefined, fallback = '招聘信息未说明') { return value === null || value === undefined || value === '' ? fallback : String(value) }

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [job, setJob] = useState<JobOpportunityDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState<JobFeedbackType | null>(null)
  const [sent, setSent] = useState<JobFeedbackType | null>(null)
  useEffect(() => { fetchJob(id).then(setJob).catch((cause) => setError(cause instanceof Error ? cause.message : '加载职位失败')) }, [id])
  async function report(type: JobFeedbackType) { setSending(type); try { await submitJobFeedback(id, type); setSent(type) } catch (cause) { setError(cause instanceof Error ? cause.message : '反馈失败') } finally { setSending(null) } }
  if (error && !job) return <div className="mx-auto max-w-4xl px-4 py-16 text-center text-sm text-destructive">{error}<div className="mt-4"><Button variant="outline" nativeButton={false} render={<Link href="/jobs" />}>返回工作机会</Button></div></div>
  if (!job) return <div className="grid min-h-80 place-items-center text-muted-foreground"><Loader2 className="size-5 animate-spin" /></div>
  return <div className="mx-auto w-full max-w-5xl px-4 py-5 md:px-8" data-testid="job-detail">
    <div className="mb-4 flex items-center gap-2"><Button variant="ghost" size="icon" aria-label="返回工作机会" nativeButton={false} render={<Link href="/jobs" />}><ArrowLeft className="size-4" /></Button><span className="text-sm font-semibold">工作机会详情</span></div>
    {error && <p role="alert" className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
    <section className="mb-6 border-b pb-6"><div className="flex flex-wrap items-start justify-between gap-4"><div><div className="flex items-center gap-2"><Building2 className="size-5 text-muted-foreground" /><h1 className="text-2xl font-semibold">{job.jobTitle}</h1></div><p className="mt-2 text-muted-foreground">{job.companyName || '公司未说明'}{job.department ? ` · ${job.department}` : ''}</p></div>{job.applicationUrl && <Button nativeButton={false} render={<a href={job.applicationUrl} target="_blank" rel="noreferrer" />} className="gap-2">前往投递<ExternalLink className="size-4" /></Button>}</div><div className="mt-5 flex flex-wrap gap-x-5 gap-y-2 text-sm text-muted-foreground"><span className="inline-flex gap-1.5"><MapPin className="size-4" />{value(job.locationText)}</span><span className="inline-flex gap-1.5"><WalletCards className="size-4" />{value(job.salaryRaw, '薪资未公开')}</span><span>{job.workMode}</span><span>{job.employmentType}</span><span>{new Date(job.postedAt).toLocaleString('zh-CN')}</span></div></section>
    <div className="grid gap-5 lg:grid-cols-[1fr_320px]">
      <div className="space-y-5">
        <Card className="rounded-lg p-5"><h2 className="font-semibold">匹配分析</h2>{job.match ? <><div className="mt-4 flex items-end gap-3"><strong className="text-3xl text-primary">{job.match.matchScore}</strong><span className="pb-1 text-sm text-muted-foreground">/ 100</span></div><Progress value={job.match.matchScore} className="mt-2" /><div className="mt-5 grid gap-4 md:grid-cols-3"><Reason title="符合" icon={CheckCircle2} color="text-success" items={job.match.matchedReasons} /><Reason title="不符合" icon={XCircle} color="text-destructive" items={job.match.mismatchReasons} /><Reason title="信息缺失" icon={CircleHelp} color="text-warning" items={job.match.unknownConstraints} /></div></> : <p className="mt-3 text-sm text-muted-foreground">创建求职档案后会生成确定性匹配分和原因。</p>}</Card>
        <Card className="rounded-lg p-5"><h2 className="font-semibold">核心要求</h2><p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-muted-foreground">{value(job.requirementsSummary)}</p><dl className="mt-5 grid gap-4 text-sm sm:grid-cols-2"><Fact label="经验" value={job.minimumYearsExperience === null ? null : `${job.minimumYearsExperience} 年以上`} /><Fact label="学历" value={job.degreeLevel} /><Fact label="英语" value={job.englishLevel} /><Fact label="签证支持" value={job.visaSponsorship === null ? null : job.visaSponsorship ? '明确支持' : '明确不支持'} /><Fact label="工作许可" value={job.workAuthorizationText} /><Fact label="搬迁支持" value={job.relocationSupport === null ? null : job.relocationSupport ? '支持' : '不支持'} /></dl><div className="mt-4 flex flex-wrap gap-1.5">{job.requiredSkills.map((skill) => <Badge key={skill} variant="secondary">{skill}</Badge>)}</div></Card>
        {job.ageRequirementPresent && <div className="rounded-lg border border-warning/40 bg-warning/10 p-4 text-sm"><div className="flex gap-2 font-medium text-warning"><AlertTriangle className="size-4" />招聘原文包含年龄限制</div><p className="mt-2 text-muted-foreground">{job.ageRequirementText}。该条件可能涉及就业歧视，系统不会将用户年龄用于推荐计算。</p></div>}
        {job.complianceFlags.length > 0 && <Card className="rounded-lg border-warning/30 p-4"><h2 className="flex items-center gap-2 font-semibold"><Flag className="size-4 text-warning" />合规提示</h2><ul className="mt-2 space-y-1 text-sm text-muted-foreground">{job.complianceFlags.map((item) => <li key={item}>· {item}</li>)}</ul></Card>}
        <Card className="rounded-lg p-5"><h2 className="font-semibold">原始信息与证据</h2><p className="mt-3 whitespace-pre-wrap rounded-md bg-muted/50 p-3 text-sm leading-6">{job.rawExcerpt}</p><div className="mt-4 space-y-2 text-sm">{Object.entries(job.fieldEvidence).map(([field, evidence]) => <div key={field} className="grid gap-1 sm:grid-cols-[160px_1fr]"><span className="text-muted-foreground">{field}</span><span>{evidence}</span></div>)}</div></Card>
      </div>
      <aside className="space-y-4"><Card className="rounded-lg p-4"><h2 className="font-semibold">来源信息</h2><div className="mt-3 space-y-3">{job.sources.map((source) => <div key={source.id} className="border-b pb-3 last:border-0 last:pb-0"><p className="flex items-center gap-1.5 text-sm font-medium"><Radio className="size-3.5" />{source.channel === 'telegram' ? 'Telegram' : '企业微信'}</p><p className="mt-1 text-xs text-muted-foreground">{source.chatName || '私聊来源'} · {source.authorName || '发布者未说明'}</p>{source.sourceMessageUrl ? <a className="mt-2 inline-flex items-center gap-1 text-xs text-primary" href={source.sourceMessageUrl} target="_blank" rel="noreferrer">查看原始消息<ExternalLink className="size-3" /></a> : <p className="mt-2 text-xs text-muted-foreground">该消息来自私有群组，只能在原平台查看。</p>}</div>)}</div></Card><Card className="rounded-lg p-4"><h2 className="font-semibold">信息缺失</h2><div className="mt-3 flex flex-wrap gap-1.5">{job.missingFields.length ? job.missingFields.map((field) => <Badge key={field} variant="outline">{field}</Badge>) : <span className="text-sm text-muted-foreground">未标记缺失字段</span>}</div></Card><Card className="rounded-lg p-4"><h2 className="font-semibold">反馈</h2><div className="mt-3 flex flex-wrap gap-2">{feedback.map(([type, label]) => <Button key={type} size="sm" variant={sent === type ? 'secondary' : 'outline'} disabled={Boolean(sending)} onClick={() => report(type)}>{sending === type && <Loader2 className="size-3 animate-spin" />}{label}</Button>)}</div></Card></aside>
    </div>
  </div>
}

function Fact({ label, value: content }: { label: string; value: string | null | undefined }) { return <div><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1">{value(content)}</dd></div> }
function Reason({ title, icon: Icon, color, items }: { title: string; icon: typeof CheckCircle2; color: string; items: string[] }) { return <div><h3 className={`flex items-center gap-1.5 text-sm font-medium ${color}`}><Icon className="size-4" />{title}</h3><ul className="mt-2 space-y-1 text-xs text-muted-foreground">{items.length ? items.map((item) => <li key={item}>· {item}</li>) : <li>无</li>}</ul></div> }
