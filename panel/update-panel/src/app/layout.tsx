import type { Metadata } from "next"
import { Geist, Geist_Mono } from "next/font/google"

import { Providers } from "@/components/providers"
import "./globals.css"

// The panel is a login app. A year-long static cache kept the previous
// HTML, which then asked for chunk files the new build had deleted.
export const dynamic = "force-dynamic"

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
})

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
})

export const metadata: Metadata = {
  title: "ToolsMandi Update Panel",
  description: "Operator panel for tool proxies, quotas, and cookie updates.",
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <body className="min-h-svh bg-background text-foreground" suppressHydrationWarning>
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){var key="panel-static-reload";function recover(){try{if(sessionStorage.getItem(key)==="1")return;sessionStorage.setItem(key,"1")}catch(e){return}var url=new URL(location.href);url.searchParams.set("__v",String(Date.now()));location.replace(url.toString())}window.addEventListener("error",function(event){var el=event.target;if(!el||el===window)return;var src=el.src||el.href||"";if(src.indexOf("/_next/static/")===-1)return;recover()},true);window.addEventListener("pageshow",function(event){if(event.persisted)recover()})})();`,
          }}
        />
        <Providers>{children}</Providers>
      </body>
    </html>
  )
}
