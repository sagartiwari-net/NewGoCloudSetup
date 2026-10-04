"use client"

import { useState } from "react"
import { toast } from "sonner"
import { CopyIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useResource } from "@/hooks/use-resource"
import { listAllWebsites, listAutomateTasks, listIngestLogs, type AutomateTask } from "@/lib/api/client"
import type { IngestLog } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { useListPage } from "@/lib/page-size"

const examples = [
  `{ "cookie": [ { "name": "session", "value": "…" } ] }`,
  `{ "session_data": { "cookies": [], "local_storage": {}, "indexed_db": {} } }`,
  `{ "key": "<ingest_key>", "cookie": [] }`,
]

export default function AutomatePage() {
  const { session } = useAuth()
  const [tab, setTab] = useState("tasks")
  const [query, setQuery] = useState("")
  const [websiteId, setWebsiteId] = useState("all")
  const [status, setStatus] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const [selected, setSelected] = useState<AutomateTask | null>(null)
  const { data, loading } = useResource(
    `${session?.role ?? "none"}-${tab}-${page}-${pageSize}-${query}-${websiteId}-${status}`,
    async () => {
      const site = websiteId === "all" ? 0 : Number(websiteId)
      const [tasks, logs, websites] = await Promise.all([
        tab === "tasks"
          ? listAutomateTasks({ page, pageSize, query, websiteId: site, status })
          : Promise.resolve(null),
        tab === "logs"
          ? listIngestLogs({ page, pageSize, query, websiteId: site, status, accountId: 0 })
          : Promise.resolve(null),
        listAllWebsites(),
      ])
      return { tasks, logs, websites }
    },
  )

  const columns: DataColumn<AutomateTask>[] = [
    { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
    { id: "name", header: "Account", cell: (row) => row.name, sortValue: (row) => row.name },
    {
      id: "uid",
      header: "Task UID",
      cell: (row) => <span className="font-mono">{row.automation_task_uid || "—"}</span>,
      sortValue: (row) => row.automation_task_uid,
    },
    {
      id: "key",
      header: "Ingest key",
      cell: (row) => <span className="font-mono">{row.automation_ingest_key || "—"}</span>,
      sortValue: (row) => row.automation_ingest_key,
    },
    {
      id: "status",
      header: "Last ingest status",
      cell: (row) => (row.last_status ? <StatusBadge status={row.last_status} /> : "—"),
      sortValue: (row) => row.last_status,
    },
    { id: "time", header: "Last ingest time", cell: (row) => formatTime(row.last_time), sortValue: (row) => row.last_time },
    { id: "bytes", header: "Bytes saved", cell: (row) => row.last_bytes, sortValue: (row) => row.last_bytes },
    { id: "error", header: "Error", cell: (row) => row.last_error || "—", sortValue: (row) => row.last_error },
    {
      id: "open",
      header: "Details",
      cell: (row) => (
        <Button variant="outline" size="sm" onClick={() => setSelected(row)}>
          Open
        </Button>
      ),
    },
  ]

  const logColumns: DataColumn<IngestLog>[] = [
    { id: "key", header: "Ingest key", cell: (row) => <span className="font-mono">{row.ingest_key}</span>, sortValue: (row) => row.ingest_key },
    { id: "account", header: "Account", cell: (row) => row.account_name, sortValue: (row) => row.account_name },
    { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
    { id: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} />, sortValue: (row) => row.status },
    { id: "error", header: "Error", cell: (row) => row.error_message || "—", sortValue: (row) => row.error_message },
    { id: "bytes", header: "Bytes", cell: (row) => row.bytes_saved, sortValue: (row) => row.bytes_saved },
    { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
  ]

  const ingestUrl = selected?.automation_ingest_key
    ? `https://ctrl.toolsmandi.com/api/updateauto?key=${selected.automation_ingest_key}`
    : ""
  const taskStatus = [
    { label: "All", value: "all" },
    { label: "saved", value: "saved" },
    { label: "failed", value: "failed" },
    { label: "no ingest yet", value: "none" },
  ]
  const logStatus = [
    { label: "All", value: "all" },
    { label: "saved", value: "saved" },
    { label: "failed", value: "failed" },
  ]

  return (
    <>
      <PageHeader
        title="Automate Task"
        description="The URL and key GoAuto should POST when it refreshes a cookie. This screen does not send the request."
        action={
          <Tabs
            value={tab}
            onValueChange={(value) => {
              setTab(value)
              setStatus("all")
              setPage(1)
            }}
          >
            <TabsList>
              <TabsTrigger value="tasks">Tasks</TabsTrigger>
              <TabsTrigger value="logs">Logs</TabsTrigger>
            </TabsList>
          </Tabs>
        }
      />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="automate-search">Search</FieldLabel>
          <Input
            id="automate-search"
            value={query}
            placeholder={tab === "tasks" ? "Website, account, task UID, or key" : "Key, account, error, or website"}
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="automate-website">Website</FieldLabel>
          <ChoiceSelect
            id="automate-website"
            value={websiteId}
            onValueChange={(value) => {
              setWebsiteId(value)
              setPage(1)
            }}
            items={[
              { label: "All websites", value: "all" },
              ...(data?.websites ?? []).map((website) => ({ label: website.name, value: String(website.id) })),
            ]}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="automate-status">Status</FieldLabel>
          <ChoiceSelect
            id="automate-status"
            value={status}
            onValueChange={(value) => {
              setStatus(value)
              setPage(1)
            }}
            items={tab === "tasks" ? taskStatus : logStatus}
          />
        </Field>
      </div>
      <Card>
        <CardContent>
          {tab === "tasks" ? (
            <DataTable
              rows={data?.tasks?.items ?? []}
              columns={columns}
              rowKey={(row) => row.id}
              loading={loading}
              paging={{
                page: data?.tasks?.page ?? page,
                total: data?.tasks?.total ?? 0,
                pageSize,
                onPageChange: setPage,
              }}
              resetKey={`${query}-${websiteId}-${status}`}
              emptyTitle="No mapped accounts"
              emptyDescription="Accounts show up here even before they have an ingest key."
            />
          ) : (
            <DataTable
              rows={data?.logs?.items ?? []}
              columns={logColumns}
              rowKey={(row) => row.id}
              loading={loading}
              paging={{
                page: data?.logs?.page ?? page,
                total: data?.logs?.total ?? 0,
                pageSize,
                onPageChange: setPage,
              }}
              resetKey={`${query}-${websiteId}-${status}`}
              emptyTitle="No ingest attempts"
              emptyDescription="Saved and failed cookie updates appear here."
            />
          )}
        </CardContent>
      </Card>
      <Sheet open={Boolean(selected)} onOpenChange={(next) => { if (!next) setSelected(null) }}>
        <SheetContent className="overflow-y-auto sm:max-w-xl">
          <SheetHeader>
            <SheetTitle>{selected?.name ?? "Account"}</SheetTitle>
            <SheetDescription>
              {selected ? `${selected.website_name} cookie update preview` : "Ingest preview"}
            </SheetDescription>
          </SheetHeader>
          {selected ? (
            <div className="flex flex-col gap-4 px-4 pb-4">
              <Alert>
                <AlertTitle>Preview only</AlertTitle>
                <AlertDescription>Nothing is sent from this panel. Copy the URL into GoAuto.</AlertDescription>
              </Alert>
              <div className="flex flex-col gap-2">
                <p className="text-sm font-medium">Ingest URL</p>
                <p className="font-mono text-sm break-all">{ingestUrl || "Add an ingest key on the account first."}</p>
                <Button
                  variant="outline"
                  className="self-start"
                  disabled={!ingestUrl}
                  onClick={() => {
                    void navigator.clipboard.writeText(ingestUrl).then(
                      () => toast.success("URL copied"),
                      () => toast.error("Could not copy the URL")
                    )
                  }}
                >
                  <CopyIcon data-icon="inline-start" />
                  Copy URL
                </Button>
              </div>
              <Separator />
              <div className="flex flex-col gap-1">
                <p className="text-sm">Method POST</p>
                <p className="font-mono text-sm">Authorization: Bearer ••••</p>
              </div>
              <div className="flex flex-col gap-2">
                <p className="text-sm font-medium">Accepted body shapes</p>
                {examples.map((example) => (
                  <pre key={example} className="font-mono text-sm whitespace-pre-wrap break-all">
                    {selected.automation_ingest_key
                      ? example.replace("<ingest_key>", selected.automation_ingest_key)
                      : example}
                  </pre>
                ))}
              </div>
              <Separator />
              <AccountLogs accountId={selected.id} />
            </div>
          ) : null}
        </SheetContent>
      </Sheet>
    </>
  )
}

function AccountLogs({ accountId }: { accountId: number }) {
  const { data, loading } = useResource(`account-logs-${accountId}`, () =>
    listIngestLogs({ page: 1, pageSize: 20, query: "", websiteId: 0, status: "all", accountId }),
  )
  const logs = data?.items ?? []
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm font-medium">Log for this key</p>
      {loading ? <p className="text-sm text-muted-foreground">Loading</p> : null}
      {!loading && logs.length === 0 ? (
        <p className="text-sm text-muted-foreground">No attempts for this account.</p>
      ) : null}
      {logs.map((log) => (
        <div key={log.id} className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={log.status} />
            <span className="text-sm">{formatTime(log.created_at)}</span>
            <span className="text-sm text-muted-foreground">{log.bytes_saved} bytes</span>
            <span className="text-sm text-muted-foreground">{log.client_ip}</span>
          </div>
          {log.error_message ? <p className="text-sm text-destructive">{log.error_message}</p> : null}
        </div>
      ))}
    </div>
  )
}
