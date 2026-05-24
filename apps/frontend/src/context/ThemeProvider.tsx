import { createContext, useContext, useEffect, useState, type PropsWithChildren } from "react";

type Theme = "light" | "dark" | "system";
type Resolved = "light" | "dark";
type Ctx = { theme: Theme; setTheme: (t: Theme) => void; resolved: Resolved };

const ThemeCtx = createContext<Ctx | null>(null);

function readStoredTheme(): Theme {
  const stored = localStorage.getItem("theme");
  if (stored === "light" || stored === "dark" || stored === "system") return stored;
  return "system";
}

export function ThemeProvider({ children }: PropsWithChildren) {
  const [theme, setTheme] = useState<Theme>(() => readStoredTheme());
  const [resolved, setResolved] = useState<Resolved>("light");

  useEffect(() => {
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      const r: Resolved = theme === "system" ? (mql.matches ? "dark" : "light") : theme;
      document.documentElement.classList.toggle("dark", r === "dark");
      setResolved(r);
    };
    apply();
    mql.addEventListener("change", apply);
    return () => {
      mql.removeEventListener("change", apply);
    };
  }, [theme]);

  useEffect(() => {
    localStorage.setItem("theme", theme);
  }, [theme]);

  return <ThemeCtx.Provider value={{ theme, setTheme, resolved }}>{children}</ThemeCtx.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeCtx);
  if (!ctx) throw new Error("useTheme outside ThemeProvider");
  return ctx;
}
