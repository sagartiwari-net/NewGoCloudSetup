"use client"

import { createContext, useContext, useEffect, useMemo, useSyncExternalStore } from "react"

import {
  clearCatalogCache,
  currentOperator,
  login,
  logout,
  readSession,
  readSessionRaw,
  writeSession,
  type Session,
} from "@/lib/api/client"
import type { Role } from "@/lib/api/types"

type AuthValue = {
  session: Session | null
  ready: boolean
  signIn: (username: string, password: string) => Promise<void>
  signOut: () => void
  setRole: (role: Role) => void
}

const AuthContext = createContext<AuthValue | null>(null)
const SESSION_EVENT = "update-panel-session"

let cachedRaw = ""
let cachedSession: Session | null = null

function presentSession(session: Session | null): Session | null {
  if (!session) return null
  if (session.accountRole === "master") return session
  return { ...session, role: "reseller" }
}

function snapshotSession() {
  const raw = readSessionRaw()
  if (raw !== cachedRaw) {
    cachedRaw = raw
    cachedSession = presentSession(readSession())
  }
  return cachedSession
}

function emitSession() {
  cachedRaw = ""
  window.dispatchEvent(new Event(SESSION_EVENT))
}

function subscribeSession(callback: () => void) {
  window.addEventListener(SESSION_EVENT, callback)
  window.addEventListener("storage", callback)
  return () => {
    window.removeEventListener(SESSION_EVENT, callback)
    window.removeEventListener("storage", callback)
  }
}

function useReady() {
  return useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false
  )
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const session = useSyncExternalStore(subscribeSession, snapshotSession, () => null)
  const ready = useReady()

  const value = useMemo<AuthValue>(
    () => ({
      session,
      ready,
      async signIn(username, password) {
        clearCatalogCache()
        await login(username, password)
        emitSession()
      },
      signOut() {
        clearCatalogCache()
        void logout().finally(() => emitSession())
      },
      setRole(role) {
        if (!session || session.accountRole !== "master") return
        writeSession({ ...session, role, accountRole: "master", token: session.token })
        emitSession()
      },
    }),
    [ready, session]
  )

  useEffect(() => {
    const current = readSession()
    if (!current?.username) return
    let cancelled = false
    currentOperator()
      .then((operator) => {
        if (cancelled) return
        const stored = readSession()
        if (!stored || stored.username !== current.username) return
        const accountRole = operator.role === "master" ? "master" : "reseller"
        const role = accountRole === "master" ? stored.role : "reseller"
        if (stored.username === operator.username && stored.role === role && stored.accountRole === accountRole && !stored.token) return
        writeSession({ username: operator.username, role, accountRole, token: "" })
        emitSession()
      })
      .catch(() => {
        const stored = readSession()
        if (!stored || stored.accountRole !== "master") return
        writeSession({ ...stored, role: "reseller", accountRole: "reseller", token: "" })
        emitSession()
      })
    return () => {
      cancelled = true
    }
  }, [session?.username])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error("useAuth must be used inside AuthProvider")
  return value
}
