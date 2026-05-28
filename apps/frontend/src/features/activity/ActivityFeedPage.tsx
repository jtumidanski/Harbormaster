import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TableSkeleton } from "@/components/common/TableSkeleton";
import { AppError } from "@/lib/api/errors";
import { activityKeys } from "@/lib/api/keys";
import { listAuditEvents } from "./api";
import { FiltersSidebar } from "./FiltersSidebar";
import type { AuditEvent, AuditFilter } from "./types";

const PAGE_SIZE = 50;
const MAX_ERROR_LENGTH = 100;

function readFilter(sp: URLSearchParams): AuditFilter {
  const f: AuditFilter = {};
  const action = sp.get("action");
  const targetType = sp.get("target_type");
  const targetId = sp.get("target_id");
  const outcome = sp.get("outcome");
  const from = sp.get("from");
  const to = sp.get("to");
  if (action) f.action = action;
  if (targetType) f.target_type = targetType;
  if (targetId) f.target_id = targetId;
  if (outcome) f.outcome = outcome;
  if (from) f.from = from;
  if (to) f.to = to;
  return f;
}

function readPage(sp: URLSearchParams): number {
  const p = Number(sp.get("page") ?? "1");
  return Number.isFinite(p) && p >= 1 ? Math.floor(p) : 1;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function truncate(s: string | null, max: number): string {
  if (!s) return "";
  return s.length > max ? `${s.slice(0, max)}…` : s;
}

function OutcomeBadge({ outcome }: { outcome: string }) {
  const ok = outcome === "success";
  return (
    <Badge
      variant="outline"
      className={
        ok
          ? "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/30 dark:text-emerald-200"
          : "bg-destructive/15 text-destructive"
      }
    >
      {outcome}
    </Badge>
  );
}

export function ActivityFeedPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filter = useMemo(() => readFilter(searchParams), [searchParams]);
  const page = readPage(searchParams);
  const pageParams = useMemo(() => ({ number: page, size: PAGE_SIZE }), [page]);

  const q = useQuery({
    queryKey: activityKeys.list(filter, pageParams),
    queryFn: () => listAuditEvents(filter, pageParams),
  });

  function applyFilter(next: AuditFilter) {
    const sp = new URLSearchParams();
    if (next.action) sp.set("action", next.action);
    if (next.target_type) sp.set("target_type", next.target_type);
    if (next.target_id) sp.set("target_id", next.target_id);
    if (next.outcome) sp.set("outcome", next.outcome);
    if (next.from) sp.set("from", next.from);
    if (next.to) sp.set("to", next.to);
    sp.set("page", "1");
    setSearchParams(sp);
  }

  function resetFilter() {
    setSearchParams(new URLSearchParams());
  }

  function setPage(n: number) {
    const sp = new URLSearchParams(searchParams);
    sp.set("page", String(n));
    setSearchParams(sp);
  }

  // The event id lives at the JSON:API resource level (data[].id), not in
  // attributes — merge it in so row keys (and any id-based lookups) work.
  const events: AuditEvent[] = q.data?.data?.map((d) => ({ ...d.attributes, id: d.id })) ?? [];
  const meta = q.data?.meta?.page;
  const totalPages = meta?.total_pages ?? 1;

  return (
    <div className="grid grid-cols-1 gap-6 p-6 lg:grid-cols-[280px_1fr]">
      <FiltersSidebar value={filter} onApply={applyFilter} onReset={resetFilter} />

      <div>
        <h1 className="mb-4 text-2xl font-semibold">Activity</h1>

        {q.isLoading ? (
          <TableSkeleton columns={6} />
        ) : q.isError ? (
          <p className="text-destructive">
            {q.error instanceof AppError ? q.error.message : "Failed to load audit events."}
          </p>
        ) : events.length === 0 ? (
          <div className="rounded-md border p-8 text-center text-muted-foreground">
            No events match the current filters.
          </div>
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>When</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Target</TableHead>
                  <TableHead>Outcome</TableHead>
                  <TableHead>Source IP</TableHead>
                  <TableHead>Error</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((e) => (
                  <TableRow key={e.id}>
                    <TableCell className="text-muted-foreground">
                      {formatDate(e.occurred_at)}
                    </TableCell>
                    <TableCell className="font-medium">{e.action}</TableCell>
                    <TableCell>
                      {e.target_type}
                      {e.target_id ? `:${e.target_id}` : ""}
                    </TableCell>
                    <TableCell>
                      <OutcomeBadge outcome={e.outcome} />
                    </TableCell>
                    <TableCell className="text-muted-foreground">{e.source_ip}</TableCell>
                    <TableCell className="text-destructive" title={e.error_message ?? undefined}>
                      {truncate(e.error_message, MAX_ERROR_LENGTH)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <div className="mt-4 flex items-center justify-end gap-2 text-sm">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage(Math.max(1, page - 1))}
          >
            Previous
          </Button>
          <span className="text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage(page + 1)}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  );
}
