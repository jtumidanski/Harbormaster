import { Link } from "react-router-dom";
import type { EventSummary } from "./types";

function formatRelative(iso: string): string {
  try {
    const then = new Date(iso).getTime();
    const now = Date.now();
    const diffSec = Math.max(0, Math.round((now - then) / 1000));
    if (diffSec < 60) return `${diffSec}s ago`;
    const m = Math.floor(diffSec / 60);
    if (m < 60) return `${m}m ago`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.floor(h / 24);
    return `${d}d ago`;
  } catch {
    return iso;
  }
}

function OutcomeBadge({ outcome }: { outcome: string }) {
  const ok = outcome === "success";
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs ${
        ok
          ? "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/30 dark:text-emerald-200"
          : "bg-destructive/15 text-destructive"
      }`}
    >
      {outcome}
    </span>
  );
}

export function RecentActivityList({ events }: { events: EventSummary[] }) {
  const shown = events.slice(0, 25);
  return (
    <div className="rounded-lg border bg-card text-card-foreground shadow-sm">
      <div className="flex items-center justify-between border-b p-4">
        <h2 className="text-lg font-semibold">Recent activity</h2>
        <Link to="/activity" className="text-sm text-primary hover:underline">
          View all activity
        </Link>
      </div>
      {shown.length === 0 ? (
        <p className="p-4 text-sm text-muted-foreground">No recent activity.</p>
      ) : (
        <ul className="divide-y">
          {shown.map((e) => (
            <li key={e.id} className="flex items-center justify-between gap-3 px-4 py-2 text-sm">
              <div className="flex min-w-0 flex-1 items-center gap-3">
                <span className="w-20 shrink-0 text-xs text-muted-foreground">
                  {formatRelative(e.occurred_at)}
                </span>
                <span className="font-medium">{e.action}</span>
                <span className="truncate text-muted-foreground">
                  {e.target_type}
                  {e.target_id ? `:${e.target_id}` : ""}
                </span>
              </div>
              <OutcomeBadge outcome={e.outcome} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
