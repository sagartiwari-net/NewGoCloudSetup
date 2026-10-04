"use client"

import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts"

import { CountUp } from "@/components/count-up"
import { UserLink } from "@/components/user-link"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import type { AnalyticsSummary } from "@/lib/api/types"
import { dayLabel } from "@/lib/format"

const activityConfig = {
  logins: { label: "Logins", color: "var(--primary)" },
  extension: { label: "Extension", color: "var(--chart-2)" },
  switches: { label: "Switches", color: "var(--chart-3)" },
} satisfies ChartConfig

const rankConfig = {
  total: { label: "Activity", color: "var(--primary)" },
  count: { label: "Count", color: "var(--chart-2)" },
} satisfies ChartConfig

export function AnalyticsDashboard({
  summary,
  loading,
  websiteId,
}: {
  summary: AnalyticsSummary | null
  loading: boolean
  websiteId: number
}) {
  const series =
    summary?.series.map((row) => ({
      ...row,
      day: dayLabel(row.day),
    })) ?? []
  const topUsers = summary?.top_users ?? []
  const topActions = summary?.top_actions ?? []
  const tools = summary?.tools ?? []
  const switchReasons = summary?.switch_reasons ?? []

  return (
    <div className="flex flex-col gap-3">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Kpi title="Logins" value={summary?.counts.logins} loading={loading} />
        <Kpi title="Unique users" value={summary?.counts.unique_users} loading={loading} />
        <Kpi title="Extension events" value={summary?.counts.extension} loading={loading} />
        <Kpi title="Switches" value={summary?.counts.switches} loading={loading} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Activity per day</CardTitle>
          <CardDescription>Last {summary?.days ?? 7} days</CardDescription>
        </CardHeader>
        <CardContent>
          {!loading ? (
            <ChartContainer config={activityConfig} className="aspect-auto h-[220px] max-h-[220px] w-full">
              <BarChart data={series} barCategoryGap="24%">
                <CartesianGrid vertical={false} stroke="var(--border)" />
                <XAxis dataKey="day" tickLine={false} axisLine={false} />
                <YAxis
                  allowDecimals={false}
                  domain={[0, (max: number) => Math.max(max, 1)]}
                  width={32}
                  tickLine={false}
                  axisLine={false}
                />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar dataKey="logins" fill="var(--color-logins)" radius={4} />
                <Bar dataKey="extension" fill="var(--color-extension)" radius={4} />
                <Bar dataKey="switches" fill="var(--color-switches)" radius={4} />
              </BarChart>
            </ChartContainer>
          ) : null}
        </CardContent>
      </Card>

      <div className="grid gap-3 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Top users</CardTitle>
            <CardDescription>Logins + extension actions</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {!loading && topUsers.length > 0 ? (
              <ChartContainer config={rankConfig} className="aspect-auto h-[220px] max-h-[220px] w-full">
                <BarChart data={topUsers} layout="vertical" margin={{ left: 16 }}>
                  <CartesianGrid horizontal={false} stroke="var(--border)" />
                  <XAxis type="number" allowDecimals={false} tickLine={false} axisLine={false} />
                  <YAxis
                    type="category"
                    dataKey="username"
                    width={96}
                    tickLine={false}
                    axisLine={false}
                  />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Bar dataKey="total" fill="var(--color-total)" radius={4} />
                </BarChart>
              </ChartContainer>
            ) : null}
            <div className="flex flex-col gap-1">
              {topUsers.length === 0 && !loading ? (
                <p className="text-sm text-muted-foreground">No user activity in this range.</p>
              ) : null}
              {topUsers.map((row) => (
                <div key={row.username} className="flex items-center justify-between gap-3 text-sm">
                  <UserLink username={row.username} websiteId={websiteId || undefined} />
                  <span className="text-muted-foreground">
                    {row.logins} logins · {row.extension} ext
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Top extension actions</CardTitle>
            <CardDescription>Most used tool actions</CardDescription>
          </CardHeader>
          <CardContent>
            {!loading && topActions.length > 0 ? (
              <ChartContainer config={rankConfig} className="aspect-auto h-[260px] max-h-[260px] w-full">
                <BarChart data={topActions.map((row) => ({ name: row.name, count: row.count }))}>
                  <CartesianGrid vertical={false} stroke="var(--border)" />
                  <XAxis dataKey="name" tickLine={false} axisLine={false} />
                  <YAxis allowDecimals={false} width={32} tickLine={false} axisLine={false} />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Bar dataKey="count" fill="var(--color-count)" radius={4} />
                </BarChart>
              </ChartContainer>
            ) : (
              <p className="text-sm text-muted-foreground">No extension actions in this range.</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Tool breakdown</CardTitle>
            <CardDescription>By tool key / name</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {tools.length === 0 && !loading ? (
              <p className="text-sm text-muted-foreground">No tool usage yet.</p>
            ) : null}
            {tools.map((row) => (
              <div key={`${row.tool_key}-${row.tool_name}`} className="flex items-center justify-between gap-3 text-sm">
                <span>
                  {row.tool_key || row.tool_name}
                  {row.tool_key && row.tool_name && row.tool_key !== row.tool_name ? (
                    <span className="text-muted-foreground"> · {row.tool_name}</span>
                  ) : null}
                </span>
                <span className="text-muted-foreground">{row.count}</span>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Account switches</CardTitle>
            <CardDescription>Top reasons</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {switchReasons.length === 0 && !loading ? (
              <p className="text-sm text-muted-foreground">No switches in this range.</p>
            ) : null}
            {switchReasons.map((row) => (
              <div key={row.name} className="flex items-center justify-between gap-3 text-sm">
                <span>{row.name}</span>
                <span className="text-muted-foreground">{row.count}</span>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function Kpi({ title, value, loading }: { title: string; value?: number; loading: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle>{loading || value === undefined ? "—" : <CountUp value={value} />}</CardTitle>
      </CardHeader>
    </Card>
  )
}
