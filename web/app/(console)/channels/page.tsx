"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Settings2, FlaskConical, Power, Webhook } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { StatusBadge } from "@/components/shared/status-badge";
import { ChannelIcon } from "@/components/shared/channel-icon";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { apiGet, apiPost } from "@/lib/api";
import { CHANNEL_META } from "@/lib/mock-data";
import { formatNumber, formatRelativeTime } from "@/lib/utils";
import type { Channel } from "@/lib/types";
import { TableSkeleton } from "@/components/shared/table-skeleton";

const CAP_LABEL: Record<string, string> = {
  webhook: "Webhook",
  polling: "Polling",
  rpa: "RPA",
  api: "API",
  manual_approval: "Manual Approval",
};

async function fetchChannels() {
  const payload = await apiGet<{ channels: Channel[] }>("/api/v1/channels");
  return payload.channels;
}

export default function ChannelsPage() {
  const { data, isLoading, refetch } = useQuery({ queryKey: ["channels"], queryFn: fetchChannels });
  const [configTarget, setConfigTarget] = React.useState<Channel | null>(null);
  const [disableTarget, setDisableTarget] = React.useState<Channel | null>(null);

  return (
    <div>
      <PageHeader
        title="Channels"
        description="Every IM connector is standardized into the same event pipeline — no single channel is the core system."
        actions={
          <Button size="sm">
            <Webhook className="h-3.5 w-3.5" />
            Add connector
          </Button>
        }
      />

      {isLoading ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-[220px] animate-pulse rounded-lg border border-border bg-muted/40" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {data?.map((channel) => (
            <Card key={channel.id}>
              <CardContent className="p-4">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <ChannelIcon kind={channel.kind} className="h-8 w-8 [&_svg]:h-4 [&_svg]:w-4" />
                    <div>
                      <p className="text-sm font-semibold leading-tight">{CHANNEL_META[channel.kind].label}</p>
                      <p className="text-xs text-muted-foreground">{channel.name}</p>
                    </div>
                  </div>
                  <StatusBadge status={channel.status} />
                </div>

                <div className="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-xs">
                  <div>
                    <p className="text-muted-foreground">Receive via</p>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {channel.receiveCapabilities.map((c) => (
                        <Badge key={c} variant="muted">{CAP_LABEL[c]}</Badge>
                      ))}
                    </div>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Send via</p>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {channel.sendCapabilities.map((c) => (
                        <Badge key={c} variant="muted">{CAP_LABEL[c]}</Badge>
                      ))}
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-3 gap-2 rounded-md bg-muted/50 p-2.5 text-xs">
                  <div>
                    <p className="text-muted-foreground">Messages today</p>
                    <p className="mt-0.5 font-semibold tabular-nums">{formatNumber(channel.messagesToday)}</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Errors (24h)</p>
                    <p className={`mt-0.5 font-semibold tabular-nums ${channel.errorCount24h > 10 ? "text-destructive" : ""}`}>
                      {channel.errorCount24h}
                    </p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Last sync</p>
                    <p className="mt-0.5 font-semibold">{formatRelativeTime(channel.lastSyncAt)}</p>
                  </div>
                </div>

                <div className="mt-3 flex gap-1.5">
                  <Button variant="outline" size="sm" className="flex-1" onClick={() => setConfigTarget(channel)}>
                    <Settings2 className="h-3.5 w-3.5" />
                    Configure
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="flex-1"
                    onClick={() => toast.promise(apiPost(`/api/v1/channels/${channel.id}/test`).then(() => refetch()), {
                      loading: `Testing ${CHANNEL_META[channel.kind].label} connection…`,
                      success: "Connection healthy",
                      error: "Test failed",
                    })}
                  >
                    <FlaskConical className="h-3.5 w-3.5" />
                    Test
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    className="h-7 w-7 shrink-0"
                    disabled={channel.status === "disabled"}
                    onClick={() => setDisableTarget(channel)}
                    aria-label="Disable"
                  >
                    <Power className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={!!configTarget} onOpenChange={(o) => !o && setConfigTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Configure {configTarget && CHANNEL_META[configTarget.kind].label}</DialogTitle>
            <DialogDescription>
              Connector-level settings for {configTarget?.name}. Credentials are stored encrypted.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="display-name">Display name</Label>
              <Input id="display-name" defaultValue={configTarget?.name} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="webhook-url">Webhook URL</Label>
              <Input id="webhook-url" defaultValue="https://hooks.imintegration.io/ingest/wecom-01" readOnly />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="secret">Signing secret</Label>
              <Input id="secret" type="password" defaultValue="sk_live_••••••••••••" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setConfigTarget(null)}>Cancel</Button>
            <Button
              size="sm"
              onClick={() => {
                toast.success("Connector settings saved");
                setConfigTarget(null);
              }}
            >
              Save changes
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!disableTarget}
        onOpenChange={(o) => !o && setDisableTarget(null)}
        title={`Disable ${disableTarget ? CHANNEL_META[disableTarget.kind].label : ""}?`}
        description="Inbound messages on this connector will stop being ingested and queued outbound messages will be held until it's re-enabled."
        confirmLabel="Disable connector"
        onConfirm={() => {
          if (!disableTarget) return;
          toast.promise(apiPost(`/api/v1/channels/${disableTarget.id}/disable`).then(() => refetch()), {
            loading: "Disabling connector…",
            success: `${CHANNEL_META[disableTarget.kind].label} disabled`,
            error: "Disable failed",
          });
        }}
      />
    </div>
  );
}
