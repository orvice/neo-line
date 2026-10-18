import { NavLink } from "react-router-dom"
import { cn } from "@/lib/utils"

const tabClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    "rounded-md px-3 py-1.5 text-sm font-medium transition",
    isActive
      ? "bg-card text-foreground shadow-xs ring ring-hairline"
      : "text-muted-foreground hover:bg-accent hover:text-foreground"
  )

export function CertificateNavTabs() {
  return (
    <nav className="flex flex-wrap gap-2">
      <NavLink to="/certificates/managed" className={tabClass}>
        托管证书
      </NavLink>
      <NavLink to="/certificates/issuers" className={tabClass}>
        ACME Issuer
      </NavLink>
      <NavLink to="/certificates/dns-accounts" className={tabClass}>
        DNS 账户
      </NavLink>
    </nav>
  )
}
