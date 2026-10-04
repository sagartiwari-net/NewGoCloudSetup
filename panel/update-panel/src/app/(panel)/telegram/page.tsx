"use client"

import { useRef, useState, type RefObject } from "react"
import { PlusIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { UserLink } from "@/components/user-link"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { useResource } from "@/hooks/use-resource"
import {
  clearTelegramDeliveries,
  deleteTelegramDestination,
  getTelegramSettings,
  getTelegramTemplates,
  listAllWebsites,
  listResellers,
  listTelegramDeliveries,
  listTelegramDestinations,
  listTelegramRoutes,
  saveBotToken,
  saveRepeatMinutes,
  saveTelegramDestination,
  saveTelegramRoute,
  saveTelegramTemplates,
  type Reseller,
  type TelegramRouteRow,
} from "@/lib/api/client"
import type { TelegramDelivery, TelegramDestination, TelegramEvent, TelegramTemplates, Website } from "@/lib/api/types"
import { formatTime, maskSecret } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const placeholders = ["{username}", "{tool}", "{website}", "{time}", "{ip}", "{reason}", "{count}", "{from}", "{to}"]
const sample: Record<string, string> = {
  username: "nina",
  tool: "Ahrefs",
  website: "Ahrefs",
  from: "Ahrefs 1",
  to: "Ahrefs 2",
  time: "27/9/2026, 1:14:00 pm",
  ip: "203.0.113.10",
  reason: "8 opens from 2 IPs",
  count: "8",
}

function renderTemplate(template: string) {
  return template.replace(/\{(\w+)\}/g, (token, key: string) => sample[key] ?? token)
}

export default function TelegramPage() {
  const [tab, setTab] = useState("configuration")
  const [opened, setOpened] = useState({ configuration: true, routing: false, format: false, sent: false })

  function openTab(value: string) {
    setTab(value)
    setOpened((current) => ({ ...current, [value]: true }))
  }

  return (
    <>
      <PageHeader
        title="Telegram"
        description="Choose the bot, the chats, which tool sends which event, and the message text."
      />
      <Tabs value={tab} onValueChange={openTab} className="gap-4">
        <TabsList className="h-auto w-full flex-wrap justify-start gap-3 bg-transparent p-0 group-data-horizontal/tabs:h-auto">
          <TabsTrigger className="h-9 flex-none rounded-md border border-border px-3 data-active:bg-primary data-active:text-primary-foreground" value="configuration">Configuration</TabsTrigger>
          <TabsTrigger className="h-9 flex-none rounded-md border border-border px-3 data-active:bg-primary data-active:text-primary-foreground" value="routing">Routing</TabsTrigger>
          <TabsTrigger className="h-9 flex-none rounded-md border border-border px-3 data-active:bg-primary data-active:text-primary-foreground" value="format">Format</TabsTrigger>
          <TabsTrigger className="h-9 flex-none rounded-md border border-border px-3 data-active:bg-primary data-active:text-primary-foreground" value="sent">Sent</TabsTrigger>
        </TabsList>
        <TabsContent value="configuration">{opened.configuration ? <ConfigurationTab /> : null}</TabsContent>
        <TabsContent value="routing">{opened.routing ? <RoutingTab /> : null}</TabsContent>
        <TabsContent value="format">{opened.format ? <FormatTab /> : null}</TabsContent>
        <TabsContent value="sent">{opened.sent ? <SentTab /> : null}</TabsContent>
      </Tabs>
    </>
  )
}

function ConfigurationTab() {
  const { session } = useAuth()
  const isMaster = session?.role === "master"
  const [query, setQuery] = useState("")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `telegram-config-${session?.role}-${page}-${pageSize}-${query}-${resellerId}`,
    async () => {
      const [settings, destinations, resellers] = await Promise.all([
        isMaster ? getTelegramSettings() : Promise.resolve(null),
        listTelegramDestinations({
          page,
          pageSize,
          query,
          resellerId: isMaster && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        isMaster
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return { settings, destinations, resellers: resellers.items }
    },
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<TelegramDestination | null>(null)
  const [pendingDelete, setPendingDelete] = useState<TelegramDestination | null>(null)
  const [deleting, setDeleting] = useState(false)
  const rows = data?.destinations.items ?? []

  const columns: DataColumn<TelegramDestination>[] = [
    { id: "reseller", header: "Reseller", cell: (row) => row.reseller_name, sortValue: (row) => row.reseller_name },
    { id: "label", header: "Label", cell: (row) => row.label, sortValue: (row) => row.label },
    { id: "chat", header: "Chat id", cell: (row) => <span className="font-mono">{row.chat_id}</span>, sortValue: (row) => row.chat_id },
    { id: "enabled", header: "Enabled", cell: (row) => (row.enabled ? "Yes" : "No") },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-3">
          <Button variant="outline" size="sm" onClick={() => { setEditing(row); setOpen(true) }}>Edit</Button>
          <Button variant="destructive" size="sm" onClick={() => setPendingDelete(row)}>Delete</Button>
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      {isMaster && data?.settings ? (
        <MasterSettings token={data.settings.botToken ?? ""} minutes={data.settings.repeatMinutes} onSaved={reload} />
      ) : null}
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-1 flex-col gap-3 md:flex-row md:items-end">
          <Field className="md:max-w-sm md:flex-1">
            <FieldLabel htmlFor="chat-search">Search</FieldLabel>
            <Input
              id="chat-search"
              value={query}
              placeholder="Label, chat id, or reseller"
              onChange={(event) => {
                setQuery(event.target.value)
                setPage(1)
              }}
            />
          </Field>
          {isMaster ? (
            <Field className="md:w-64">
              <FieldLabel htmlFor="chat-reseller">Reseller</FieldLabel>
              <ChoiceSelect
                id="chat-reseller"
                value={resellerId}
                onValueChange={(value) => {
                  setResellerId(value)
                  setPage(1)
                }}
                items={[
                  { label: "All resellers", value: "all" },
                  ...(data?.resellers ?? []).map((row) => ({ label: row.username, value: String(row.id) })),
                ]}
              />
            </Field>
          ) : null}
        </div>
        <Button onClick={() => { setEditing(null); setOpen(true) }}>
          <PlusIcon data-icon="inline-start" />
          New chat
        </Button>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Chats</CardTitle>
          <CardDescription>A short label plus the Telegram chat id. One reseller can have many rows.</CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.destinations.page ?? page,
              total: data?.destinations.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            emptyTitle="No chats"
            emptyDescription="Add a label and a chat id."
          />
        </CardContent>
      </Card>
      <FormOverlay open={open} onOpenChange={setOpen} title={editing ? "Edit chat" : "New chat"} description="The label is the short name you see in routing.">
        {open ? (
          <ChatForm
            key={editing?.id ?? "new"}
            destination={editing}
            resellers={data?.resellers ?? []}
            master={isMaster}
            onDone={async () => {
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(next) => { if (!next) setPendingDelete(null) }}
        title="Delete chat"
        description={pendingDelete ? `Delete ${pendingDelete.label}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteTelegramDestination(pendingDelete.id), "Chat deleted").then((ok) => {
            setDeleting(false)
            if (!ok) return
            setPendingDelete(null)
            reload()
          })
        }}
      />
    </div>
  )
}

function MasterSettings({ token, minutes, onSaved }: { token: string; minutes: number; onSaved: () => void }) {
  const [value, setValue] = useState(token)
  const [wait, setWait] = useState(String(minutes))
  const [revealed, setRevealed] = useState(false)
  const [pending, setPending] = useState(false)

  return (
    <Card>
      <CardHeader>
        <CardTitle>Bot</CardTitle>
        <CardDescription>One bot sends every message. Repeat wait blocks a second message for the same username, tool, and event.</CardDescription>
      </CardHeader>
      <CardContent>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="bot-token">Bot token</FieldLabel>
            <Input id="bot-token" className="font-mono" value={revealed ? value : maskSecret(value)} readOnly={!revealed} onChange={(event) => setValue(event.target.value)} />
          </Field>
          <Field className="max-w-xs">
            <FieldLabel htmlFor="repeat-wait">Repeat wait (min)</FieldLabel>
            <Input id="repeat-wait" inputMode="numeric" value={wait} onChange={(event) => setWait(event.target.value)} />
            <FieldDescription>The number is minutes. After a message for a username, tool, and event, the next one waits this long.</FieldDescription>
          </Field>
          <div className="flex flex-wrap gap-3">
            <Button type="button" variant="outline" onClick={() => setRevealed((current) => !current)}>{revealed ? "Hide" : "Reveal"}</Button>
            <Button
              disabled={pending}
              onClick={() => {
                setPending(true)
                void runMutation(async () => {
                  await saveBotToken(value)
                  await saveRepeatMinutes(Number(wait))
                  return { status: "ok" as const }
                }, "Configuration saved").then((ok) => {
                  setPending(false)
                  if (ok) onSaved()
                })
              }}
            >
              Save
            </Button>
          </div>
        </FieldGroup>
      </CardContent>
    </Card>
  )
}

function ChatForm({
  destination,
  resellers,
  master,
  onDone,
}: {
  destination: TelegramDestination | null
  resellers: Reseller[]
  master: boolean
  onDone: () => Promise<void>
}) {
  const [label, setLabel] = useState(destination?.label ?? "")
  const [chatId, setChatId] = useState(destination?.chat_id ?? "")
  const [resellerId, setResellerId] = useState(destination ? String(destination.reseller_id) : resellers[0] ? String(resellers[0].id) : "")
  const [enabled, setEnabled] = useState(destination?.enabled ?? true)
  const [pending, setPending] = useState(false)

  return (
    <FieldGroup>
      {master ? (
        <Field>
          <FieldLabel htmlFor="dest-reseller">Reseller</FieldLabel>
          <ChoiceSelect
            id="dest-reseller"
            value={resellerId}
            onValueChange={setResellerId}
            items={resellers.map((row) => ({ label: row.username, value: String(row.id) }))}
          />
        </Field>
      ) : null}
      <Field>
        <FieldLabel htmlFor="dest-label">Label</FieldLabel>
        <Input id="dest-label" value={label} onChange={(event) => setLabel(event.target.value)} />
      </Field>
      <Field>
        <FieldLabel htmlFor="dest-chat">Chat id</FieldLabel>
        <Input id="dest-chat" className="font-mono" value={chatId} onChange={(event) => setChatId(event.target.value)} />
      </Field>
      <Field orientation="horizontal">
        <Switch id="dest-enabled" checked={enabled} onCheckedChange={setEnabled} />
        <FieldLabel htmlFor="dest-enabled">Enabled</FieldLabel>
      </Field>
      <Button
        disabled={pending}
        onClick={() => {
          setPending(true)
          void runMutation(
            () =>
              saveTelegramDestination({
                id: destination?.id,
                reseller_id: master ? Number(resellerId) : destination?.reseller_id ?? 0,
                label,
                chat_id: chatId,
                enabled,
              }),
            destination ? "Chat updated" : "Chat saved",
          ).then(async (ok) => {
            setPending(false)
            if (ok) await onDone()
          })
        }}
      >
        Save chat
      </Button>
    </FieldGroup>
  )
}

function RoutingTab() {
  const { session } = useAuth()
  const isMaster = session?.role === "master"
  const [query, setQuery] = useState("")
  const [event, setEvent] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `telegram-routes-${session?.role}-${page}-${pageSize}-${query}-${event}-${resellerId}`,
    async () => {
      const [routes, resellers] = await Promise.all([
        listTelegramRoutes({
          page,
          pageSize,
          query,
          event: event === "all" ? "" : event,
          resellerId: isMaster && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        isMaster
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return { routes, resellers: resellers.items }
    },
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<TelegramRouteRow | null>(null)
  const rows = data?.routes.items ?? []

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-1 flex-col gap-3 md:flex-row md:items-end">
          <Field className="md:max-w-sm md:flex-1">
            <FieldLabel htmlFor="route-search">Search</FieldLabel>
            <Input
              id="route-search"
              value={query}
              placeholder="Tool"
              onChange={(eventTarget) => {
                setQuery(eventTarget.target.value)
                setPage(1)
              }}
            />
          </Field>
          <Field className="md:w-48">
            <FieldLabel htmlFor="route-event">Event</FieldLabel>
            <ChoiceSelect
              id="route-event"
              value={event}
              onValueChange={(value) => {
                setEvent(value)
                setPage(1)
              }}
              items={[
                { label: "All", value: "all" },
                { label: "logout", value: "logout" },
                { label: "spam", value: "spam" },
              ]}
            />
          </Field>
          {isMaster ? (
            <Field className="md:w-64">
              <FieldLabel htmlFor="route-reseller">Reseller</FieldLabel>
              <ChoiceSelect
                id="route-reseller"
                value={resellerId}
                onValueChange={(value) => {
                  setResellerId(value)
                  setPage(1)
                }}
                items={[
                  { label: "All resellers", value: "all" },
                  ...(data?.resellers ?? []).map((row) => ({ label: row.username, value: String(row.id) })),
                ]}
              />
            </Field>
          ) : null}
        </div>
        <Button onClick={() => { setEditing(null); setOpen(true) }}>
          <PlusIcon data-icon="inline-start" />
          New route
        </Button>
      </div>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={[
              { id: "tool", header: "Tool", cell: (row) => `${row.website_name} — ${row.website_domain}`, sortValue: (row) => row.website_domain },
              { id: "events", header: "Events", cell: (row) => row.events.join(", ") },
              { id: "chats", header: "Chats", cell: (row) => row.chat_labels.join(", ") || "—" },
              {
                id: "edit",
                header: "Edit",
                cell: (row) => (
                  <Button variant="outline" size="sm" onClick={() => { setEditing(row); setOpen(true) }}>Edit</Button>
                ),
              },
            ]}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.routes.page ?? page,
              total: data?.routes.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            emptyTitle="No routes"
            emptyDescription="Pick a tool, the events, and the chats that should hear them."
          />
        </CardContent>
      </Card>
      <FormOverlay open={open} onOpenChange={setOpen} title={editing ? "Edit route" : "New route"} description="One website has one route. Chats can come from more than one reseller.">
        {open ? <RouteForm key={editing?.id ?? "new"} route={editing} onDone={async () => { setOpen(false); reload() }} /> : null}
      </FormOverlay>
    </div>
  )
}

function RouteForm({ route, onDone }: { route: TelegramRouteRow | null; onDone: () => Promise<void> }) {
  const { session } = useAuth()
  const { data } = useResource(`route-form-${session?.role}`, async () => {
    const [websites, destinations, resellers] = await Promise.all([
      listAllWebsites(),
      listTelegramDestinations({ page: 1, pageSize: 50, query: "", resellerId: 0, allChats: true }),
      session?.role === "master"
        ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
        : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
    ])
    return { websites, destinations: destinations.items, resellers: resellers.items }
  })
  const [websiteId, setWebsiteId] = useState(route ? String(route.website_id) : "")
  const [events, setEvents] = useState<TelegramEvent[]>(route?.events ?? ["logout", "spam"])
  const [destinationIds, setDestinationIds] = useState<number[]>(route?.destination_ids ?? [])
  const [pending, setPending] = useState(false)
  const site = Number(websiteId)
  const groups = new Map<string, TelegramDestination[]>()
  for (const chat of data?.destinations ?? []) {
    const name = chat.reseller_name || "Chats"
    const list = groups.get(name) ?? []
    list.push(chat)
    groups.set(name, list)
  }

  return (
    <FieldGroup>
      <Field>
        <FieldLabel htmlFor="route-tool">Tool</FieldLabel>
        <ChoiceSelect
          id="route-tool"
          value={websiteId}
          onValueChange={(value) => {
            setWebsiteId(value)
            setDestinationIds([])
          }}
          items={(data?.websites ?? []).map((website: Website) => {
            const owner = data?.resellers.find((row) => row.website_ids.includes(website.id))
            return {
              label: `${website.name} — ${website.domain} — ${owner?.username ?? "—"}`,
              value: String(website.id),
            }
          })}
        />
      </Field>
      <FieldSet>
        <FieldLegend>Events</FieldLegend>
        <Field orientation="horizontal">
          <Checkbox
            id="route-event-logout"
            checked={events.includes("logout")}
            onCheckedChange={(next) =>
              setEvents((current) =>
                next ? ([...new Set([...current, "logout" as const])] as TelegramEvent[]) : current.filter((item) => item !== "logout"),
              )
            }
          />
          <FieldLabel htmlFor="route-event-logout">Logout</FieldLabel>
        </Field>
        <Field orientation="horizontal">
          <Checkbox
            id="route-event-spam"
            checked={events.includes("spam")}
            onCheckedChange={(next) =>
              setEvents((current) =>
                next ? ([...new Set([...current, "spam" as const])] as TelegramEvent[]) : current.filter((item) => item !== "spam"),
              )
            }
          />
          <FieldLabel htmlFor="route-event-spam">Spam</FieldLabel>
        </Field>
      </FieldSet>
      {[...groups.entries()].map(([resellerName, chats]) => (
        <FieldSet key={resellerName}>
          <FieldLegend>{resellerName}</FieldLegend>
          {chats.map((chat) => (
            <Field key={chat.id} orientation="horizontal">
              <Checkbox
                id={`route-chat-${chat.id}`}
                checked={destinationIds.includes(chat.id)}
                onCheckedChange={(next) =>
                  setDestinationIds((current) => (next ? [...current, chat.id] : current.filter((id) => id !== chat.id)))
                }
              />
              <FieldLabel htmlFor={`route-chat-${chat.id}`}>{chat.label}</FieldLabel>
            </Field>
          ))}
        </FieldSet>
      ))}
      {groups.size === 0 ? <Alert><AlertDescription>No chats yet.</AlertDescription></Alert> : null}
      <Button
        disabled={pending || !websiteId || events.length === 0}
        onClick={() => {
          setPending(true)
          void runMutation(
            () => saveTelegramRoute({ website_id: site, events, destination_ids: destinationIds }),
            "Route saved",
          ).then(async (ok) => {
            setPending(false)
            if (ok) await onDone()
          })
        }}
      >
        Save route
      </Button>
    </FieldGroup>
  )
}

function FormatTab() {
  const { session } = useAuth()
  const { data, reload } = useResource(`telegram-templates-${session?.role}`, getTelegramTemplates)
  if (!data) return null
  return <TemplateCards templates={data} master={session?.role === "master"} onSaved={reload} />
}

function TemplateCards({
  templates,
  master,
  onSaved,
}: {
  templates: TelegramTemplates
  master: boolean
  onSaved: () => void
}) {
  const [logout, setLogout] = useState(templates.logout)
  const [spam, setSpam] = useState(templates.spam)
  const [pending, setPending] = useState(false)
  const logoutRef = useRef<HTMLTextAreaElement>(null)
  const spamRef = useRef<HTMLTextAreaElement>(null)

  function insert(which: "logout" | "spam", token: string) {
    const ref = which === "logout" ? logoutRef : spamRef
    const current = which === "logout" ? logout : spam
    const element = ref.current
    const start = element?.selectionStart ?? current.length
    const end = element?.selectionEnd ?? current.length
    const next = `${current.slice(0, start)}${token}${current.slice(end)}`
    if (which === "logout") setLogout(next)
    else setSpam(next)
  }

  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <TemplateCard title="Logout" value={logout} master={master} inputRef={logoutRef} onChange={setLogout} onInsert={(token) => insert("logout", token)} />
      <TemplateCard title="Spam report" value={spam} master={master} inputRef={spamRef} onChange={setSpam} onInsert={(token) => insert("spam", token)} />
      {master ? (
        <Button
          className="justify-self-start lg:col-span-2"
          disabled={pending}
          onClick={() => {
            setPending(true)
            void runMutation(() => saveTelegramTemplates({ logout, spam }), "Templates saved").then((ok) => {
              setPending(false)
              if (ok) onSaved()
            })
          }}
        >
          Save templates
        </Button>
      ) : null}
    </div>
  )
}

function TemplateCard({
  title,
  value,
  master,
  inputRef,
  onChange,
  onInsert,
}: {
  title: string
  value: string
  master: boolean
  inputRef: RefObject<HTMLTextAreaElement | null>
  onChange: (value: string) => void
  onInsert: (token: string) => void
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>Preview uses sample values so you can read the exact message.</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {master ? (
          <>
            <div className="flex flex-wrap gap-2">
              {placeholders.map((token) => (
                <Button key={token} type="button" variant="outline" size="sm" onClick={() => onInsert(token)}>{token}</Button>
              ))}
            </div>
            <Textarea ref={inputRef} rows={4} value={value} onChange={(event) => onChange(event.target.value)} />
          </>
        ) : null}
        <p className="text-sm">{renderTemplate(value)}</p>
      </CardContent>
    </Card>
  )
}

function SentTab() {
  const { session } = useAuth()
  const isMaster = session?.role === "master"
  const [query, setQuery] = useState("")
  const [event, setEvent] = useState("all")
  const [status, setStatus] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const [confirmClear, setConfirmClear] = useState(false)
  const [clearing, setClearing] = useState(false)
  const { data, loading, reload } = useResource(
    `telegram-sent-${session?.role}-${page}-${pageSize}-${query}-${event}-${status}-${resellerId}`,
    async () => {
      const [deliveries, resellers] = await Promise.all([
        listTelegramDeliveries({
          page,
          pageSize,
          query,
          event: event === "all" ? "" : event,
          status: status === "all" ? "" : status,
          resellerId: isMaster && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        isMaster
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return { deliveries, resellers: resellers.items }
    },
  )
  const rows = data?.deliveries.items ?? []
  const columns: DataColumn<TelegramDelivery>[] = [
    {
      id: "username",
      header: "Username",
      cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    },
    { id: "tool", header: "Tool", cell: (row) => row.website_name },
    { id: "event", header: "Event", cell: (row) => row.event },
    { id: "recipients", header: "Recipients", cell: (row) => row.recipients.map((item) => item.label).join(", ") },
    { id: "status", header: "Status", cell: (row) => row.status },
    { id: "time", header: "Time", cell: (row) => formatTime(row.created_at) },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-1 flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
          <Field className="md:max-w-sm md:flex-1">
            <FieldLabel htmlFor="sent-search">Search</FieldLabel>
            <Input
              id="sent-search"
              value={query}
              placeholder="Username, tool, label, or chat id"
              onChange={(eventTarget) => {
                setQuery(eventTarget.target.value)
                setPage(1)
              }}
            />
          </Field>
          <Field className="md:w-40">
            <FieldLabel htmlFor="sent-event">Event</FieldLabel>
            <ChoiceSelect id="sent-event" value={event} onValueChange={(value) => { setEvent(value); setPage(1) }} items={[{ label: "All", value: "all" }, { label: "logout", value: "logout" }, { label: "spam", value: "spam" }]} />
          </Field>
          <Field className="md:w-40">
            <FieldLabel htmlFor="sent-status">Status</FieldLabel>
            <ChoiceSelect id="sent-status" value={status} onValueChange={(value) => { setStatus(value); setPage(1) }} items={[{ label: "All", value: "all" }, { label: "sent", value: "sent" }, { label: "skipped", value: "skipped" }]} />
          </Field>
          {isMaster ? (
            <Field className="md:w-64">
              <FieldLabel htmlFor="sent-reseller">Reseller</FieldLabel>
              <ChoiceSelect
                id="sent-reseller"
                value={resellerId}
                onValueChange={(value) => { setResellerId(value); setPage(1) }}
                items={[{ label: "All resellers", value: "all" }, ...(data?.resellers ?? []).map((row) => ({ label: row.username, value: String(row.id) }))]}
              />
            </Field>
          ) : null}
        </div>
        <Button variant="outline" onClick={() => setConfirmClear(true)}>Clear</Button>
      </div>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.deliveries.page ?? page,
              total: data?.deliveries.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            emptyTitle="No deliveries"
            emptyDescription="Sent and skipped messages show up here."
          />
        </CardContent>
      </Card>
      <ConfirmDialog
        open={confirmClear}
        onOpenChange={setConfirmClear}
        title="Clear sent log"
        description="Delete the delivery rows? Chats, routes, and templates stay."
        confirmLabel="Clear"
        pending={clearing}
        onConfirm={() => {
          setClearing(true)
          void runMutation(clearTelegramDeliveries, "Sent log cleared").then((ok) => {
            setClearing(false)
            if (!ok) return
            setConfirmClear(false)
            setPage(1)
            reload()
          })
        }}
      />
    </div>
  )
}
