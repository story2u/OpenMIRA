"use client";

import { Upload } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useImportChannelAccountsMutation } from "../../hooks/use-channel-accounts";
import { ACCOUNT_CSV_ACCEPT } from "../../lib/adminAccounts.js";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";
import { Label } from "../ui/label";

export function CsvImportCard() {
  const [file, setFile] = useState<File | null>(null);
  const [inputKey, setInputKey] = useState(0);
  const mutation = useImportChannelAccountsMutation();

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file) {
      toast.error("请选择 CSV 文件");
      return;
    }
    await mutation.mutateAsync(file);
    setFile(null);
    setInputKey((value) => value + 1);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>CSV 导入</CardTitle>
        <CardDescription>批量导入通道账号，字段名保持后端接口约定。</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end" onSubmit={submit}>
          <div className="grid gap-2">
            <Label htmlFor="account-csv">CSV 文件</Label>
            <Input
              id="account-csv"
              key={inputKey}
              type="file"
              accept={ACCOUNT_CSV_ACCEPT}
              onChange={(event) => setFile(event.target.files?.[0] || null)}
            />
            <p className="text-xs text-muted-foreground">{file ? `已选择：${file.name}` : "请选择 .csv 文件，导入成功后账号列表会自动刷新。"}</p>
          </div>
          <Button type="submit" variant="outline" loading={mutation.isPending} disabled={!file}>
            <Upload className="h-4 w-4" aria-hidden="true" />
            导入
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
