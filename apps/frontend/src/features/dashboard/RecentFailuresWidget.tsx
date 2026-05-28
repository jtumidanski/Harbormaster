import { Link } from "react-router-dom";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { DashboardWindow, FailureSummary } from "./types";

const WINDOW_LABEL: Record<DashboardWindow, string> = {
  "24h": "24h",
  "7d": "7d",
  "30d": "30d",
};

const WINDOW_HOURS: Record<DashboardWindow, number> = {
  "24h": 24,
  "7d": 24 * 7,
  "30d": 24 * 30,
};

function cutoffISO(window: DashboardWindow): string {
  const now = Date.now();
  return new Date(now - WINDOW_HOURS[window] * 3600 * 1000).toISOString();
}

export function RecentFailuresWidget({
  window,
  onWindowChange,
  count,
  entries,
}: {
  window: DashboardWindow;
  onWindowChange: (w: DashboardWindow) => void;
  count: number;
  entries: FailureSummary[];
}) {
  const shown = entries.slice(0, 10);
  const from = cutoffISO(window);
  const to = new Date().toISOString();
  const seeAllHref = `/activity?outcome=failure&from=${encodeURIComponent(
    from,
  )}&to=${encodeURIComponent(to)}`;

  return (
    <div className="flex h-full flex-col rounded-lg border bg-card text-card-foreground shadow-sm">
      <div className="flex items-center justify-between border-b p-4">
        <h2 className="text-lg font-semibold">Recent failures</h2>
        <Select value={window} onValueChange={(v) => onWindowChange(v as DashboardWindow)}>
          <SelectTrigger className="h-8 w-36" aria-label="Failures window">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="24h">Last 24h</SelectItem>
            <SelectItem value="7d">Last 7 days</SelectItem>
            <SelectItem value="30d">Last 30 days</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="p-4">
        <p className="mb-2 text-sm font-medium">
          {count} {count === 1 ? "failure" : "failures"} in {WINDOW_LABEL[window]}
        </p>
        {shown.length === 0 ? (
          <p className="text-sm text-muted-foreground">No failures.</p>
        ) : (
          <ul className="divide-y">
            {shown.map((f) => (
              <li key={f.id} className="py-2 text-sm">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-medium">{f.action}</span>
                  <span className="text-xs text-muted-foreground">
                    {f.target_type}
                    {f.target_id ? `:${f.target_id}` : ""}
                  </span>
                </div>
                {f.error_message && (
                  <p className="mt-1 truncate text-xs text-destructive">{f.error_message}</p>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="mt-auto border-t p-3 text-right">
        <Link to={seeAllHref} className="text-sm text-primary hover:underline">
          See all
        </Link>
      </div>
    </div>
  );
}
