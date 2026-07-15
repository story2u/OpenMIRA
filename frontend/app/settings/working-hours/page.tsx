'use client'

import { ArrowLeft, Bot, Globe } from 'lucide-react'
import Link from 'next/link'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { fetchSettings, updateWorkSchedule } from '@/lib/api'
import type { WorkScheduleSlot } from '@/lib/types'
import { cn } from '@/lib/utils'

const days = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']
// 时段：8:00 - 22:00，每格 1 小时
const hours = Array.from({ length: 14 }, (_, i) => i + 8)

function emptySchedule(): boolean[][] {
  return days.map(() => hours.map(() => false))
}

const HOUR_FMT = (h: number) => `${String(h).padStart(2, '0')}:00`

// 后端 slots（每天可多段）→ 14×7 布尔网格。
function slotsToGrid(slots: WorkScheduleSlot[]): boolean[][] {
  const grid = emptySchedule()
  for (const slot of slots) {
    const dayIndex = slot.weekday - 1
    if (dayIndex < 0 || dayIndex > 6) continue
    const start = parseInt(slot.start.slice(0, 2), 10)
    const end = parseInt(slot.end.slice(0, 2), 10)
    for (let h = start; h < end; h++) {
      const hourIndex = h - 8
      if (hourIndex >= 0 && hourIndex < hours.length) grid[dayIndex][hourIndex] = true
    }
  }
  return grid
}

// 网格 → slots：把每天连续选中的小时合并成一段。
function gridToSlots(grid: boolean[][]): WorkScheduleSlot[] {
  const slots: WorkScheduleSlot[] = []
  grid.forEach((row, dayIndex) => {
    let runStart: number | null = null
    for (let i = 0; i <= row.length; i++) {
      const on = i < row.length && row[i]
      if (on && runStart === null) {
        runStart = i
      } else if (!on && runStart !== null) {
        slots.push({
          weekday: dayIndex + 1,
          start: HOUR_FMT(runStart + 8),
          end: HOUR_FMT(i + 8),
        })
        runStart = null
      }
    }
  })
  return slots
}

export default function WorkingHoursPage() {
  const [schedule, setSchedule] = useState<boolean[][]>(emptySchedule)
  const [timezone, setTimezone] = useState('Asia/Shanghai')
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [autoReplyOutsideHours, setAutoReplyOutsideHours] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const dragRef = useRef<{ active: boolean; value: boolean }>({ active: false, value: true })

  // 加载真实工作时间（与 iOS/Android 同源）。默认（isDefault）表示后端回退值。
  useEffect(() => {
    let active = true
    fetchSettings()
      .then((bundle) => {
        if (!active) return
        setSchedule(slotsToGrid(bundle.workSchedule.slots))
        setTimezone(bundle.workSchedule.timezone)
        setAutoReplyOutsideHours(bundle.workSchedule.autoReplyOutsideHours)
        setLoaded(true)
      })
      .catch((e) => active && setError(e instanceof Error ? e.message : '加载工作时间失败'))
    return () => {
      active = false
    }
  }, [])

  const setCell = useCallback((day: number, hour: number, value: boolean) => {
    setSchedule((prev) => {
      if (prev[day][hour] === value) return prev
      const next = prev.map((row) => [...row])
      next[day][hour] = value
      return next
    })
  }, [])

  const handlePointerDown = (day: number, hour: number) => {
    const value = !schedule[day][hour]
    dragRef.current = { active: true, value }
    setCell(day, hour, value)
  }

  const handlePointerEnter = (day: number, hour: number) => {
    if (dragRef.current.active) {
      setCell(day, hour, dragRef.current.value)
    }
  }

  const save = async () => {
    setSaving(true)
    setError(null)
    try {
      const saved = await updateWorkSchedule({
        timezone,
        slots: gridToSlots(schedule),
        autoReplyOutsideHours,
      })
      setSchedule(slotsToGrid(saved.slots))
      setTimezone(saved.timezone)
      setAutoReplyOutsideHours(saved.autoReplyOutsideHours)
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const totalHours = schedule.flat().filter(Boolean).length

  return (
    <div
      className="mx-auto w-full max-w-3xl px-4 py-6 md:px-8"
      onPointerUp={() => {
        dragRef.current.active = false
      }}
      onPointerLeave={() => {
        dragRef.current.active = false
      }}
    >
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
          <h1 className="text-lg font-semibold tracking-tight md:text-xl">工作时间设置</h1>
          <p className="text-xs text-muted-foreground">
            选中的时段为人工审核模式；非工作时间可单独启用安全接待
          </p>
        </div>
      </div>

      <div className="flex flex-col gap-5">
        <Card className="gap-4 rounded-xl p-4 shadow-sm md:p-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm font-medium">一周时间表</p>
            <p className="text-xs text-muted-foreground">
              点击或拖动选择 · 已选 <span className="font-semibold text-primary">{totalHours}</span> 小时/周
            </p>
          </div>

          <div className="overflow-x-auto">
            <div className="min-w-[560px] select-none">
              {/* 小时刻度 */}
              <div className="mb-1 flex pl-12">
                {hours.map((h) => (
                  <span key={h} className="flex-1 text-center text-[10px] tabular-nums text-muted-foreground">
                    {h}
                  </span>
                ))}
              </div>
              <div className="flex flex-col gap-1">
                {days.map((day, dayIndex) => (
                  <div key={day} className="flex items-center gap-2">
                    <span className="w-10 shrink-0 text-xs text-muted-foreground">{day}</span>
                    <div className="flex flex-1 gap-0.5">
                      {hours.map((hour, hourIndex) => {
                        const selected = schedule[dayIndex][hourIndex]
                        return (
                          <button
                            key={hour}
                            type="button"
                            onPointerDown={(e) => {
                              e.preventDefault()
                              handlePointerDown(dayIndex, hourIndex)
                            }}
                            onPointerEnter={() => handlePointerEnter(dayIndex, hourIndex)}
                            className={cn(
                              'h-7 flex-1 rounded-sm transition-colors',
                              selected ? 'bg-primary hover:bg-primary/85' : 'bg-muted hover:bg-accent',
                            )}
                            aria-pressed={selected}
                            aria-label={`${day} ${hour}:00 至 ${hour + 1}:00 ${selected ? '工作时间' : '非工作时间'}`}
                          />
                        )
                      })}
                    </div>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex items-center gap-4 pl-12 text-[11px] text-muted-foreground">
                <span className="inline-flex items-center gap-1.5">
                  <span className="size-2.5 rounded-sm bg-primary" aria-hidden="true" />
                  工作时间 · 人工审核
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <span className="size-2.5 rounded-sm bg-muted ring-1 ring-border" aria-hidden="true" />
                  非工作时间 · 待人工或按授权安全接待
                </span>
              </div>
            </div>
          </div>
        </Card>

        <Card className="gap-3 rounded-xl p-4 shadow-sm md:p-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex min-w-0 gap-3">
              <Bot className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div>
                <Label htmlFor="after-hours-auto-reply" className="text-sm font-medium">
                  非工作时间 AI 安全接待
                </Label>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  仅对已单独授权的 Telegram Business 私聊生效。Agent 完成风险分析后，才可能发送一条确认需求的简短回复。
                </p>
              </div>
            </div>
            <Switch
              id="after-hours-auto-reply"
              checked={autoReplyOutsideHours}
              onCheckedChange={setAutoReplyOutsideHours}
              aria-label="启用非工作时间 AI 安全接待"
            />
          </div>
        </Card>

        <Card className="gap-3 rounded-xl p-4 shadow-sm md:p-5">
          <div className="flex items-center gap-2">
            <Globe className="size-4 text-muted-foreground" />
            <Label className="text-sm font-medium">时区</Label>
          </div>
          <Select
            items={{
              'Asia/Shanghai': '(UTC+8) 中国标准时间 · 上海',
              'Asia/Hong_Kong': '(UTC+8) 香港时间',
              'Asia/Singapore': '(UTC+8) 新加坡时间',
              'Asia/Tokyo': '(UTC+9) 日本标准时间 · 东京',
              'Europe/London': '(UTC+0) 格林威治时间 · 伦敦',
              'America/New_York': '(UTC-5) 美国东部时间 · 纽约',
            }}
            value={timezone}
            onValueChange={(value) => {
              if (value) setTimezone(value)
            }}
          >
            <SelectTrigger className="max-w-72" aria-label="选择时区">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="Asia/Shanghai">(UTC+8) 中国标准时间 · 上海</SelectItem>
              <SelectItem value="Asia/Hong_Kong">(UTC+8) 香港时间</SelectItem>
              <SelectItem value="Asia/Singapore">(UTC+8) 新加坡时间</SelectItem>
              <SelectItem value="Asia/Tokyo">(UTC+9) 日本标准时间 · 东京</SelectItem>
              <SelectItem value="Europe/London">(UTC+0) 格林威治时间 · 伦敦</SelectItem>
              <SelectItem value="America/New_York">(UTC-5) 美国东部时间 · 纽约</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">日夜模式切换将依据所选时区判断</p>
        </Card>

        <div className="flex items-center justify-end gap-3">
          {error && <span className="text-sm text-destructive">{error}</span>}
          <Button onClick={save} disabled={!loaded || saving}>
            {saving ? '保存中…' : '保存设置'}
          </Button>
        </div>
      </div>
    </div>
  )
}
