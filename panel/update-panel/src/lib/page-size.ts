"use client"

import { useState, useSyncExternalStore } from "react"

const KEY = "update-panel-page-size"
const EVENT = "update-panel-page-size"

export type PageSize = 10 | 20 | 50

export function parsePageSize(value: string | null): PageSize {
  const size = Number(value)
  if (size === 10 || size === 20 || size === 50) return size
  return 10
}

function subscribe(onStoreChange: () => void) {
  window.addEventListener(EVENT, onStoreChange)
  window.addEventListener("storage", onStoreChange)
  return () => {
    window.removeEventListener(EVENT, onStoreChange)
    window.removeEventListener("storage", onStoreChange)
  }
}

function snapshot() {
  return parsePageSize(window.localStorage.getItem(KEY))
}

export function setPageSize(next: PageSize) {
  window.localStorage.setItem(KEY, String(next))
  window.dispatchEvent(new Event(EVENT))
}

export function usePageSize() {
  return useSyncExternalStore(subscribe, snapshot, () => 10 as PageSize)
}

export function usePageIndex(pageSize: number) {
  const [page, setPage] = useState(1)
  const [seen, setSeen] = useState(pageSize)
  if (seen !== pageSize) {
    setSeen(pageSize)
    setPage(1)
  }
  return [page, setPage] as const
}

export function useListPage() {
  const pageSize = usePageSize()
  const [page, setPage] = usePageIndex(pageSize)
  return { page, setPage, pageSize, setPageSize }
}
