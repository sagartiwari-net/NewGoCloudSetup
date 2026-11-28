"use client"

import { useMemo, useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import {
  currentOperator,
  downloadAhrefsBarExtension,
  downloadAhrefsBarTMCheck,
  getAhrefsBarExtInfo,
  listResellerChoices,
} from "@/lib/api/client"
import { runMutation } from "@/lib/mutate"

export default function AhrefsBarDownloadPage() {
  const { session } = useAuth()
  const isMaster = session?.role === "master"
  const [resellerId, setResellerId] = useState(0)

  const { data: resellerChoices } = useResource(isMaster ? "ab-dl-reseller-choices" : "skip", async () => {
    if (!isMaster) {
      return { me: null as null | { id: number; username: string }, items: [] as Array<{ id: number; username: string }> }
    }
    const [me, page] = await Promise.all([currentOperator(), listResellerChoices()])
    return {
      me: { id: me.id, username: me.username },
      items: page.items.filter((r) => r.ahrefs_bar_enabled).map((r) => ({ id: r.id, username: r.username })),
    }
  })

  const resellerItems = useMemo(() => {
    const items: Array<{ label: string; value: string }> = [{ label: "Choose reseller…", value: "0" }]
    if (resellerChoices?.me) {
      items.push({ label: `Master (${resellerChoices.me.username})`, value: String(resellerChoices.me.id) })
    }
    for (const r of resellerChoices?.items ?? []) {
      items.push({ label: r.username, value: String(r.id) })
    }
    return items
  }, [resellerChoices])

  const effectiveResellerId = isMaster ? resellerId : 0
  const canLoad = !isMaster || resellerId > 0
  const { data, loading } = useResource(
    `${session?.role ?? "none"}-ext-${effectiveResellerId}`,
    () => (canLoad ? getAhrefsBarExtInfo(isMaster ? resellerId : 0) : Promise.resolve(null)),
  )
  const [busyZip, setBusyZip] = useState(false)
  const [busyCheck, setBusyCheck] = useState(false)

  return (
    <>
      <PageHeader
        title="Ahrefs Bar download"
        description="Branded extension zip (one reseller + their portals only) and tm-check.php for aMember."
      />
      {isMaster ? (
        <Field className="max-w-md">
          <FieldLabel htmlFor="ab-dl-reseller">Reseller for this zip</FieldLabel>
          <ChoiceSelect
            id="ab-dl-reseller"
            value={String(resellerId)}
            onValueChange={(v) => setResellerId(Number(v) || 0)}
            items={resellerItems}
          />
          <FieldDescription>
            Required. Zip stamps that reseller id and only their enabled portal hosts — another reseller&apos;s aMember will not unlock it.
          </FieldDescription>
        </Field>
      ) : null}
      {isMaster && !canLoad ? (
        <p className="text-sm text-muted-foreground">Pick a reseller to see bound portals and download their extension.</p>
      ) : loading && !data ? (
        <div className="flex justify-center py-16"><Spinner /></div>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Extension zip</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
                <li>Add portals first (bind aMember host to this reseller).</li>
                <li>Download zip → unzip (manifest.json should be at the top of that folder).</li>
                <li>
                  Chrome → Load unpacked → choose that unzipped folder.
                </li>
                <li>
                  Prod API is <span className="font-mono">barapi.upays.in</span>. Local test needs{" "}
                  <span className="font-mono">127.0.0.1:8088</span> stamped instead.
                </li>
              </ol>
              <dl className="space-y-2 text-sm">
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Public API</dt>
                  <dd className="font-mono text-xs">{data?.public_api || "—"}</dd>
                </div>
                {data?.reseller_name || data?.reseller_id ? (
                  <div className="flex justify-between gap-4">
                    <dt className="text-muted-foreground">Stamped for</dt>
                    <dd className="text-right text-xs">
                      {data.reseller_name || "reseller"}{" "}
                      <span className="font-mono text-muted-foreground">(id {data.reseller_id})</span>
                    </dd>
                  </div>
                ) : null}
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Source</dt>
                  <dd className="text-right text-xs">{data?.extension_ok ? "ready" : "missing AHREFS_BAR_EXT_DIR"}</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Bound portals</dt>
                  <dd>{data?.portals?.length ?? 0}</dd>
                </div>
              </dl>
              {(data?.portals ?? []).length > 0 ? (
                <ul className="list-inside list-disc text-xs text-muted-foreground">
                  {data!.portals.map((p) => (
                    <li key={p} className="font-mono">{p}</li>
                  ))}
                </ul>
              ) : (
                <p className="text-xs text-muted-foreground">No portals for this reseller — zip host permissions will only include Ahrefs + API.</p>
              )}
              <Button
                disabled={!data?.extension_ok || busyZip || (isMaster && resellerId <= 0)}
                onClick={() => {
                  setBusyZip(true)
                  void runMutation(
                    () => downloadAhrefsBarExtension(isMaster ? resellerId : 0),
                    "Extension zip ready",
                  ).finally(() => setBusyZip(false))
                }}
              >
                {busyZip ? <Spinner data-icon="inline-start" /> : null}
                Download extension.zip
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>tm-check.php</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
                <li>Download tm-check.php.</li>
                <li>
                  Upload into the <span className="font-medium text-foreground">aMember root</span> (folder that has{" "}
                  <span className="font-mono">library/Am/Lite.php</span>) — often{" "}
                  <span className="font-mono">/member/tm-check.php</span>, not only the domain web root.
                </li>
                <li>Portals → set that full tm-check URL (+ cookie name, usually <span className="font-mono">amember_nr</span> or <span className="font-mono">PHPSESSID</span>).</li>
              </ol>
              <p className="text-xs text-muted-foreground">
                Template {data?.template_ok ? "ready" : "missing — set AHREFS_BAR_TEMPLATE_DIR"}
              </p>
              <p className="text-xs text-muted-foreground">
                Logged-out browser open should return <span className="font-mono">not_logged_in</span>.{" "}
                <span className="font-mono">check_failed</span> / <span className="font-mono">amember_lite_missing</span> means wrong install path.
              </p>
              <Button
                variant="outline"
                disabled={!data?.template_ok || busyCheck}
                onClick={() => {
                  setBusyCheck(true)
                  void runMutation(() => downloadAhrefsBarTMCheck(), "tm-check.php ready").finally(() => setBusyCheck(false))
                }}
              >
                {busyCheck ? <Spinner data-icon="inline-start" /> : null}
                Download tm-check.php
              </Button>
            </CardContent>
          </Card>
        </div>
      )}
    </>
  )
}
