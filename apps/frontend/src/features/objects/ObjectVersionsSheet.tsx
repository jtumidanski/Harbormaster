import { useMemo, useState } from "react";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Download, RotateCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { AppError } from "@/lib/api/errors";
import { objectsKeys } from "@/lib/api/keys";
import {
  deleteVersion,
  listVersions,
  restoreVersion,
  undeleteObject,
  versionDownloadURL,
} from "./api";
import type { ObjectVersionItem, ObjectVersionListResponse } from "./types";

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatBytes(bytes: number | null): string {
  if (bytes === null) return "—";
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
    return new Date(iso).toLocaleString(undefined, {
      dateStyle: "short",
      timeStyle: "short",
    });
  } catch {
    return iso;
  }
}

// ─── Confirmation Dialog Types ────────────────────────────────────────────────

type RestoreTarget = { versionId: string } | null;
type DeleteTarget = { versionId: string } | null;

// ─── Props ────────────────────────────────────────────────────────────────────

export type ObjectVersionsSheetProps = {
  bucket: string;
  objectKey: string;
  prefix: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// ─── Component ────────────────────────────────────────────────────────────────

export function ObjectVersionsSheet({
  bucket,
  objectKey,
  prefix,
  open,
  onOpenChange,
}: ObjectVersionsSheetProps) {
  const qc = useQueryClient();

  const query = useInfiniteQuery({
    queryKey: objectsKeys.versions(bucket, objectKey),
    initialPageParam: "" as string,
    queryFn: ({ pageParam }: { pageParam: string }): Promise<ObjectVersionListResponse> =>
      listVersions(bucket, objectKey, pageParam || undefined),
    getNextPageParam: (last: ObjectVersionListResponse): string | undefined => {
      const token = last.meta?.page?.next_token;
      return token ? token : undefined;
    },
    enabled: open && objectKey.length > 0 && bucket.length > 0,
  });

  const versions: ObjectVersionItem[] = useMemo(() => {
    if (!query.data) return [];
    return query.data.pages.flatMap((p) => p.data);
  }, [query.data]);

  // The "effective" latest version — the first in the sorted list (backend
  // returns most-recent first). Used to decide whether to show Undelete.
  const latestVersion = versions[0] ?? null;
  const latestIsDeleteMarker =
    latestVersion !== null &&
    latestVersion.attributes.is_latest &&
    latestVersion.attributes.is_delete_marker;

  // ── Confirmation state ──
  const [restoreTarget, setRestoreTarget] = useState<RestoreTarget>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget>(null);

  // ── Invalidation helpers ──
  const invalidateAll = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: objectsKeys.versions(bucket, objectKey) }),
      qc.invalidateQueries({ queryKey: objectsKeys.list(bucket, prefix) }),
    ]);
  };

  // ── Mutations ──
  const restoreMutation = useMutation({
    mutationFn: (versionId: string) => restoreVersion(bucket, objectKey, versionId),
    onSuccess: async () => {
      await invalidateAll();
      toast.success("Version restored successfully.");
      setRestoreTarget(null);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Failed to restore version.");
      else toast.error("Failed to restore version.");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (versionId: string) => deleteVersion(bucket, objectKey, versionId),
    onSuccess: async () => {
      await invalidateAll();
      toast.success("Version deleted permanently.");
      setDeleteTarget(null);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Failed to delete version.");
      else toast.error("Failed to delete version.");
    },
  });

  const undeleteMutation = useMutation({
    mutationFn: () => undeleteObject(bucket, objectKey),
    onSuccess: async () => {
      await invalidateAll();
      toast.success("Object undeleted successfully.");
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Failed to undelete object.");
      else toast.error("Failed to undelete object.");
    },
  });

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side="right" className="flex w-full flex-col gap-4 sm:max-w-2xl">
          <SheetHeader>
            <SheetTitle>Version history</SheetTitle>
            <SheetDescription className="break-all font-mono text-xs">{objectKey}</SheetDescription>
          </SheetHeader>

          {latestIsDeleteMarker && (
            <div className="flex items-center gap-3 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm dark:border-amber-800 dark:bg-amber-950/30">
              <span className="flex-1 text-amber-800 dark:text-amber-300">
                The current version is a delete marker — the object appears deleted.
              </span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={undeleteMutation.isPending}
                onClick={() => undeleteMutation.mutate()}
              >
                {undeleteMutation.isPending ? "Undeleting…" : "Undelete"}
              </Button>
            </div>
          )}

          {query.isLoading ? (
            <div className="rounded border bg-background p-6 text-sm text-muted-foreground">
              Loading versions…
            </div>
          ) : query.isError ? (
            <div className="rounded border bg-background p-6 text-sm text-destructive">
              {query.error instanceof AppError
                ? query.error.message
                : "Failed to load version history."}
            </div>
          ) : versions.length === 0 ? (
            <div className="rounded border bg-background p-6 text-center text-sm text-muted-foreground">
              No versions found.
            </div>
          ) : (
            <div className="flex-1 overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Version ID</TableHead>
                    <TableHead className="w-24 text-right">Size</TableHead>
                    <TableHead className="w-36">Modified</TableHead>
                    <TableHead className="w-28">Status</TableHead>
                    <TableHead className="w-28 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {versions.map((v) => {
                    const a = v.attributes;
                    const shortId =
                      a.version_id.length > 12 ? a.version_id.slice(0, 8) + "…" : a.version_id;
                    return (
                      <TableRow key={v.id}>
                        <TableCell className="font-mono text-xs" title={a.version_id}>
                          {shortId}
                        </TableCell>
                        <TableCell className="text-right text-xs text-muted-foreground">
                          {formatBytes(a.size)}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {formatDate(a.last_modified)}
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap gap-1">
                            {a.is_latest && (
                              <Badge variant="secondary" className="text-xs">
                                Latest
                              </Badge>
                            )}
                            {a.is_delete_marker && (
                              <Badge
                                variant="outline"
                                className="text-xs text-amber-600 border-amber-400"
                              >
                                Delete marker
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center justify-end gap-1">
                            {!a.is_delete_marker && (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                asChild
                                aria-label={`Download version ${a.version_id}`}
                              >
                                <a
                                  href={versionDownloadURL(bucket, objectKey, a.version_id)}
                                  rel="noopener"
                                  download
                                >
                                  <Download className="h-4 w-4" aria-hidden="true" />
                                </a>
                              </Button>
                            )}
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              aria-label={`Restore version ${a.version_id}`}
                              disabled={a.is_delete_marker}
                              onClick={() => setRestoreTarget({ versionId: a.version_id })}
                            >
                              <RotateCcw className="h-4 w-4" aria-hidden="true" />
                              <span className="sr-only">Restore</span>
                            </Button>
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              aria-label={`Delete version ${a.version_id}`}
                              onClick={() => setDeleteTarget({ versionId: a.version_id })}
                            >
                              <Trash2 className="h-4 w-4" aria-hidden="true" />
                              <span className="sr-only">Delete version</span>
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>

              {query.hasNextPage && (
                <div className="mt-2 flex justify-center">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={query.isFetchingNextPage}
                    onClick={() => {
                      void query.fetchNextPage();
                    }}
                  >
                    {query.isFetchingNextPage ? "Loading…" : "Load more"}
                  </Button>
                </div>
              )}
            </div>
          )}
        </SheetContent>
      </Sheet>

      {/* ── Restore confirmation dialog ── */}
      <Dialog
        open={restoreTarget !== null}
        onOpenChange={(o) => {
          if (!o) setRestoreTarget(null);
        }}
      >
        <DialogContent aria-label="Restore version">
          <DialogHeader>
            <DialogTitle>Restore version</DialogTitle>
            <DialogDescription>
              This will make version <span className="font-mono">{restoreTarget?.versionId}</span>{" "}
              the current (latest) version of <span className="font-mono">{objectKey}</span>. The
              previous current version will still exist as an older version.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRestoreTarget(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              disabled={restoreMutation.isPending}
              onClick={() => {
                if (restoreTarget) restoreMutation.mutate(restoreTarget.versionId);
              }}
            >
              {restoreMutation.isPending ? "Restoring…" : "Restore"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Delete version confirmation dialog ── */}
      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(o) => {
          if (!o) setDeleteTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete version permanently</DialogTitle>
            <DialogDescription>
              This permanently and irreversibly removes version{" "}
              <span className="font-mono">{deleteTarget?.versionId}</span> of{" "}
              <span className="font-mono">{objectKey}</span>. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (deleteTarget) deleteMutation.mutate(deleteTarget.versionId);
              }}
            >
              {deleteMutation.isPending ? "Deleting…" : "Delete permanently"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
