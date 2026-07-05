"use client";

import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type SortingState,
} from "@tanstack/react-table";
import { RefreshCw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import type { ChannelAccount } from "../../types/channel-account";
import { EmptyState } from "./empty-state";
import { channelAccountColumns } from "./channel-account-columns";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../ui/table";

export function ChannelAccountTable({
  accounts,
  busy,
  refreshing,
  onRefresh,
  onEdit,
  onAssign,
  onToggleAI,
  onDelete,
}: {
  accounts: ChannelAccount[];
  busy: boolean;
  refreshing: boolean;
  onRefresh: () => void;
  onEdit: (account: ChannelAccount) => void;
  onAssign: (account: ChannelAccount) => void;
  onToggleAI: (account: ChannelAccount) => void;
  onDelete: (account: ChannelAccount) => void;
}) {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState("");
  const columns = useMemo(
    () => channelAccountColumns({ onEdit, onAssign, onToggleAI, onDelete, busy }),
    [busy, onAssign, onDelete, onEdit, onToggleAI],
  );
  const table = useReactTable({
    data: accounts,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: { pagination: { pageSize: 10 } },
  });

  return (
    <Card className="min-w-0">
      <CardHeader className="gap-4 lg:flex lg:flex-row lg:items-center lg:justify-between">
        <div className="grid gap-1.5">
          <CardTitle>账号列表</CardTitle>
          <CardDescription>共 {accounts.length} 个通道账号，支持搜索、排序和分页。</CardDescription>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" aria-hidden="true" />
            <Input
              className="pl-9 sm:w-64"
              value={globalFilter}
              onChange={(event) => setGlobalFilter(event.target.value)}
              placeholder="搜索账号、设备、消息端"
            />
          </div>
          <Button type="button" variant="outline" loading={refreshing} onClick={onRefresh}>
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            刷新
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {accounts.length === 0 ? (
          <EmptyState title="暂无通道账号" description="新增账号或导入 CSV 后，数据将在这里显示。" />
        ) : (
          <div className="overflow-x-auto overflow-y-hidden rounded-md border border-border">
            <Table className="min-w-[860px]">
              <TableHeader>
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id} className="hover:bg-transparent">
                    {headerGroup.headers.map((header) => (
                      <TableHead key={header.id}>
                        {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows.length ? (
                  table.getRowModel().rows.map((row) => (
                    <TableRow key={row.id}>
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={columns.length} className="h-28 text-center text-muted-foreground">
                      当前筛选条件下没有匹配结果
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        )}
        {accounts.length > 0 ? (
          <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
            <span>
              第 {table.getState().pagination.pageIndex + 1} / {table.getPageCount()} 页
            </span>
            <div className="flex gap-2">
              <Button type="button" variant="outline" size="sm" disabled={!table.getCanPreviousPage()} onClick={() => table.previousPage()}>
                上一页
              </Button>
              <Button type="button" variant="outline" size="sm" disabled={!table.getCanNextPage()} onClick={() => table.nextPage()}>
                下一页
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
