"use client"

import { useState } from "react"
import { PlusIcon } from "lucide-react"

import { AccountEditor } from "@/components/account-editor"
import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Truncated } from "@/components/truncated"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import {
  assignAccountsProxy,
  assignAccountsUserAgent,
  listAccounts,
  listIngestLogs,
  listProxies,
  listUserAgents,
  listAllWebsites,
  type AccountListItem,
} from "@/lib/api/client"
import type { IngestLog } from "@/lib/api/types"
import { formatIst } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

function latestFor(logs: IngestLog[], accountId: number) {
  return logs.find((log) => log.account_id === accountId)
}

export default function AccountsPage() {
  const { session } = useAuth()
  const isMaster = session?.role === "master"
  const [query, setQuery] = useState("")
  const [websiteFilter, setWebsiteFilter] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${websiteFilter}`,
    async () => {
      const [accounts, websites, proxies, agents, logs] = await Promise.all([
        listAccounts({
          page,
          pageSize,
          query,
          websiteId: websiteFilter === "all" ? 0 : Number(websiteFilter),
        }),
        listAllWebsites(),
        listProxies({ page: 1, pageSize: 50, query: "", status: "all" }),
        listUserAgents({ page: 1, pageSize: 50, query: "" }),
        isMaster
          ? listIngestLogs({ page: 1, pageSize: 50, query: "", websiteId: 0, status: "all", accountId: 0 })
          : Promise.resolve({ items: [] as IngestLog[], total: 0, page: 1, pageSize: 50 }),
      ])
      return {
        accounts,
        websites,
        proxies: proxies.items,
        agents: agents.items,
        logs: logs.items,
      }
    },
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AccountListItem | null>(null)
  const [selected, setSelected] = useState<number[]>([])
  const [bulk, setBulk] = useState<"proxy" | "agent" | null>(null)
  const [bulkValue, setBulkValue] = useState("")
  const [applying, setApplying] = useState(false)

  const rows = data?.accounts.items ?? []
  const allChecked = rows.length > 0 && rows.every((row) => selected.includes(row.id))

  const columns: DataColumn<AccountListItem>[] = [
    {
      id: "select",
      header: "Select",
      headerCell: () => (
        <Checkbox
          aria-label="Select accounts on this page"
          checked={allChecked}
          onCheckedChange={(next) => setSelected(next ? rows.map((row) => row.id) : [])}
        />
      ),
      cell: (row) => (
        <Checkbox
          aria-label={`Select ${row.name}`}
          checked={selected.includes(row.id)}
          onCheckedChange={(next) =>
            setSelected((current) => (next ? [...current, row.id] : current.filter((id) => id !== row.id)))
          }
        />
      ),
    },
    { id: "name", header: "Name", cell: (row) => row.name, sortValue: (row) => row.name },
    { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
    { id: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} />, sortValue: (row) => row.status },
    {
      id: "description",
      header: "Description",
      cell: (row) => <Truncated value={row.description || "—"} />,
      sortValue: (row) => row.description,
    },
    {
      id: "cookie",
      header: "Cookie updated",
      cell: (row) => (row.cookie_updated_at ? formatIst(row.cookie_updated_at) : "—"),
      sortValue: (row) => row.cookie_updated_at,
    },
    {
      id: "edit",
      header: "Edit",
      cell: (row) => (
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setEditing(row)
            setOpen(true)
          }}
        >
          Edit
        </Button>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Mapped Accounts"
        description="Logins the proxies use. Each account keeps its own cookie, user agent, and proxy."
        action={
          <Button
            onClick={() => {
              setEditing(null)
              setOpen(true)
            }}
          >
            <PlusIcon data-icon="inline-start" />
            New account
          </Button>
        }
      />
      <div className="flex flex-col gap-3 md:flex-row md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="account-search">Search</FieldLabel>
          <Input
            id="account-search"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
              setSelected([])
            }}
            placeholder="Name, website, or description"
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="account-website">Website</FieldLabel>
          <ChoiceSelect
            id="account-website"
            value={websiteFilter}
            onValueChange={(value) => {
              setWebsiteFilter(value)
              setPage(1)
              setSelected([])
            }}
            items={[
              { label: "All websites", value: "all" },
              ...(data?.websites ?? []).map((website) => ({ label: website.name, value: String(website.id) })),
            ]}
          />
        </Field>
      </div>
      {selected.length > 0 ? (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-sm">{selected.length} selected</p>
          <Button
            variant="outline"
            onClick={() => {
              setBulk("proxy")
              setBulkValue("")
            }}
          >
            Set proxy
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              setBulk("agent")
              setBulkValue("")
            }}
          >
            Set user agent
          </Button>
        </div>
      ) : null}
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.accounts.page ?? page,
              total: data?.accounts.total ?? 0,
              pageSize,
              onPageChange: (next) => {
                setPage(next)
                setSelected([])
              },
            }}
            resetKey={`${query}-${websiteFilter}-${isMaster ? "master" : "reseller"}`}
            emptyTitle={query || websiteFilter !== "all" ? "No matches" : "No mapped accounts"}
            emptyDescription="Create an account to attach a login cookie to a website."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit account" : "New account"}
        description={
          isMaster
            ? "Basic holds the cookie, proxy, and note. Advanced holds the automation fields."
            : "Cookie, proxy, note, and show limit for this account."
        }
      >
        {open && session ? (
          <AccountEditor
            key={editing?.id ?? "new"}
            account={editing}
            websites={data?.websites ?? []}
            proxies={data?.proxies ?? []}
            agents={data?.agents ?? []}
            latestIngest={editing ? latestFor(data?.logs ?? [], editing.id) ?? null : null}
            role={session.role}
            onDone={() => {
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <FormOverlay
        open={bulk !== null}
        onOpenChange={(next) => {
          if (!next) setBulk(null)
        }}
        title={bulk === "agent" ? "Set user agent" : "Set proxy"}
        description="This replaces the one-off value on the checked accounts. Later edits to the saved row still update them."
      >
        <div className="flex flex-col gap-4">
          <Field>
            <FieldLabel htmlFor="bulk-choice">{bulk === "agent" ? "User agent" : "Proxy"}</FieldLabel>
            <ChoiceSelect
              id="bulk-choice"
              value={bulkValue}
              onValueChange={setBulkValue}
              items={
                bulk === "agent"
                  ? (data?.agents ?? []).map((agent) => ({ label: agent.name, value: String(agent.id) }))
                  : (data?.proxies ?? []).map((proxy) => ({ label: proxy.name, value: String(proxy.id) }))
              }
            />
          </Field>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Button variant="outline" onClick={() => setBulk(null)}>
              Cancel
            </Button>
            <Button
              disabled={!bulkValue || applying}
              onClick={() => {
                if (!bulkValue) return
                setApplying(true)
                const ids = selected
                const run =
                  bulk === "agent"
                    ? () => assignAccountsUserAgent(ids, Number(bulkValue))
                    : () => assignAccountsProxy(ids, Number(bulkValue))
                void runMutation(run, "Accounts updated").then((ok) => {
                  setApplying(false)
                  if (!ok) return
                  setBulk(null)
                  setSelected([])
                  reload()
                })
              }}
            >
              Update {selected.length} accounts
            </Button>
          </div>
        </div>
      </FormOverlay>
    </>
  )
}
