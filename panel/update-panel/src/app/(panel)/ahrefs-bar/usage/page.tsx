"use client"

import { useState } from "react"
import { ArrowLeftIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { Truncated } from "@/components/truncated"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import {
  getAhrefsBarMeters,
  listAhrefsBarUsage,
  listAhrefsBarUsageUsers,
  resetAhrefsBarMeters,
} from "@/lib/api/client"
import type { AhrefsBarUsageRow, AhrefsBarUsageUserRow } from "@/lib/api/types"
import { useListPage } from "@/lib/page-size"

function shortEndpoint(ep: string) {
  return ep
    .replace(/^tbGet/, "")
    .replace(/V3$/, "")
    .replace(/PlanPrice$/, "")
}

function formatWhen(iso: string) {
  if (!iso) return "—"
  try {
    const d = new Date(iso.endsWith("Z") || iso.includes("+") ? iso : iso + "Z")
    if (Number.isNaN(d.getTime())) return iso
    return d.toLocaleString(undefined, {
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })
  } catch {
    return iso
  }
}

function meterLabel(used: number, limit: number) {
  return `${used} / ${limit}`
}

export default function AhrefsBarUsagePage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [selectedUser, setSelectedUser] = useState<string | null>(null)
  const [selectedResellerId, setSelectedResellerId] = useState<number | undefined>()
  const [resetting, setResetting] = useState(false)
  const { page, setPage, pageSize } = useListPage()

  const { data: usageData, loading: usageLoading, reload: reloadUsage } = useResource(
    `ab-usage-detail-${session?.role ?? "none"}-${selectedUser ?? ""}-${page}-${pageSize}-${query}`,
    () =>
      selectedUser
        ? listAhrefsBarUsage({ page, pageSize, query, login: selectedUser })
        : Promise.resolve({ items: [] as AhrefsBarUsageRow[], page: 1, total: 0, pageSize }),
  )
  const { data: usersData, loading: usersLoading, reload: reloadUsers } = useResource(
    `ab-usage-users-${session?.role ?? "none"}-${page}-${pageSize}-${query}`,
    () =>
      selectedUser
        ? Promise.resolve({ items: [] as AhrefsBarUsageUserRow[], page: 1, total: 0, pageSize })
        : listAhrefsBarUsageUsers({ page, pageSize, query }),
  )
  const loading = selectedUser ? usageLoading : usersLoading
  const reload = () => (selectedUser ? reloadUsage() : reloadUsers())

  const {
    data: meters,
    loading: metersLoading,
    reload: reloadMeters,
  } = useResource(
    `ab-meters-${selectedUser ?? ""}-${selectedResellerId ?? 0}`,
    () =>
      selectedUser
        ? getAhrefsBarMeters(selectedUser, selectedResellerId)
        : Promise.resolve(null),
  )

  async function handleReset(login: string, resellerId?: number) {
    if (!login || resetting) return
    setResetting(true)
    try {
      await resetAhrefsBarMeters(login, resellerId)
      await Promise.all([reload(), reloadMeters()])
    } finally {
      setResetting(false)
    }
  }

  const userColumns: DataColumn<AhrefsBarUsageUserRow>[] = [
    {
      id: "user",
      header: "User",
      cell: (row) => (
        <button
          type="button"
          className="text-left font-medium text-foreground underline-offset-4 hover:underline"
          onClick={() => {
            setSelectedUser(row.amember_login)
            setSelectedResellerId(row.reseller_id)
            setPage(1)
          }}
        >
          {row.amember_login || "—"}
        </button>
      ),
      sortValue: (row) => row.amember_login,
    },
    {
      id: "hourly",
      header: "Hourly",
      cell: (row) => (
        <span className="tabular-nums">
          {meterLabel(row.hourly_used ?? 0, row.hourly_limit ?? 0)}
        </span>
      ),
      sortValue: (row) => row.hourly_used ?? 0,
    },
    {
      id: "daily",
      header: "Daily",
      cell: (row) => (
        <span className="tabular-nums">
          {meterLabel(row.daily_used ?? 0, row.daily_limit ?? 0)}
        </span>
      ),
      sortValue: (row) => row.daily_used ?? 0,
    },
    {
      id: "requests",
      header: "Requests",
      cell: (row) => row.requests,
      sortValue: (row) => row.requests,
    },
    {
      id: "credits",
      header: "Credits",
      cell: (row) => row.credits,
      sortValue: (row) => row.credits,
    },
    {
      id: "cache",
      header: "Cache hits",
      cell: (row) => row.cache_hits,
      sortValue: (row) => row.cache_hits,
    },
    {
      id: "last",
      header: "Last activity",
      cell: (row) => formatWhen(row.last_at),
      sortValue: (row) => row.last_at,
    },
    {
      id: "actions",
      header: "",
      cell: (row) => (
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setSelectedUser(row.amember_login)
              setSelectedResellerId(row.reseller_id)
              setPage(1)
            }}
          >
            Logs
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={resetting}
            onClick={() => handleReset(row.amember_login, row.reseller_id)}
          >
            Reset
          </Button>
        </div>
      ),
    },
  ]

  const detailColumns: DataColumn<AhrefsBarUsageRow>[] = [
    {
      id: "time",
      header: "When",
      cell: (row) => (
        <span className="whitespace-nowrap text-muted-foreground">{formatWhen(row.created_at)}</span>
      ),
      sortValue: (row) => row.created_at,
    },
    {
      id: "endpoint",
      header: "Action",
      cell: (row) => (
        <span className="font-mono text-xs" title={row.endpoint}>
          {shortEndpoint(row.endpoint)}
        </span>
      ),
      sortValue: (row) => row.endpoint,
    },
    {
      id: "target",
      header: "Target",
      cell: (row) => <Truncated value={row.target || "—"} maxWidthClass="max-w-[14rem] md:max-w-xs" />,
      sortValue: (row) => row.target,
    },
    {
      id: "credits",
      header: "Cr",
      cell: (row) => row.credits,
      sortValue: (row) => row.credits,
    },
    {
      id: "cache",
      header: "Cache",
      cell: (row) => (row.cache_hit ? "hit" : "—"),
      sortValue: (row) => (row.cache_hit ? 1 : 0),
    },
    {
      id: "status",
      header: "Status",
      cell: (row) => row.status_code,
      sortValue: (row) => row.status_code,
    },
  ]

  return (
    <>
      <PageHeader
        title={selectedUser ? `Usage · ${selectedUser}` : "Ahrefs Bar usage"}
        description={
          selectedUser
            ? "Request log for this user (what the toolbar called)."
            : "One row per user. Hourly / daily meters + Reset clear the current windows."
        }
        action={
          selectedUser ? (
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                disabled={resetting}
                onClick={() => handleReset(selectedUser, selectedResellerId)}
              >
                Reset meters
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setSelectedUser(null)
                  setSelectedResellerId(undefined)
                  setPage(1)
                }}
              >
                <ArrowLeftIcon data-icon="inline-start" />
                All users
              </Button>
            </div>
          ) : undefined
        }
      />
      {selectedUser ? (
        <div className="grid gap-3 sm:grid-cols-2 md:max-w-lg">
          <Card>
            <CardContent className="pt-4">
              <p className="text-xs text-muted-foreground">Hourly</p>
              <p className="mt-1 text-lg font-medium tabular-nums">
                {metersLoading && !meters
                  ? "…"
                  : meterLabel(meters?.hourly_used ?? 0, meters?.hourly_limit ?? 0)}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-4">
              <p className="text-xs text-muted-foreground">Daily (IST)</p>
              <p className="mt-1 text-lg font-medium tabular-nums">
                {metersLoading && !meters
                  ? "…"
                  : meterLabel(meters?.daily_used ?? 0, meters?.daily_limit ?? 0)}
              </p>
            </CardContent>
          </Card>
        </div>
      ) : null}
      <Field className="md:max-w-sm">
        <FieldLabel htmlFor="ab-usage-search">Search</FieldLabel>
        <Input
          id="ab-usage-search"
          value={query}
          placeholder={selectedUser ? "Endpoint or target" : "User login"}
          onChange={(e) => {
            setQuery(e.target.value)
            setPage(1)
          }}
        />
      </Field>
      <Card>
        <CardContent>
          {selectedUser ? (
            <DataTable
              rows={usageData?.items ?? []}
              columns={detailColumns}
              rowKey={(row) => row.id}
              loading={loading}
              paging={{
                page: usageData?.page ?? page,
                total: usageData?.total ?? 0,
                pageSize,
                onPageChange: setPage,
              }}
              resetKey={`${selectedUser}-${query}`}
              emptyTitle={query ? "No matches" : "No requests"}
              emptyDescription="This user has no toolbar proxy requests yet."
            />
          ) : (
            <DataTable
              rows={usersData?.items ?? []}
              columns={userColumns}
              rowKey={(row) => `${row.reseller_id ?? 0}:${row.amember_login}`}
              loading={loading}
              paging={{
                page: usersData?.page ?? page,
                total: usersData?.total ?? 0,
                pageSize,
                onPageChange: setPage,
              }}
              resetKey={query}
              emptyTitle={query ? "No matches" : "No usage yet"}
              emptyDescription="Toolbar proxy requests will group here per user."
            />
          )}
        </CardContent>
      </Card>
    </>
  )
}
