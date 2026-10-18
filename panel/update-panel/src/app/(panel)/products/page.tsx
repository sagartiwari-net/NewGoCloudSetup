"use client"

import { useEffect, useState, type FormEvent } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon, XIcon } from "lucide-react"
import { toast } from "sonner"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { Textarea } from "@/components/ui/textarea"
import { bulkProductIDs, deleteProduct, listProducts, listResellerChoices, listWebsiteChoices, saveProduct, type Reseller } from "@/lib/api/client"
import type { ProductMap, Website } from "@/lib/api/types"
import { runMutation } from "@/lib/mutate"
import { useListPage } from "@/lib/page-size"

const schema = z.object({
  website_id: z.string().min(1, "Choose a website"),
  product_name: z.string().trim().min(1, "Enter a product name"),
})

type FormValues = z.infer<typeof schema>

function splitProductIDs(text: string) {
  const seen = new Set<string>()
  const ids: string[] = []
  for (const part of text.split(/[\s,;]+/)) {
    const value = part.trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    ids.push(value)
  }
  return ids
}

function websiteChoiceLabel(website: Website) {
  const domain = website.domain.trim()
  if (!domain || domain.toLowerCase() === website.name.trim().toLowerCase()) return website.name
  return `${website.name} · ${domain}`
}

export default function ProductsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [websiteId, setWebsiteId] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const [bulkOpen, setBulkOpen] = useState(false)
  const [selected, setSelected] = useState<number[]>([])
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${websiteId}-${resellerId}`,
    async () => {
      const [products, websites, resellers] = await Promise.all([
        listProducts({
          page,
          pageSize,
          query,
          websiteId: websiteId === "all" ? 0 : Number(websiteId),
          resellerId: session?.role === "master" && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        listWebsiteChoices(),
        session?.role === "master" ? listResellerChoices() : Promise.resolve({ items: [] as Reseller[] }),
      ])
      return { products, websites, resellers: resellers.items }
    },
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<ProductMap | null>(null)
  const [pendingDelete, setPendingDelete] = useState<ProductMap | null>(null)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    setSelected([])
  }, [query, websiteId, resellerId])

  const rows = data?.products.items ?? []
  const pageIds = rows.map((row) => row.id)
  const allChecked = pageIds.length > 0 && pageIds.every((id) => selected.includes(id))
  const selectedReseller = (data?.resellers ?? []).find((reseller) => String(reseller.id) === resellerId)
  const websites = (data?.websites ?? []).filter((website) => {
    if (session?.role !== "master" || resellerId === "all") return true
    return selectedReseller?.website_ids.includes(website.id) ?? false
  })

  const columns: DataColumn<ProductMap>[] = [
    {
      id: "select",
      header: "Select",
      headerCell: () => (
        <Checkbox
          aria-label="Select mappings on this page"
          checked={allChecked}
          onCheckedChange={(next) => {
            setSelected((current) => next
              ? [...new Set([...current, ...pageIds])]
              : current.filter((id) => !pageIds.includes(id)))
          }}
        />
      ),
      cell: (row) => (
        <Checkbox
          aria-label={`Select ${row.website_name}`}
          checked={selected.includes(row.id)}
          onCheckedChange={(next) => {
            setSelected((current) => next ? [...current, row.id] : current.filter((id) => id !== row.id))
          }}
        />
      ),
    },
    { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
    {
      id: "id",
      header: "Product ids",
      cell: (row) => (
        <div className="flex flex-wrap gap-2">
          {row.product_ids.map((id) => (
            <Badge key={id} variant="secondary">{id}</Badge>
          ))}
        </div>
      ),
      sortValue: (row) => row.product_ids.join(","),
    },
    { id: "name", header: "Product name", cell: (row) => row.product_name, sortValue: (row) => row.product_name },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-3">
          <Button variant="outline" size="sm" onClick={() => { setEditing(row); setOpen(true) }}>
            Edit
          </Button>
          <Button variant="destructive" size="sm" onClick={() => setPendingDelete(row)}>
            Delete
          </Button>
        </div>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Product Mapping"
        description="One website and product name can hold several aMember ids."
        action={
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" disabled={selected.length === 0} onClick={() => setBulkOpen(true)}>
              Update {selected.length === 0 ? "selected" : selected.length} ids
            </Button>
            <Button onClick={() => { setEditing(null); setOpen(true) }}>
              <PlusIcon data-icon="inline-start" />
              New mapping
            </Button>
          </div>
        }
      />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="product-search">Search</FieldLabel>
          <Input
            id="product-search"
            value={query}
            placeholder="Product id, name, or website"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="product-website">Website</FieldLabel>
          <ChoiceSelect
            id="product-website"
            value={websiteId}
            onValueChange={(value) => {
              setWebsiteId(value)
              setPage(1)
            }}
            items={[
              { label: "All websites", value: "all" },
              ...websites.map((website) => ({ label: websiteChoiceLabel(website), value: String(website.id) })),
            ]}
          />
        </Field>
        {session?.role === "master" ? (
          <Field className="md:w-64">
            <FieldLabel htmlFor="product-reseller">Reseller</FieldLabel>
            <ChoiceSelect
              id="product-reseller"
              value={resellerId}
              onValueChange={(value) => {
                setResellerId(value)
                setWebsiteId("all")
                setPage(1)
              }}
              items={[
                { label: "All resellers", value: "all" },
                ...(data?.resellers ?? []).map((reseller) => ({ label: reseller.username, value: String(reseller.id) })),
              ]}
            />
          </Field>
        ) : null}
      </div>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.products.page ?? page,
              total: data?.products.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${websiteId}-${resellerId}`}
            emptyTitle={query ? "No matches" : "No product mappings"}
            emptyDescription="Map a product id so billing can open the right website."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit mapping" : "New mapping"}
        description="Add one or more aMember product ids. Each id can belong to only one mapping."
      >
        {open ? (
          <ProductForm
            key={editing?.id ?? "new"}
            product={editing}
            websites={websites}
            onDone={async () => {
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <FormOverlay
        open={bulkOpen}
        onOpenChange={setBulkOpen}
        title="Update selected ids"
        description={`Adds or removes these ids on the ${selected.length} selected mappings. Other rows stay unchanged.`}
      >
        {bulkOpen ? (
          <BulkIdsForm
            key={selected.join(",")}
            mappingIds={selected}
            onDone={async () => {
              setBulkOpen(false)
              setSelected([])
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(next) => { if (!next) setPendingDelete(null) }}
        title="Delete mapping"
        description={pendingDelete ? `Delete ${pendingDelete.product_name}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteProduct(pendingDelete.id), "Mapping deleted").then((ok) => {
            setDeleting(false)
            if (ok) {
              setPendingDelete(null)
              reload()
            }
          })
        }}
      />
    </>
  )
}

function BulkIdsForm({
  mappingIds,
  onDone,
}: {
  mappingIds: number[]
  onDone: () => Promise<void>
}) {
  const [action, setAction] = useState<"add" | "remove">("add")
  const [text, setText] = useState("")
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)

  async function onSubmit(event: FormEvent) {
    event.preventDefault()
    const ids = splitProductIDs(text)
    if (mappingIds.length === 0) {
      setError("Select at least one mapping")
      return
    }
    if (ids.length === 0) {
      setError("Enter at least one product id")
      return
    }
    setPending(true)
    setError("")
    try {
      const result = await bulkProductIDs({
        action,
        mapping_ids: mappingIds,
        product_ids: ids,
      })
      if (result.updated === 0 && result.skipped.length === 0) {
        toast.error("No selected mappings were updated")
        return
      }
      if (result.updated > 0) toast.success(action === "add" ? `Added ids on ${result.updated} mappings` : `Removed ids on ${result.updated} mappings`)
      if (result.skipped.length > 0) toast.error(result.skipped.join(". "))
      if (result.updated > 0) await onDone()
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "Something went wrong")
    } finally {
      setPending(false)
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <FieldGroup>
        <Field>
          <FieldLabel>Action</FieldLabel>
          <ToggleGroup
            data-mode="choice"
            spacing={0}
            value={[action]}
            onValueChange={(values) => {
              const next = values[values.length - 1]
              if (next === "add" || next === "remove") setAction(next)
            }}
            aria-label="Add or remove product ids"
          >
            <ToggleGroupItem value="add">Add</ToggleGroupItem>
            <ToggleGroupItem value="remove">Remove</ToggleGroupItem>
          </ToggleGroup>
        </Field>
        <Field data-invalid={Boolean(error) || undefined}>
          <FieldLabel htmlFor="bulk-product-ids">Product ids</FieldLabel>
          <Textarea
            id="bulk-product-ids"
            className="font-mono"
            rows={5}
            value={text}
            placeholder={"123\n456, 789"}
            aria-invalid={Boolean(error) || undefined}
            onChange={(event) => setText(event.target.value)}
          />
          <p className="text-sm text-muted-foreground">Separate ids with a comma, space, or new line. A mapping is never left with zero ids.</p>
          {error ? <FieldError>{error}</FieldError> : null}
        </Field>
        <Button type="submit" disabled={pending}>
          {pending ? <Spinner data-icon="inline-start" /> : null}
          {action === "add" ? "Add ids" : "Remove ids"}
        </Button>
      </FieldGroup>
    </form>
  )
}

function ProductForm({
  product,
  websites,
  onDone,
}: {
  product: ProductMap | null
  websites: Website[]
  onDone: () => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      website_id: product ? String(product.website_id) : websites[0] ? String(websites[0].id) : "",
      product_name: product?.product_name ?? "",
    },
  })
  const [ids, setIds] = useState(product?.product_ids ?? [])
  const [draft, setDraft] = useState("")
  const [idError, setIdError] = useState("")

  function addId() {
    const value = draft.trim()
    if (!value) {
      setIdError("Enter a product id")
      return
    }
    if (ids.includes(value)) {
      setIdError("That product id is already on this mapping")
      return
    }
    setIds((current) => [...current, value])
    setDraft("")
    setIdError("")
  }

  async function onSubmit(values: FormValues) {
    if (ids.length === 0) {
      setIdError("Add at least one product id")
      return
    }
    const ok = await runMutation(
      () =>
        saveProduct({
          id: product?.id,
          website_id: Number(values.website_id),
          product_ids: ids,
          product_name: values.product_name,
        }),
      product ? "Mapping updated" : "Mapping saved"
    )
    if (ok) await onDone()
  }

  const errors = form.formState.errors

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.website_id) || undefined}>
          <FieldLabel htmlFor="website_id">Website</FieldLabel>
          <Controller
            control={form.control}
            name="website_id"
            render={({ field }) => (
              <ChoiceSelect
                id="website_id"
                value={field.value}
                onValueChange={field.onChange}
                items={websites.map((website) => ({ label: websiteChoiceLabel(website), value: String(website.id) }))}
              />
            )}
          />
          <FieldError errors={[errors.website_id]} />
        </Field>
        <Field data-invalid={Boolean(idError) || undefined}>
          <FieldLabel htmlFor="product_id">Product ids</FieldLabel>
          <div className="flex flex-wrap gap-2">
            {ids.map((id) => (
              <Badge key={id} variant="secondary">
                {id}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`Remove ${id}`}
                  onClick={() => setIds((current) => current.filter((item) => item !== id))}
                >
                  <XIcon />
                </Button>
              </Badge>
            ))}
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Input
              id="product_id"
              className="font-mono md:max-w-xs"
              value={draft}
              aria-invalid={Boolean(idError) || undefined}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault()
                  addId()
                }
              }}
            />
            <Button type="button" variant="outline" onClick={addId}>
              Add
            </Button>
          </div>
          {idError ? <FieldError>{idError}</FieldError> : null}
        </Field>
        <Field data-invalid={Boolean(errors.product_name) || undefined}>
          <FieldLabel htmlFor="product_name">Product name</FieldLabel>
          <Input id="product_name" aria-invalid={Boolean(errors.product_name) || undefined} {...form.register("product_name")} />
          <FieldError errors={[errors.product_name]} />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save mapping
        </Button>
      </FieldGroup>
    </form>
  )
}
