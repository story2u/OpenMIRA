'use client'

import { useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { useAppStore } from '@/lib/app-store'
import { templateCategories } from '@/lib/mock-data'
import type { ReplyTemplate } from '@/lib/types'

const variables = ['{{联系人姓名}}', '{{公司名称}}', '{{团队规模}}', '{{工作时间}}']

interface TemplateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  template?: ReplyTemplate | null
}

export function TemplateDialog({ open, onOpenChange, template }: TemplateDialogProps) {
  const { addTemplate, updateTemplate } = useAppStore()
  const [title, setTitle] = useState('')
  const [category, setCategory] = useState('开场白')
  const [content, setContent] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (open) {
      setTitle(template?.title ?? '')
      setCategory(template?.category ?? '开场白')
      setContent(template?.content ?? '')
    }
  }, [open, template])

  const insertVariable = (variable: string) => {
    const el = textareaRef.current
    if (!el) {
      setContent((prev) => prev + variable)
      return
    }
    const start = el.selectionStart ?? content.length
    const end = el.selectionEnd ?? content.length
    const next = content.slice(0, start) + variable + content.slice(end)
    setContent(next)
    requestAnimationFrame(() => {
      el.focus()
      const pos = start + variable.length
      el.setSelectionRange(pos, pos)
    })
  }

  const handleSave = () => {
    if (!title.trim() || !content.trim()) return
    if (template) {
      updateTemplate({ ...template, title: title.trim(), category, content: content.trim() })
    } else {
      addTemplate({ title: title.trim(), category, content: content.trim() })
    }
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{template ? '编辑模板' : '新建模板'}</DialogTitle>
          <DialogDescription>
            模板内容支持插入变量占位符，发送时自动替换为真实信息
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="template-title">模板标题</Label>
            <Input
              id="template-title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="例如：首次咨询欢迎语"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>分类</Label>
            <Select
              value={category}
              onValueChange={(value) => {
                if (value) setCategory(value)
              }}
            >
              <SelectTrigger aria-label="模板分类">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {templateCategories
                  .filter((c) => c !== '全部')
                  .map((c) => (
                    <SelectItem key={c} value={c}>
                      {c}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="template-content">模板内容</Label>
            <Textarea
              id="template-content"
              ref={textareaRef}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder="输入模板内容…"
              className="min-h-28"
            />
            <div className="flex flex-wrap gap-1.5 pt-1">
              {variables.map((variable) => (
                <button
                  key={variable}
                  type="button"
                  onClick={() => insertVariable(variable)}
                  className="rounded-md border border-dashed border-primary/40 bg-accent px-2 py-0.5 font-mono text-[11px] text-accent-foreground transition-colors hover:bg-primary/15"
                >
                  {variable}
                </button>
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={handleSave} disabled={!title.trim() || !content.trim()}>
            保存模板
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
