"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { Link2Off, Send } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { useAssignAccountMutation, useUnassignAccountMutation } from "../../hooks/use-channel-accounts";
import {
  accountAssignmentSchema,
  defaultAccountAssignmentValues,
  type AccountAssignmentFormValues,
} from "../../lib/schemas/channel-account";
import type { ChannelAccount, MessageLoad } from "../../types/channel-account";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "../ui/form";
import { Input } from "../ui/input";

export function AccountAssignCard({
  accounts,
  assignees,
  selectedAccount,
  onSelectedAccountChange,
}: {
  accounts: ChannelAccount[];
  assignees: MessageLoad[];
  selectedAccount: ChannelAccount | null;
  onSelectedAccountChange: (account: ChannelAccount | null) => void;
}) {
  const assignMutation = useAssignAccountMutation();
  const unassignMutation = useUnassignAccountMutation();
  const form = useForm<AccountAssignmentFormValues>({
    resolver: zodResolver(accountAssignmentSchema),
    defaultValues: defaultAccountAssignmentValues(),
  });

  useEffect(() => {
    if (!selectedAccount) return;
    form.reset({
      accountId: selectedAccount.accountId,
      assigneeId: selectedAccount.assigneeId,
      assigneeName: selectedAccount.assigneeName,
    });
  }, [form, selectedAccount]);

  async function onSubmit(values: AccountAssignmentFormValues) {
    await assignMutation.mutateAsync(values);
    form.reset(defaultAccountAssignmentValues());
    onSelectedAccountChange(null);
  }

  async function unassign() {
    const accountId = form.getValues("accountId");
    if (!accountId) {
      toast.error("请选择通道账号");
      return;
    }
    const account = accounts.find((item) => item.accountId === accountId);
    const confirmed = window.confirm(`取消分配 ${account?.accountName || accountId}？`);
    if (!confirmed) return;
    await unassignMutation.mutateAsync(accountId);
    form.reset(defaultAccountAssignmentValues());
    onSelectedAccountChange(null);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>账号分配</CardTitle>
        <CardDescription>将通道账号分配给消息端账号，或解除当前分配关系。</CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form className="grid gap-4 lg:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_minmax(180px,1fr)_auto] lg:items-end" onSubmit={form.handleSubmit(onSubmit)}>
            <FormField
              name="accountId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>分配账号</FormLabel>
                  <FormControl>
                    <select
                      className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      value={field.value}
                      onChange={(event) => {
                        field.onChange(event.target.value);
                        const account = accounts.find((item) => item.accountId === event.target.value) || null;
                        onSelectedAccountChange(account);
                        if (account) {
                          form.setValue("assigneeId", account.assigneeId);
                          form.setValue("assigneeName", account.assigneeName);
                        }
                      }}
                    >
                      <option value="">选择账号</option>
                      {accounts.map((account) => (
                        <option key={account.accountId} value={account.accountId}>
                          {account.accountName} / {account.accountId}
                        </option>
                      ))}
                    </select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              name="assigneeId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>消息端账号</FormLabel>
                  <FormControl>
                    {assignees.length > 0 ? (
                      <select
                        className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        value={field.value}
                        onChange={(event) => {
                          field.onChange(event.target.value);
                          const assignee = assignees.find((item) => item.assigneeId === event.target.value);
                          form.setValue("assigneeName", assignee?.assigneeName || "");
                        }}
                      >
                        <option value="">选择消息端账号</option>
                        {assignees.map((assignee) => (
                          <option key={assignee.assigneeId} value={assignee.assigneeId}>
                            {assignee.assigneeName} / {assignee.assigneeId}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <Input {...field} placeholder="请输入消息端账号" />
                    )}
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              name="assigneeName"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>账号名称</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="assignee_name" />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex flex-wrap gap-2">
              <Button type="submit" loading={assignMutation.isPending}>
                <Send className="h-4 w-4" aria-hidden="true" />
                分配
              </Button>
              <Button type="button" variant="destructive-outline" loading={unassignMutation.isPending} onClick={() => void unassign()}>
                <Link2Off className="h-4 w-4" aria-hidden="true" />
                取消分配
              </Button>
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}
