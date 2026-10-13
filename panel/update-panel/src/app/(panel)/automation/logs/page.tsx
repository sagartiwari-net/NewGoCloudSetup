"use client"

import { useState } from "react"

import { ChoiceSelect } from "@/components/choice-select"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import { automationOptions, listAutomationLogs } from "@/lib/api/client"
import type { AutomationLog } from "@/lib/api/types"
import { formatIst } from "@/lib/format"
import { useListPage } from "@/lib/page-size"

export default function AutomationLogsPage() {
  const [query, setQuery] = useState("")
  const [jobId, setJobId] = useState("all")
  const [level, setLevel] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading } = useResource(`automation-logs-${page}-${pageSize}-${query}-${jobId}-${level}`, async () => {
    const [logs, options] = await Promise.all([
      listAutomationLogs({
        page,
        pageSize,
        query,
        jobId: jobId === "all" ? undefined : Number(jobId),
        level: level === "all" ? undefined : level,
      }),
      automationOptions(),
    ])
    return { logs, jobs: options.jobs }
  })
  const rows = data?.logs.items ?? []
  const filtered = query.trim() !== "" || jobId !== "all" || level !== "all"

  const columns: DataColumn<AutomationLog>[] = [
    {
      id: "time",
      header: "Time",
      cell: (row) => formatIst(row.created_at),
      sortValue: (row) => row.created_at,
    },
    { id: "job", header: "Job", cell: (row) => row.job_name, sortValue: (row) => row.job_name },
    {
      id: "credential",
      header: "Credential",
      cell: (row) => row.credential_name,
      sortValue: (row) => row.credential_name,
    },
    {
      id: "level",
      header: "Level",
      cell: (row) => <Badge variant={row.level === "error" ? "destructive" : "secondary"}>{row.level}</Badge>,
      sortValue: (row) => row.level,
    },
    { id: "message", header: "Message", cell: (row) => row.message, sortValue: (row) => row.message },
  ]

  return (
    <>
      <PageHeader title="Logs" description="What the worker did in the last 3 hours." />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="log-search">Search</FieldLabel>
          <Input
            id="log-search"
            value={query}
            placeholder="Message, job, or credential"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="log-job">Job</FieldLabel>
          <ChoiceSelect
            id="log-job"
            value={jobId}
            onValueChange={(value) => {
              setJobId(value)
              setPage(1)
            }}
            items={[
              { label: "All jobs", value: "all" },
              ...(data?.jobs ?? []).map((job) => ({ label: job.name, value: String(job.id) })),
            ]}
          />
        </Field>
        <Field className="md:w-48">
          <FieldLabel htmlFor="log-level">Level</FieldLabel>
          <ChoiceSelect
            id="log-level"
            value={level}
            onValueChange={(value) => {
              setLevel(value)
              setPage(1)
            }}
            items={[
              { label: "All levels", value: "all" },
              { label: "Info", value: "info" },
              { label: "Error", value: "error" },
            ]}
          />
        </Field>
      </div>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.logs.page ?? page,
              total: data?.logs.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${jobId}-${level}`}
            emptyTitle={filtered ? "No matches" : "No logs"}
            emptyDescription={filtered ? "Try another job or level." : "Logs show up after a job runs."}
          />
        </CardContent>
      </Card>
    </>
  )
}
