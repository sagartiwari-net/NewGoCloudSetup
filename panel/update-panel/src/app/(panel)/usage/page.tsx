"use client"

import { useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { DataTable, type DataColumn } from "@/components/data-table"
import { ListFilters } from "@/components/list-filters"
import { PageHeader } from "@/components/page-header"
import { UserLink } from "@/components/user-link"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useIsMobile } from "@/hooks/use-mobile"
import { useResource } from "@/hooks/use-resource"
import {
  listQuota,
  listResellers,
  listTools,
  resetUserUsage,
  type Reseller,
} from "@/lib/api/client"
import type { PanelUser, UsageEvent } from "@/lib/api/types"
import { formatIst, formatMeterList, logType } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

type QuotaRow = {
  id: string
  username: string
  website_id: number
  website_name: string
  tool_name: string
  user: PanelUser
  events: UsageEvent[]
}

export default function UsagePage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [toolId, setToolId] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${toolId}-${resellerId}`,
    async () => {
      const [quota, tools, resellers] = await Promise.all([
        listQuota({
          page,
          pageSize,
          query,
          toolId: toolId === "all" ? 0 : Number(toolId),
          resellerId: session?.role === "master" && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        listTools(),
        session?.role === "master"
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return { quota, tools, resellers: resellers.items }
    },
  )
  const [logsFor, setLogsFor] = useState<QuotaRow | null>(null)
  const isMobile = useIsMobile()
  const limitedTools = (data?.tools ?? []).filter((tool) => Array.isArray(tool.limits) && tool.limits.length > 0)

  const rows: QuotaRow[] = (data?.quota.items ?? []).map((item) => ({
    id: `${item.user.username}-${item.user.website_id}`,
    username: item.user.username,
    website_id: item.user.website_id,
    website_name: item.user.website_name,
    tool_name: item.user.tool_name,
    user: item.user,
    events: item.events,
  }))

  const columns: DataColumn<QuotaRow>[] = [
    {
      id: "username",
      header: "Username",
      cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
      sortValue: (row) => row.username,
    },
    { id: "tool", header: "Tool", cell: (row) => row.tool_name, sortValue: (row) => row.tool_name },
    {
      id: "limits",
      header: "Limits",
      cell: (row) =>
        formatMeterList(row.user.meters),
      sortValue: (row) => row.user.meters[0]?.used ?? 0,
    },
    {
      id: "logs",
      header: "Logs",
      cell: (row) => (
        <Button variant="outline" size="sm" onClick={() => setLogsFor(row)}>
          Logs
        </Button>
      ),
    },
    {
      id: "reset",
      header: "Reset",
      cell: (row) => (
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            void runMutation(() => resetUserUsage(row.user.id), "Usage reset").then((ok) => {
              if (ok) reload()
            })
          }}
        >
          Reset
        </Button>
      ),
    },
  ]

  const openLogs = logsFor
    ? rows.find((row) => row.id === logsFor.id) ?? logsFor
    : null

  return (
    <>
      <PageHeader
        title="Quota Logs"
        description="Each line is kept for that limit's reset period, then removed."
      />
      <ListFilters
        query={query}
        onQuery={(value) => {
          setQuery(value)
          setPage(1)
        }}
        searchId="usage-search"
        placeholder="Username, tool, or path"
        toolId={toolId}
        onTool={(value) => {
          setToolId(value)
          setPage(1)
        }}
        tools={limitedTools}
        resellerId={session?.role === "master" ? resellerId : undefined}
        onReseller={
          session?.role === "master"
            ? (value) => {
                setResellerId(value)
                setPage(1)
              }
            : undefined
        }
        resellers={session?.role === "master" ? data?.resellers : undefined}
      />
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.quota.page ?? page,
              total: data?.quota.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${toolId}-${resellerId}`}
            emptyTitle={query || toolId !== "all" || resellerId !== "all" ? "No matches" : "No quota rows"}
            emptyDescription="Users on tools with limits show up here."
          />
        </CardContent>
      </Card>
      <Dialog open={Boolean(openLogs)} onOpenChange={(next) => { if (!next) setLogsFor(null) }}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle>{openLogs ? `${openLogs.username} · ${openLogs.tool_name}` : "Logs"}</DialogTitle>
            <DialogDescription>Newest deduction first. Times are India Standard Time.</DialogDescription>
          </DialogHeader>
          {openLogs ? (
            openLogs.events.length === 0 ? (
              <p className="text-sm text-muted-foreground">No deductions for this user yet.</p>
            ) : isMobile ? (
              <div className="flex flex-col gap-3">
                {openLogs.events.map((event) => (
                  <Card key={event.id}>
                    <CardContent className="flex flex-col gap-2">
                      <div className="flex flex-col gap-1">
                        <span className="text-sm text-muted-foreground">Log type</span>
                        <span>{logType(event)}</span>
                      </div>
                      <div className="flex flex-col gap-1">
                        <span className="text-sm text-muted-foreground">Usage</span>
                        <span>{event.amount.toLocaleString()}</span>
                      </div>
                      <div className="flex flex-col gap-1">
                        <span className="text-sm text-muted-foreground">Target path</span>
                        <span className="break-all">{event.target_path}</span>
                      </div>
                      <div className="flex flex-col gap-1">
                        <span className="text-sm text-muted-foreground">Time (IST)</span>
                        <span>{formatIst(event.timestamp)}</span>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Log type</TableHead>
                    <TableHead>Usage</TableHead>
                    <TableHead>Target path</TableHead>
                    <TableHead>Time (IST)</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {openLogs.events.map((event) => (
                    <TableRow key={event.id}>
                      <TableCell>{logType(event)}</TableCell>
                      <TableCell>{event.amount.toLocaleString()}</TableCell>
                      <TableCell className="break-all">{event.target_path}</TableCell>
                      <TableCell className="whitespace-nowrap">{formatIst(event.timestamp)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}
