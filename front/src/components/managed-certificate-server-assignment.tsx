import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { ManagedCertificate } from "@/lib/types"
import { certQueryKeys } from "@/lib/certificate-ui"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface ManagedCertificateServerAssignmentProps {
  certificate: ManagedCertificate
  readOnly?: boolean
  variant?: "card" | "flat"
}

export function ManagedCertificateServerAssignment({
  certificate,
  readOnly,
  variant = "card",
}: ManagedCertificateServerAssignmentProps) {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<string[]>(certificate.server_ids ?? [])

  useEffect(() => {
    setSelected(certificate.server_ids ?? [])
  }, [certificate.server_ids, certificate.id])

  const { data: serverData, isLoading } = useQuery({
    queryKey: ["servers"],
    queryFn: () => api.listServers({ page_size: 200 }),
  })
  const servers = serverData?.servers ?? []

  const mutation = useMutation({
    mutationFn: () =>
      api.updateManagedCertificate(certificate.id, {
        name: certificate.name,
        domains: certificate.domains,
        certificate_issuer_id: certificate.certificate_issuer_id,
        dns_provider_account_id: certificate.dns_provider_account_id,
        key_type: certificate.key_type,
        auto_renew_enabled: certificate.auto_renew_enabled,
        renew_before_days: certificate.renew_before_days,
        notify_group_ids: certificate.notify_group_ids ?? [],
        server_ids: selected,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: certQueryKeys.detail(certificate.id),
      })
      queryClient.invalidateQueries({ queryKey: certQueryKeys.list })
      toast.success("Server 分配已更新")
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "保存 Server 分配失败")
    },
  })

  const dirty =
    selected.length !== (certificate.server_ids?.length ?? 0) ||
    selected.some((id) => !(certificate.server_ids ?? []).includes(id))

  const toggle = (id: string) => {
    setSelected((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    )
  }

  const body = (
    <>
      {isLoading ? (
        <p className="text-muted-foreground text-sm">加载 Server 列表…</p>
      ) : servers.length === 0 ? (
        <p className="text-muted-foreground text-sm">暂无 Server，可先完成签发再分配。</p>
      ) : readOnly ? (
        <ul className="text-sm">
          {selected.length === 0 ? (
            <li className="text-muted-foreground">（无）</li>
          ) : (
            selected.map((sid) => {
              const s = servers.find((x) => x.id === sid)
              return (
                <li key={sid}>
                  {s ? (
                    <Link to={`/servers/${sid}`} className="hover:underline">
                      {s.name}
                    </Link>
                  ) : (
                    sid
                  )}
                </li>
              )
            })
          )}
        </ul>
      ) : (
        <>
          <div className="flex max-h-48 flex-col gap-1 overflow-y-auto rounded-md border p-2">
            {servers.map((s) => (
              <label
                key={s.id}
                className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-accent"
              >
                <input
                  type="checkbox"
                  checked={selected.includes(s.id)}
                  onChange={() => toggle(s.id)}
                />
                <span>
                  {s.name}{" "}
                  <span className="text-muted-foreground font-mono text-xs">
                    ({s.host})
                  </span>
                </span>
              </label>
            ))}
          </div>
          <div className="mt-3 flex justify-end">
            <Button
              size="sm"
              disabled={!dirty || mutation.isPending}
              onClick={() => mutation.mutate()}
            >
              {mutation.isPending ? "保存中…" : "保存分配"}
            </Button>
          </div>
        </>
      )}
    </>
  )

  if (variant === "flat") {
    return (
      <section className="border-b py-6 last:border-b-0">
        <div className="mb-4">
          <h2 className="text-base font-semibold">Server 分配</h2>
          <p className="text-muted-foreground mt-1 text-sm">
            将本证书授权给零到多台现有 Server；变更立即生效，不触发额外签发。
          </p>
        </div>
        {body}
      </section>
    )
  }

  return (
    <div className={cn("rounded-lg border bg-card p-6")}>
      <div className="mb-4">
        <h2 className="text-base font-semibold">Server 分配</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          将本证书授权给零到多台现有 Server；变更立即写入 MongoDB，不触发额外签发。
        </p>
      </div>
      {body}
    </div>
  )
}
