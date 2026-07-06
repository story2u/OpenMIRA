"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Key, Plus, Trash2 } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";

const platformSchema = z.object({
  workspaceName: z.string().min(2, "Workspace name is too short"),
  timezone: z.string().min(1),
  defaultLanguage: z.string().min(1),
});
type PlatformForm = z.infer<typeof platformSchema>;

function PlatformSettings() {
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<PlatformForm>({
    resolver: zodResolver(platformSchema),
    defaultValues: {
      workspaceName: "Acme Growth Ops",
      timezone: "Asia/Singapore",
      defaultLanguage: "en",
    },
  });

  function onSubmit(values: PlatformForm) {
    return new Promise((r) => setTimeout(r, 600)).then(() => {
      toast.success("Platform settings saved");
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Platform</CardTitle>
        <CardDescription>General workspace configuration</CardDescription>
      </CardHeader>
      <form onSubmit={handleSubmit(onSubmit)}>
        <CardContent className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="workspaceName">Workspace name</Label>
            <Input id="workspaceName" {...register("workspaceName")} />
            {errors.workspaceName && <p className="text-xs text-destructive">{errors.workspaceName.message}</p>}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="timezone">Timezone</Label>
              <Input id="timezone" {...register("timezone")} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="defaultLanguage">Default language</Label>
              <Input id="defaultLanguage" {...register("defaultLanguage")} />
            </div>
          </div>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" size="sm" disabled={isSubmitting}>
            {isSubmitting ? "Saving…" : "Save changes"}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

function SecuritySettings() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Security</CardTitle>
        <CardDescription>Access control and authentication policy</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {[
          { label: "Require SSO for all operators", desc: "Disable email/password login for the workspace", defaultChecked: true },
          { label: "Enforce IP allowlist", desc: "Restrict console access to approved office and VPN ranges", defaultChecked: false },
          { label: "Require approval for manual-send channels", desc: "Messages sent via RPA connectors need operator sign-off", defaultChecked: true },
        ].map((row) => (
          <div key={row.label} className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{row.label}</p>
              <p className="text-xs text-muted-foreground">{row.desc}</p>
            </div>
            <Switch defaultChecked={row.defaultChecked} onCheckedChange={() => toast.success("Security setting updated")} />
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function ApiKeysSettings() {
  const keys = [
    { id: "key_1", name: "Production ingestion key", created: "2026-02-11", scope: "ingest:write", lastUsed: "2m ago" },
    { id: "key_2", name: "SOP automation service", created: "2026-03-02", scope: "sop:execute", lastUsed: "14m ago" },
    { id: "key_3", name: "Read-only analytics export", created: "2026-05-19", scope: "read:*", lastUsed: "3d ago" },
  ];
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div>
          <CardTitle>API Keys</CardTitle>
          <CardDescription>Keys used by connectors and internal services</CardDescription>
        </div>
        <Button size="sm" onClick={() => toast.success("New API key generated")}>
          <Plus className="h-3.5 w-3.5" /> Generate key
        </Button>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Scope</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Last used</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((k) => (
              <TableRow key={k.id}>
                <TableCell className="font-medium">
                  <span className="inline-flex items-center gap-1.5"><Key className="h-3 w-3 text-muted-foreground" />{k.name}</span>
                </TableCell>
                <TableCell><Badge variant="muted">{k.scope}</Badge></TableCell>
                <TableCell className="text-xs text-muted-foreground">{k.created}</TableCell>
                <TableCell className="text-xs text-muted-foreground">{k.lastUsed}</TableCell>
                <TableCell className="text-right">
                  <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={() => toast.success("Key revoked")}>
                    <Trash2 className="h-3 w-3" /> Revoke
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function RetentionSettings() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Retention</CardTitle>
        <CardDescription>How long data is kept before automatic deletion</CardDescription>
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {[
          { label: "Message events", value: "90 days" },
          { label: "Conversation transcripts", value: "365 days" },
          { label: "Audit logs", value: "730 days" },
          { label: "AI processing traces", value: "30 days" },
        ].map((r) => (
          <div key={r.label} className="space-y-1.5">
            <Label>{r.label}</Label>
            <Select defaultValue={r.value}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="30 days">30 days</SelectItem>
                <SelectItem value="90 days">90 days</SelectItem>
                <SelectItem value="365 days">365 days</SelectItem>
                <SelectItem value="730 days">730 days</SelectItem>
              </SelectContent>
            </Select>
          </div>
        ))}
      </CardContent>
      <CardFooter className="justify-end">
        <Button size="sm" onClick={() => toast.success("Retention policy updated")}>Save changes</Button>
      </CardFooter>
    </Card>
  );
}

function SimpleGroup({ title, description, rows }: { title: string; description: string; rows: { label: string; desc: string }[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {rows.map((row) => (
          <div key={row.label} className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{row.label}</p>
              <p className="text-xs text-muted-foreground">{row.desc}</p>
            </div>
            <Switch defaultChecked onCheckedChange={() => toast.success("Setting updated")} />
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

export default function SettingsPage() {
  return (
    <div>
      <PageHeader title="Settings" description="Platform-wide configuration, grouped by area." />
      <Tabs defaultValue="platform">
        <TabsList className="flex-wrap h-auto">
          <TabsTrigger value="platform">Platform</TabsTrigger>
          <TabsTrigger value="channels">Channels</TabsTrigger>
          <TabsTrigger value="ai">AI Providers</TabsTrigger>
          <TabsTrigger value="sop">SOP</TabsTrigger>
          <TabsTrigger value="security">Security</TabsTrigger>
          <TabsTrigger value="webhooks">Webhooks</TabsTrigger>
          <TabsTrigger value="apikeys">API Keys</TabsTrigger>
          <TabsTrigger value="retention">Retention</TabsTrigger>
        </TabsList>

        <TabsContent value="platform"><PlatformSettings /></TabsContent>

        <TabsContent value="channels">
          <SimpleGroup
            title="Channels"
            description="Default behavior applied to newly connected connectors"
            rows={[
              { label: "Auto-normalize inbound payloads", desc: "Apply the standard message schema to every connector" },
              { label: "Require test before activation", desc: "New connectors must pass a test call before going live" },
            ]}
          />
        </TabsContent>

        <TabsContent value="ai">
          <SimpleGroup
            title="AI Providers"
            description="Model providers used for classification, drafting, and retrieval"
            rows={[
              { label: "Use primary provider for reply drafting", desc: "Falls back to secondary provider on timeout" },
              { label: "Log full prompts for audit", desc: "Store prompt and response pairs for 30 days" },
            ]}
          />
        </TabsContent>

        <TabsContent value="sop">
          <SimpleGroup
            title="SOP"
            description="Defaults applied across all SOP workflows"
            rows={[
              { label: "Auto-escalate on SLA breach", desc: "Notify the assigned operator's manager automatically" },
              { label: "Require dual approval on refunds over $500", desc: "Applies to the Refund Processing workflow" },
            ]}
          />
        </TabsContent>

        <TabsContent value="security"><SecuritySettings /></TabsContent>

        <TabsContent value="webhooks">
          <SimpleGroup
            title="Webhooks"
            description="Outbound event notifications to your own systems"
            rows={[
              { label: "Send event on SOP completion", desc: "POST to your configured endpoint when a workflow finishes" },
              { label: "Send event on channel disconnect", desc: "Notify your on-call system when a connector goes down" },
            ]}
          />
        </TabsContent>

        <TabsContent value="apikeys"><ApiKeysSettings /></TabsContent>

        <TabsContent value="retention"><RetentionSettings /></TabsContent>
      </Tabs>
    </div>
  );
}
