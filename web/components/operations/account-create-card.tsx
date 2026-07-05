"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { RotateCcw, Save } from "lucide-react";
import { useCreateChannelAccountMutation } from "../../hooks/use-channel-accounts";
import {
  channelAccountSchema,
  defaultChannelAccountValues,
  type ChannelAccountFormValues,
} from "../../lib/schemas/channel-account";
import type { ChannelAccount } from "../../types/channel-account";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "../ui/form";
import { Input } from "../ui/input";

export function AccountCreateCard({
  editingAccount,
  onClearEditing,
}: {
  editingAccount: ChannelAccount | null;
  onClearEditing: () => void;
}) {
  const mutation = useCreateChannelAccountMutation();
  const form = useForm<ChannelAccountFormValues>({
    resolver: zodResolver(channelAccountSchema),
    defaultValues: defaultChannelAccountValues(),
  });

  useEffect(() => {
    if (!editingAccount) {
      form.reset(defaultChannelAccountValues());
      return;
    }
    form.reset({
      accountId: editingAccount.accountId,
      accountName: editingAccount.accountName,
      agentId: editingAccount.agentId,
      deviceId: editingAccount.deviceId,
      channelUserId: editingAccount.channelUserId || editingAccount.weworkUserId,
      enterpriseId: editingAccount.enterpriseId,
      sopFlowId: editingAccount.sopFlowId,
      knowledgeTag: editingAccount.knowledgeTag,
      sopReplyWindowStart: editingAccount.sopReplyWindowStart,
      sopReplyWindowEnd: editingAccount.sopReplyWindowEnd,
      sopEnabled: editingAccount.sopEnabled,
      aiEnabled: editingAccount.aiEnabled,
      aiModel: editingAccount.aiModel,
      editing: true,
    });
  }, [editingAccount, form]);

  async function onSubmit(values: ChannelAccountFormValues) {
    await mutation.mutateAsync(values);
    form.reset(defaultChannelAccountValues());
    onClearEditing();
  }

  function reset() {
    form.reset(defaultChannelAccountValues());
    onClearEditing();
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{editingAccount ? "编辑通道账号" : "新增通道账号"}</CardTitle>
        <CardDescription>维护账号基础信息、通道绑定、SOP 与 AI 能力配置。</CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form className="grid gap-6" onSubmit={form.handleSubmit(onSubmit)}>
            <section className="grid gap-3">
              <h4 className="text-sm font-medium text-foreground">基础信息</h4>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <TextField name="accountId" label="账号 ID" placeholder="留空自动生成" disabled={Boolean(editingAccount)} />
                <TextField name="accountName" label="账号名称" placeholder="请输入账号名称" required />
                <TextField name="deviceId" label="设备 ID" placeholder="device_id" />
                <TextField name="agentId" label="Agent ID" placeholder="agent_id" />
              </div>
            </section>

            <section className="grid gap-3">
              <h4 className="text-sm font-medium text-foreground">通道信息</h4>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <TextField name="channelUserId" label="通道 UserID" placeholder="channel_user_id" />
                <TextField name="enterpriseId" label="企业 ID" placeholder="enterprise_id" />
                <TextField name="sopFlowId" label="SOP Flow" placeholder="sop_flow_id" />
                <TextField name="knowledgeTag" label="知识标签" placeholder="knowledge_tag" />
              </div>
            </section>

            <section className="grid gap-3">
              <h4 className="text-sm font-medium text-foreground">自动化配置</h4>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto_minmax(0,1fr)]">
                <TextField name="sopReplyWindowStart" label="SOP 开始" placeholder="09:00" />
                <TextField name="sopReplyWindowEnd" label="SOP 结束" placeholder="18:00" />
                <BooleanField name="sopEnabled" label="启用 SOP" />
                <BooleanField name="aiEnabled" label="启用 AI" />
                <TextField name="aiModel" label="AI 模型" placeholder="默认模型" />
              </div>
            </section>

            <div className="flex flex-wrap justify-end gap-2 border-t border-border pt-4">
              <Button type="button" variant="outline" onClick={reset}>
                <RotateCcw className="h-4 w-4" aria-hidden="true" />
                重置
              </Button>
              <Button type="submit" loading={mutation.isPending}>
                <Save className="h-4 w-4" aria-hidden="true" />
                {editingAccount ? "保存账号" : "新增账号"}
              </Button>
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}

function TextField({
  name,
  label,
  placeholder,
  disabled,
  required,
}: {
  name: keyof ChannelAccountFormValues;
  label: string;
  placeholder?: string;
  disabled?: boolean;
  required?: boolean;
}) {
  return (
    <FormField
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}{required ? <span className="ml-1 text-destructive">*</span> : null}</FormLabel>
          <FormControl>
            <Input {...field} value={String(field.value ?? "")} disabled={disabled} placeholder={placeholder} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function BooleanField({ name, label }: { name: keyof ChannelAccountFormValues; label: string }) {
  return (
    <FormField
      name={name}
      render={({ field }) => (
        <FormItem className="self-end">
          <FormControl>
            <label className="flex h-9 items-center gap-2 rounded-md border border-input bg-card px-3 text-sm shadow-sm">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={Boolean(field.value)}
                onChange={(event) => field.onChange(event.target.checked)}
              />
              {label}
            </label>
          </FormControl>
          <FormDescription> </FormDescription>
        </FormItem>
      )}
    />
  );
}
