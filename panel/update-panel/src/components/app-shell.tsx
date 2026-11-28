"use client"

import { useEffect, useRef, useState } from "react"
import { useTheme } from "next-themes"
import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import {
  ActivityIcon,
  BanIcon,
  MapPinIcon,
  BarChart3Icon,
  BotIcon,
  ChartColumnIcon,
  CalendarClockIcon,
  CloudIcon,
  DatabaseIcon,
  FileTextIcon,
  GlobeIcon,
  KeyRoundIcon,
  Link2Icon,
  LockKeyholeIcon,
  LogInIcon,
  LayoutDashboardIcon,
  LogOutIcon,
  SendIcon,
  SettingsIcon,
  PackageIcon,
  RefreshCwIcon,
  ScrollTextIcon,
  ServerIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  ClipboardListIcon,
  ShieldIcon,
  StoreIcon,
  GaugeIcon,
  UserIcon,
  UsersIcon,
} from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { readSession } from "@/lib/api/client"
import { gsap, prefersReducedMotion, useGSAP } from "@/lib/gsap"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import type { Role } from "@/lib/api/types"

const masterRoutes = [
  "/recloud",
  "/resellers",
  "/automate",
  "/settings",
  "/access-sites",
  "/telegram",
  "/shared-cookies",
  "/automation/credentials",
  "/automation/jobs",
  "/automation/storage",
  "/automation/logs",
]

const nav: Array<{
  label: string
  items: Array<{
    href: string
    label: string
    icon: React.ComponentType
    masterOnly?: boolean
    /** Only master + resellers with Ahrefs Bar access */
    ahrefsBarOnly?: boolean
  }>
}> = [
  {
    label: "Operations",
    items: [
      { href: "/", label: "Overview", icon: LayoutDashboardIcon },
      { href: "/access", label: "Access", icon: LogInIcon },
      { href: "/accounts", label: "Mapped Accounts", icon: KeyRoundIcon },
      { href: "/shared-cookies", label: "Shared cookies", icon: Link2Icon, masterOnly: true },
      { href: "/recloud", label: "Recloud", icon: CloudIcon, masterOnly: true },
      { href: "/limits", label: "Limits", icon: GaugeIcon },
      { href: "/sessions", label: "Active Logins", icon: ActivityIcon },
      { href: "/usage", label: "Quota Logs", icon: ScrollTextIcon },
      { href: "/analytics", label: "Analytics", icon: ChartColumnIcon },
      { href: "/telegram", label: "Telegram", icon: SendIcon, masterOnly: true },
      { href: "/automate", label: "Automate Task", icon: RefreshCwIcon, masterOnly: true },
      { href: "/products", label: "Product Mapping", icon: PackageIcon },
    ],
  },
  {
    label: "Safety",
    items: [
      { href: "/users", label: "Users", icon: UsersIcon },
      { href: "/violations", label: "Violations", icon: ShieldAlertIcon },
      { href: "/security", label: "Security", icon: ShieldIcon },
      { href: "/blocked-ips", label: "Blocked IPs", icon: BanIcon },
      { href: "/access-sites", label: "Allowed sites", icon: ShieldCheckIcon, masterOnly: true },
      { href: "/access-rejects", label: "Access rejects", icon: ClipboardListIcon },
      { href: "/host-reports", label: "Host reports", icon: MapPinIcon },
      { href: "/settings", label: "Settings", icon: SettingsIcon, masterOnly: true },
    ],
  },
  {
    label: "Catalog",
    items: [
      { href: "/proxies", label: "Proxy Manager", icon: ServerIcon },
      { href: "/user-agents", label: "User Agents", icon: BotIcon },
      { href: "/websites", label: "Website Domains", icon: GlobeIcon },
      { href: "/resellers", label: "Resellers", icon: StoreIcon, masterOnly: true },
    ],
  },
  {
    label: "Ahrefs Bar",
    items: [
      { href: "/ahrefs-bar", label: "Overview", icon: BarChart3Icon, ahrefsBarOnly: true },
      { href: "/ahrefs-bar/accounts", label: "Accounts", icon: KeyRoundIcon, ahrefsBarOnly: true },
      { href: "/ahrefs-bar/portals", label: "Portals", icon: GlobeIcon, ahrefsBarOnly: true },
      { href: "/ahrefs-bar/defaults", label: "Limits", icon: GaugeIcon, ahrefsBarOnly: true },
      { href: "/ahrefs-bar/users", label: "User limits", icon: UsersIcon, ahrefsBarOnly: true },
      { href: "/ahrefs-bar/usage", label: "Usage", icon: ScrollTextIcon, ahrefsBarOnly: true },
      { href: "/ahrefs-bar/download", label: "Download", icon: PackageIcon, ahrefsBarOnly: true },
    ],
  },
  {
    label: "Automation",
    items: [
      { href: "/automation/credentials", label: "Credentials", icon: LockKeyholeIcon, masterOnly: true },
      { href: "/automation/jobs", label: "Jobs", icon: CalendarClockIcon, masterOnly: true },
      { href: "/automation/storage", label: "Storage", icon: DatabaseIcon, masterOnly: true },
      { href: "/automation/logs", label: "Logs", icon: FileTextIcon, masterOnly: true },
    ],
  },
]

function isActive(pathname: string, href: string) {
  if (href === "/" || href === "/ahrefs-bar") return pathname === href
  return pathname === href || pathname.startsWith(`${href}/`)
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { session, ready } = useAuth()
  const pathname = usePathname()
  const router = useRouter()

  useEffect(() => {
    if (!ready) return
    if (!session && !readSession()) router.replace("/login")
  }, [ready, session, router])

  useEffect(() => {
    if (session?.role === "reseller" && masterRoutes.includes(pathname)) {
      router.replace("/")
    }
  }, [session, pathname, router])

  useEffect(() => {
    if (!session) return
    const onBar = pathname === "/ahrefs-bar" || pathname.startsWith("/ahrefs-bar/")
    if (!onBar) return
    const allowed = session.role === "master" || session.accountRole === "master" || !!session.ahrefs_bar_enabled
    if (!allowed) router.replace("/")
  }, [session, pathname, router])

  if (!ready || !session) {
    const signedOut = ready && !session
    return (
      <div className="flex min-h-svh flex-col items-center justify-center gap-3 bg-background p-6 text-center">
        <p className="text-sm text-muted-foreground">{signedOut ? "Redirecting to sign in…" : "Loading panel…"}</p>
        {signedOut ? (
          <Link href="/login" className="text-sm font-medium text-primary underline underline-offset-4">
            Open sign in
          </Link>
        ) : null}
      </div>
    )
  }

  return (
    <ShellFrame
      role={session.role}
      accountRole={session.accountRole}
      username={session.username}
      ahrefsBarEnabled={session.ahrefs_bar_enabled}
    >
      {children}
    </ShellFrame>
  )
}

function ShellFrame({
  children,
  role,
  accountRole,
  username,
  ahrefsBarEnabled,
}: {
  children: React.ReactNode
  role: Role
  accountRole?: Role
  username: string
  ahrefsBarEnabled?: boolean
}) {
  const { signOut, setRole } = useAuth()
  const { theme, setTheme } = useTheme()
  const pathname = usePathname()
  const router = useRouter()
  const motionRef = useRef<HTMLDivElement>(null)

  useGSAP(
    () => {
      const page = motionRef.current?.querySelector("[data-page]")
      if (!(page instanceof HTMLElement)) return
      if (prefersReducedMotion()) {
        gsap.set(page, { autoAlpha: 1, y: 0 })
        return
      }
      gsap.fromTo(
        page,
        { autoAlpha: 0, y: 16 },
        { autoAlpha: 1, y: 0, duration: 0.35, ease: "power2.out", overwrite: true }
      )
    },
    { scope: motionRef, dependencies: [pathname], revertOnUpdate: true }
  )

  const initial = username.slice(0, 1).toUpperCase()

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <div className="flex flex-col gap-0.5 px-2 py-1">
            <span className="text-sm font-medium">ToolsMandi</span>
            <span className="text-sm text-muted-foreground">Update Panel</span>
          </div>
        </SidebarHeader>
        <SidebarContent>
          {nav.map((group) => {
            const barOk = role === "master" || accountRole === "master" || !!ahrefsBarEnabled
            const items = group.items.filter((item) => {
              if (item.masterOnly && role !== "master") return false
              if (item.ahrefsBarOnly && !barOk) return false
              return true
            })
            if (items.length === 0) return null
            return (
              <SidebarGroup key={group.label}>
                <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {items.map((item) => {
                      const Icon = item.icon
                      return (
                        <SidebarMenuItem key={item.href}>
                          <SidebarMenuButton
                            isActive={isActive(pathname, item.href)}
                            tooltip={item.label}
                            render={<Link href={item.href} prefetch={false} />}
                          >
                            <Icon />
                            <span>{item.label}</span>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      )
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            )
          })}
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={pathname === "/profile"}
                tooltip="Profile"
                render={<Link href="/profile" prefetch={false} />}
              >
                <UserIcon />
                <span>Profile</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <header className="sticky top-0 z-20 flex w-full shrink-0 items-center justify-between gap-3 border-b bg-background px-4 py-3">
          <SidebarTrigger className="min-h-10 min-w-10 md:min-h-8 md:min-w-8" />
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" className="gap-2 px-2" />}
              aria-label={`Account menu for ${username}`}
            >
              <Avatar>
                <AvatarFallback>{initial}</AvatarFallback>
              </Avatar>
              <span className="hidden sm:inline">{username}</span>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-64">
              <DropdownMenuGroup>
                <DropdownMenuLabel>Theme</DropdownMenuLabel>
                <div className="px-1.5 py-1">
                  <ToggleGroup
                    data-mode="choice"
                    spacing={0}
                    value={[theme === "dark" ? "dark" : "light"]}
                    onValueChange={(values) => {
                      const next = values[values.length - 1]
                      if (next === "light" || next === "dark") setTheme(next)
                    }}
                    aria-label="Color theme"
                  >
                    <ToggleGroupItem value="light">Light</ToggleGroupItem>
                    <ToggleGroupItem value="dark">Dark</ToggleGroupItem>
                  </ToggleGroup>
                </div>
              </DropdownMenuGroup>
              {accountRole === "master" ? (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuGroup>
                    <DropdownMenuLabel>View as</DropdownMenuLabel>
                    <div className="px-1.5 py-1">
                      <ToggleGroup
                        data-mode="choice"
                        spacing={0}
                        value={[role]}
                        onValueChange={(values) => {
                          const next = values[values.length - 1]
                          if (next === "master" || next === "reseller") setRole(next as Role)
                        }}
                        aria-label="View as role"
                      >
                        <ToggleGroupItem value="master">Master</ToggleGroupItem>
                        <ToggleGroupItem value="reseller">Reseller</ToggleGroupItem>
                      </ToggleGroup>
                    </div>
                  </DropdownMenuGroup>
                </>
              ) : null}
              <DropdownMenuSeparator />
              {role === "master" ? (
                <DropdownMenuItem render={<Link href="/settings" prefetch={false} />}>
                  <SettingsIcon />
                  Settings
                </DropdownMenuItem>
              ) : null}
              <DropdownMenuItem
                onClick={() => {
                  signOut()
                  router.replace("/login")
                }}
              >
                <LogOutIcon />
                Log out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>
        <div ref={motionRef} className="flex flex-1 flex-col">
          <div data-page className="flex flex-1 flex-col gap-4 p-4">
            {children}
          </div>
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
