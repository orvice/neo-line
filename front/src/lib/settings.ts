import { useQuery } from "@tanstack/react-query"

import { api } from "./api"
import type { Settings } from "./types"

const DEFAULT_SETTINGS: Settings = {
  site_name: "neo-line",
  status_page_title: "服务状态",
}

// useSettings loads the site-wide presentation settings, falling back to
// sensible defaults while loading or on error so the UI always has a name.
export function useSettings(): Settings {
  const { data } = useQuery({
    queryKey: ["settings"],
    queryFn: () => api.getSettings(),
    staleTime: 60_000,
  })
  return data?.settings ?? DEFAULT_SETTINGS
}
