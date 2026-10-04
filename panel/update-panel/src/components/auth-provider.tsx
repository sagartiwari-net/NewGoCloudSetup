"use client"

import { createContext, useContext, useMemo, useSyncExternalStore } from "react"

import {
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

function snapshotSession() {
  const raw = readSessionRaw()
  if (raw !== cachedRaw) {
    cachedRaw = raw
    cachedSession = readSession()
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
        await login(username, password)
        emitSession()
      },
      signOut() {
        void logout().finally(() => emitSession())
      },
      setRole(role) {
        if (!session) return
        writeSession({ ...session, role, token: session.token })
        emitSession()
      },
    }),
    [ready, session]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error("useAuth must be used inside AuthProvider")
  return value
}
