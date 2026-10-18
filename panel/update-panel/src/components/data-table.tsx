"use client"

import { useEffect, useMemo, useState } from "react"
import {
  flexRender,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table"
import { InboxIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { setPageSize } from "@/lib/page-size"
import { cn } from "@/lib/utils"
import { Card, CardContent } from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export type DataColumn<T> = {
  id: string
  header: string
  headerCell?: () => React.ReactElement
  cell: (row: T) => React.ReactNode
  sortValue?: (row: T) => string | number
}

const MOBILE_FULL_WIDTH = new Set([
  "actions",
  "ua",
  "path",
  "query",
  "details",
  "description",
  "url",
  "meters",
  "logs",
])

function mobileFieldClass(columnId: string) {
  if (MOBILE_FULL_WIDTH.has(columnId)) return "basis-full"
  return "min-w-[8.5rem] flex-[1_1_8.5rem]"
}

export function DataTable<T>({
  rows,
  columns,
  rowKey,
  loading,
  emptyTitle,
  emptyDescription,
  resetKey = "",
  paging,
}: {
  rows: T[]
  columns: DataColumn<T>[]
  rowKey: (row: T) => string | number
  loading?: boolean
  emptyTitle: string
  emptyDescription: string
  resetKey?: string
  paging?: {
    page: number
    total: number
    pageSize: number
    onPageChange: (page: number) => void
  }
}) {
  const [sorting, setSorting] = useState<SortingState>([])
  const [pageIndex, setPageIndex] = useState(0)
  const pageSize = paging ? Math.max(rows.length, 1) : Math.max(rows.length, 1)

  useEffect(() => {
    setPageIndex(0)
  }, [resetKey])

  const tableColumns = useMemo<ColumnDef<T>[]>(
    () =>
      columns.map((column) => ({
        id: column.id,
        accessorFn: (row) => column.sortValue?.(row) ?? "",
        header: column.headerCell ?? column.header,
        cell: ({ row }) => column.cell(row.original),
        enableSorting: Boolean(column.sortValue),
      })),
    [columns]
  )

  const table = useReactTable({
    data: rows,
    columns: tableColumns,
    state: { sorting, pagination: { pageIndex, pageSize } },
    onSortingChange: setSorting,
    onPaginationChange: (updater) => {
      const next = typeof updater === "function" ? updater({ pageIndex, pageSize }) : updater
      setPageIndex(next.pageIndex)
    },
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getRowId: (row) => String(rowKey(row)),
  })

  if (loading) {
    return (
      <div className="flex flex-col gap-2">
        {Array.from({ length: 6 }, (_, index) => (
          <Skeleton key={index} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  const pageRows = paging ? table.getSortedRowModel().rows : table.getRowModel().rows
  const pageCount = paging ? Math.max(1, Math.ceil(paging.total / paging.pageSize)) : table.getPageCount()
  const currentPage = paging ? paging.page : pageIndex + 1
  const pager = (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <p className="text-sm text-muted-foreground">
        Page {currentPage} of {pageCount}
      </p>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        {paging ? (
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Rows</span>
            <ToggleGroup
              data-mode="choice"
              spacing={0}
              value={[String(paging.pageSize)]}
              onValueChange={(values) => {
                const next = Number(values[values.length - 1])
                if (next === 10 || next === 20 || next === 50) setPageSize(next)
              }}
              aria-label="Rows per page"
            >
              <ToggleGroupItem value="10">10</ToggleGroupItem>
              <ToggleGroupItem value="20">20</ToggleGroupItem>
              <ToggleGroupItem value="50">50</ToggleGroupItem>
            </ToggleGroup>
          </div>
        ) : null}
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={paging ? paging.page <= 1 : !table.getCanPreviousPage()}
            onClick={() => (paging ? paging.onPageChange(paging.page - 1) : table.previousPage())}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            disabled={paging ? paging.page >= pageCount : !table.getCanNextPage()}
            onClick={() => (paging ? paging.onPageChange(paging.page + 1) : table.nextPage())}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  )

  if (rows.length === 0) {
    return (
      <div className="flex flex-col gap-3">
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <InboxIcon />
            </EmptyMedia>
            <EmptyTitle>{emptyTitle}</EmptyTitle>
            <EmptyDescription>{emptyDescription}</EmptyDescription>
          </EmptyHeader>
        </Empty>
        {paging ? pager : null}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="hidden md:block">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header) => {
                  const column = columns.find((item) => item.id === header.column.id)
                  const sortable = Boolean(column?.sortValue)
                  return (
                    <TableHead key={header.id}>
                      {sortable ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={header.column.getToggleSortingHandler()}
                        >
                          {flexRender(header.column.columnDef.header, header.getContext())}
                        </Button>
                      ) : (
                        flexRender(header.column.columnDef.header, header.getContext())
                      )}
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {pageRows.map((row) => (
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <div className="flex flex-col gap-3 md:hidden">
        {pageRows.map((row) => (
          <Card key={row.id}>
            <CardContent className="flex flex-wrap gap-x-4 gap-y-2.5">
              {columns.map((column) => (
                <div key={column.id} className={cn("flex min-w-0 max-w-full flex-col gap-0.5", mobileFieldClass(column.id))}>
                  <span className="text-xs text-muted-foreground">{column.header}</span>
                  <div className="text-sm break-words">{column.cell(row.original)}</div>
                </div>
              ))}
            </CardContent>
          </Card>
        ))}
      </div>

      {pager}
    </div>
  )
}
