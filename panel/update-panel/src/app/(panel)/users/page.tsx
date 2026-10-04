"use client"

import { useState } from "react"
import Link from "next/link"
import { ExternalLinkIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { DataTable, type DataColumn } from "@/components/data-table"
import { ListFilters } from "@/components/list-filters"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Truncated } from "@/components/truncated"
import { UserLink, userReportHref } from "@/components/user-link"
import { buttonVariants } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { useResource } from "@/hooks/use-resource"
import { listDirectoryUsers, listResellers, listTools, type Reseller } from "@/lib/api/client"
import type { DirectoryUser } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { cn } from "@/lib/utils"

export default function UsersDirectoryPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [toolId, setToolId] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const isMaster = session?.role === "master"

  const { data, loading } = useResource(
    `${session?.role ?? "none"}-directory-${page}-${pageSize}-${query}-${toolId}-${resellerId}`,
    async () => {
      const [users, tools, resellers] = await Promise.all([
        listDirectoryUsers({
          page,
          pageSize,
          query,
          toolId: toolId === "all" ? 0 : Number(toolId),
          resellerId: isMaster && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        listTools(),
        isMaster
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return { users, tools, resellers: resellers.items }
    },
  )

  const rows = data?.users.items ?? []

  const columns: DataColumn<DirectoryUser>[] = [
    {
      id: "username",
      header: "Username",
      cell: (row) => <UserLink username={row.username} />,
      sortValue: (row) => row.username,
    },
    {
      id: "tools",
      header: "Tools",
      cell: (row) => <Truncated value={row.tool_names.join(", ") || "—"} />,
      sortValue: (row) => row.tool_names.join(", "),
    },
    {
      id: "websites",
      header: "Websites",
      cell: (row) => (
        <span className="text-sm">
          {row.website_count}{" "}
          <span className="text-muted-foreground">
            {row.website_names.slice(0, 2).join(", ")}
            {row.website_names.length > 2 ? "…" : ""}
          </span>
        </span>
      ),
      sortValue: (row) => row.website_count,
    },
    {
      id: "status",
      header: "Status",
      cell: (row) => <StatusBadge status={row.status} />,
      sortValue: (row) => row.status,
    },
    {
      id: "logins",
      header: "Logins",
      cell: (row) => String(row.login_count),
      sortValue: (row) => row.login_count,
    },
    {
      id: "ips",
      header: "IPs",
      cell: (row) => String(row.distinct_ips),
      sortValue: (row) => row.distinct_ips,
    },
    {
      id: "last",
      header: "Last login",
      cell: (row) => (row.last_login_at ? formatTime(row.last_login_at) : "—"),
      sortValue: (row) => row.last_login_at,
    },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <Link
          href={userReportHref(row.username)}
          className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
        >
          Open report
          <ExternalLinkIcon data-icon="inline-end" />
        </Link>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Users"
        description="One row per username. Open a report for logins, IPs, limits, extension activity, and security."
      />
      <ListFilters
        query={query}
        onQuery={(value) => {
          setQuery(value)
          setPage(1)
        }}
        searchId="directory-user-search"
        placeholder="Username, tool, or website"
        toolId={toolId}
        onTool={(value) => {
          setToolId(value)
          setPage(1)
        }}
        tools={data?.tools ?? []}
        resellerId={isMaster ? resellerId : undefined}
        onReseller={
          isMaster
            ? (value) => {
                setResellerId(value)
                setPage(1)
              }
            : undefined
        }
        resellers={isMaster ? data?.resellers : undefined}
      />
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.username}
            loading={loading}
            paging={{
              page: data?.users.page ?? page,
              total: data?.users.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${toolId}-${resellerId}`}
            emptyTitle={query || toolId !== "all" || resellerId !== "all" ? "No matches" : "No users"}
            emptyDescription="Users appear here from panel accounts and login activity."
          />
        </CardContent>
      </Card>
    </>
  )
}
