import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react"

type Theme = "light" | "dark" | "system"
type ResolvedTheme = "light" | "dark"

const STORAGE_KEY = "neo-line-theme"

interface ThemeContextValue {
  theme: Theme
  resolvedTheme: ResolvedTheme
  setTheme: (theme: Theme) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function readStored(): Theme {
  if (typeof localStorage === "undefined") return "system"
  const value = localStorage.getItem(STORAGE_KEY)
  return value === "light" || value === "dark" || value === "system"
    ? value
    : "system"
}

function systemPrefersDark(): boolean {
  return (
    typeof matchMedia !== "undefined" &&
    matchMedia("(prefers-color-scheme: dark)").matches
  )
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readStored)
  const [systemDark, setSystemDark] = useState<boolean>(systemPrefersDark)

  useEffect(() => {
    const mql = matchMedia("(prefers-color-scheme: dark)")
    const onChange = () => setSystemDark(mql.matches)
    mql.addEventListener("change", onChange)
    return () => mql.removeEventListener("change", onChange)
  }, [])

  const resolvedTheme: ResolvedTheme =
    theme === "system" ? (systemDark ? "dark" : "light") : theme

  useEffect(() => {
    const root = document.documentElement
    root.classList.toggle("dark", resolvedTheme === "dark")
    root.style.colorScheme = resolvedTheme
  }, [resolvedTheme])

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next)
    localStorage.setItem(STORAGE_KEY, next)
  }, [])

  const value = useMemo(
    () => ({ theme, resolvedTheme, setTheme }),
    [theme, resolvedTheme, setTheme]
  )

  return <ThemeContext value={value}>{children}</ThemeContext>
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error("useTheme must be used within ThemeProvider")
  return ctx
}
