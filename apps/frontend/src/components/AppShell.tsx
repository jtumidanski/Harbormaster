import { Link, Outlet } from "react-router-dom";
import { Monitor, Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTheme } from "@/context/ThemeProvider";

type Theme = "light" | "dark" | "system";

function nextTheme(current: Theme): Theme {
  if (current === "light") return "dark";
  if (current === "dark") return "system";
  return "light";
}

function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const Icon = theme === "light" ? Sun : theme === "dark" ? Moon : Monitor;
  const label = `Theme: ${theme} (click to switch)`;
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={label}
      title={label}
      onClick={() => setTheme(nextTheme(theme))}
    >
      <Icon aria-hidden="true" />
    </Button>
  );
}

export function AppShell() {
  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex h-14 items-center justify-between border-b px-4">
        <div className="font-semibold">Harbormaster</div>
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <ThemeToggle />
          <span aria-hidden="true">{/* user menu (T2.10) */}</span>
        </div>
      </header>
      <div className="flex flex-1">
        <nav aria-label="Primary" className="w-56 border-r p-4">
          <ul className="flex flex-col gap-2 text-sm">
            <li>
              <Link to="/dashboard" className="block rounded px-2 py-1 hover:bg-accent">
                Dashboard
              </Link>
            </li>
            <li>
              <Link to="/buckets" className="block rounded px-2 py-1 hover:bg-accent">
                Buckets
              </Link>
            </li>
            <li>
              <Link to="/users" className="block rounded px-2 py-1 hover:bg-accent">
                Users
              </Link>
            </li>
            <li>
              <Link to="/policies" className="block rounded px-2 py-1 hover:bg-accent">
                Policies
              </Link>
            </li>
            <li>
              <Link to="/activity" className="block rounded px-2 py-1 hover:bg-accent">
                Activity
              </Link>
            </li>
            <li>
              <Link to="/settings/connection" className="block rounded px-2 py-1 hover:bg-accent">
                Settings
              </Link>
            </li>
          </ul>
        </nav>
        <main className="flex-1">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
