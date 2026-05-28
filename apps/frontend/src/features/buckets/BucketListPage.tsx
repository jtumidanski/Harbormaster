import { useState } from "react";
import { useNavigate } from "react-router-dom";
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
import { bucketsKeys } from "@/lib/api/keys";
import { listBuckets } from "./api";
import type { Bucket, PublicAccess } from "./types";
import { CreateBucketDialog } from "./CreateBucketDialog";

const DEFAULT_PAGE_SIZE = 25;
const DEFAULT_SORT = "name";

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let i = 0;
  let n = bytes;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n >= 10 || i === 0 ? n.toFixed(0) : n.toFixed(1)} ${units[i]}`;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function publicAccessLabel(mode: PublicAccess): string {
  switch (mode) {
    case "private":
      return "Private";
    case "public-read":
      return "Public read";
    case "public-read-write":
      return "Public RW";
  }
}

function publicAccessBadgeClass(mode: PublicAccess): string {
  switch (mode) {
    case "private":
      return "bg-muted text-muted-foreground";
    case "public-read":
      return "bg-amber-100 text-amber-900 dark:bg-amber-900/30 dark:text-amber-200";
    case "public-read-write":
      return "bg-destructive/15 text-destructive";
  }
}

function quotaCell(b: Bucket): string {
  if (!b.quota) return "—";
  const used = formatBytes(b.quota.used_bytes);
  const total = formatBytes(b.quota.bytes);
  return `${used} / ${total} (${b.quota.kind})`;
}

export function BucketListPage() {
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);
  const params = { page, size: DEFAULT_PAGE_SIZE, sort: DEFAULT_SORT };

  const q = useQuery({
    queryKey: bucketsKeys.list(params),
    queryFn: () => listBuckets(params),
  });

  const totalPages = q.data?.page?.total_pages ?? 1;
  const buckets = q.data?.buckets ?? [];

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Buckets</h1>
        <Button onClick={() => setCreateOpen(true)}>Create bucket</Button>
      </div>

      {q.isLoading ? (
        <TableSkeleton columns={8} />
      ) : q.isError ? (
        <p className="text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load buckets."}
        </p>
      ) : buckets.length === 0 ? (
        <div className="rounded-md border p-8 text-center text-muted-foreground">
          No buckets yet — create one to get started.
        </div>
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Objects</TableHead>
                <TableHead>Versioning</TableHead>
                <TableHead>Lifecycle</TableHead>
                <TableHead>Public access</TableHead>
                <TableHead>Quota</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {buckets.map((b) => (
                <TableRow
                  key={b.name}
                  className="cursor-pointer"
                  onClick={() => navigate(`/buckets/${encodeURIComponent(b.name)}`)}
                >
                  <TableCell className="font-medium">
                    <button
                      type="button"
                      className="text-primary hover:underline"
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate(`/buckets/${encodeURIComponent(b.name)}`);
                      }}
                    >
                      {b.name}
                    </button>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(b.created_at)}
                  </TableCell>
                  <TableCell>{formatBytes(b.estimated_bytes)}</TableCell>
                  <TableCell>{b.object_count.toLocaleString()}</TableCell>
                  <TableCell>
                    <Badge
                      variant="outline"
                      className={
                        b.versioning_enabled
                          ? "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/30 dark:text-emerald-200"
                          : "bg-muted text-muted-foreground"
                      }
                    >
                      {b.versioning_enabled ? "On" : "Off"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant="outline"
                      className={
                        b.has_lifecycle_rules
                          ? "bg-sky-100 text-sky-900 dark:bg-sky-900/30 dark:text-sky-200"
                          : "bg-muted text-muted-foreground"
                      }
                    >
                      {b.has_lifecycle_rules ? "Yes" : "No"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className={publicAccessBadgeClass(b.public_access)}>
                      {publicAccessLabel(b.public_access)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{quotaCell(b)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {totalPages > 1 && (
        <div className="mt-4 flex items-center justify-end gap-2 text-sm">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
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
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      )}

      <CreateBucketDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}
