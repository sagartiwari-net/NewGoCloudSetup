"use client"

import { useEffect, useRef } from "react"
import { useTheme } from "next-themes"
import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import {
  ActivityIcon,
  BanIcon,
  MapPinIcon,
  BotIcon,
  ChartColumnIcon,
  GlobeIcon,
  KeyRoundIcon,
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
  ShieldIcon,
  StoreIcon,
  GaugeIcon,
  UserIcon,
  UsersIcon,
} from "lucide-react"

import { useAuth } from "@/components/auth-provider"
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

const masterRoutes = ["/proxies", "/user-agents", "/websites", "/resellers", "/automate", "/settings"]

const nav: Array<{
  label: string
  items: Array<{
    href: string
    label: string
    icon: React.ComponentType
    masterOnly?: boolean
  }>
}> = [
  {
    label: "Operations",
    items: [
      { href: "/", label: "Overview", icon: LayoutDashboardIcon },
      { href: "/access", label: "Access", icon: LogInIcon },
      { href: "/accounts", label: "Mapped Accounts", icon: KeyRoundIcon },
      { href: "/limits", label: "Limits", icon: GaugeIcon },
      { href: "/sessions", label: "Active Logins", icon: ActivityIcon },
      { href: "/usage", label: "Quota Logs", icon: ScrollTextIcon },
      { href: "/analytics", label: "Analytics", icon: ChartColumnIcon },
      { href: "/telegram", label: "Telegram", icon: SendIcon },
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
      { href: "/host-reports", label: "Host reports", icon: MapPinIcon },
      { href: "/settings", label: "Settings", icon: SettingsIcon, masterOnly: true },
    ],
  },
  {
    label: "Catalog",
    items: [
      { href: "/proxies", label: "Proxy Manager", icon: ServerIcon, masterOnly: true },
      { href: "/user-agents", label: "User Agents", icon: BotIcon, masterOnly: true },
      { href: "/websites", label: "Website Domains", icon: GlobeIcon, masterOnly: true },
      { href: "/resellers", label: "Resellers", icon: StoreIcon, masterOnly: true },
    ],
  },
]

function isActive(pathname: string, href: string) {
  if (href === "/") return pathname === "/"
  return pathname === href || pathname.startsWith(`${href}/`)
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { session, ready } = useAuth()
  const pathname = usePathname()
  const router = useRouter()

  useEffect(() => {
    if (!ready) return
    if (!session) {
      router.replace("/login")
      return
    }
    if (session.role === "reseller" && masterRoutes.includes(pathname)) {
      router.replace("/")
    }
  }, [ready, session, pathname, router])

  if (!ready || !session) {
    return <div className="min-h-svh bg-background" />
  }

  return (
    <ShellFrame role={session.role} username={session.username}>
      {children}
    </ShellFrame>
  )
}

function ShellFrame({
  children,
  role,
  username,
}: {
  children: React.ReactNode
  role: Role
  username: string
}) {
  const { signOut, setRole } = useAuth()
  const { theme, setTheme } = useTheme()
  const pathname = usePathname()
  const router = useRouter()
  const motionRef = useRef<HTMLDivElement>(null)

  useGSAP(
    () => {
      if (prefersReducedMotion()) return
      gsap.from("[data-page]", {
        autoAlpha: 0,
        y: 16,
        duration: 0.35,
        ease: "power2.out",
      })
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
            const items = group.items.filter((item) => role === "master" || !item.masterOnly)
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
                            render={<Link href={item.href} />}
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
                render={<Link href="/profile" />}
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
              <DropdownMenuSeparator />
              {role === "master" ? (
                <DropdownMenuItem render={<Link href="/settings" />}>
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
