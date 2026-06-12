import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AppError } from "@/lib/api/errors";
import { objectsKeys } from "@/lib/api/keys";
import { Breadcrumb } from "./Breadcrumb";
import { ObjectVersionsSheet } from "./ObjectVersionsSheet";
import { PreviewPane } from "./PreviewPane";
import { ShareLinkDialog } from "./ShareLinkDialog";
import { UploadDialog } from "./UploadDialog";
import { BulkDeleteDialog } from "./BulkDeleteDialog";
import { VirtualizedObjectList } from "./VirtualizedObjectList";
import { deleteObject, downloadURL } from "./api";
import { useInfiniteObjects } from "./useInfiniteObjects";
import type { ObjectListItem } from "./types";

export type ObjectBrowserPageProps = {
  bucket: string;
};

type PreviewState = { key: string; contentType: string; size: number } | null;

export function ObjectBrowserPage({ bucket }: ObjectBrowserPageProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const prefix = searchParams.get("prefix") ?? "";
  const qc = useQueryClient();

  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  const [selectedPrefixes, setSelectedPrefixes] = useState<Set<string>>(new Set());
  // bulkTarget holds the keys/prefixes the dialog is acting on; null = closed.
  const [bulkTarget, setBulkTarget] = useState<{ keys: string[]; prefixes: string[] } | null>(null);

  const clearSelection = () => {
    setSelectedKeys(new Set());
    setSelectedPrefixes(new Set());
  };

  const toggleKey = (key: string) => {
    setSelectedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const togglePrefix = (prefix: string) => {
    setSelectedPrefixes((prev) => {
      const next = new Set(prev);
      if (next.has(prefix)) next.delete(prefix);
      else next.add(prefix);
      return next;
    });
  };

  const selectionCount = selectedKeys.size + selectedPrefixes.size;

  const setPrefix = (next: string) => {
    const sp = new URLSearchParams(searchParams);
    if (next) sp.set("prefix", next);
    else sp.delete("prefix");
    setSearchParams(sp, { replace: false });
    clearSelection();
  };

  const query = useInfiniteObjects(bucket, prefix);

  // Flatten the paged response into a single list[]. Folders (object_prefixes)
  // appear before entries within each page because that is the order the
  // backend emits them; preserving page order keeps the virtualiser stable.
  const items: ObjectListItem[] = useMemo(() => {
    if (!query.data) return [];
    return query.data.pages.flatMap((p) => p.data);
  }, [query.data]);

  const [uploadOpen, setUploadOpen] = useState(false);
  const [shareKey, setShareKey] = useState<string | null>(null);
  const [deleteKey, setDeleteKey] = useState<string | null>(null);
  const [preview, setPreview] = useState<PreviewState>(null);
  const [versionsKey, setVersionsKey] = useState<string | null>(null);

  const deleteMutation = useMutation({
    mutationFn: (key: string) => deleteObject(bucket, key),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: objectsKeys.list(bucket, prefix) });
      toast.success("Object deleted.");
      setDeleteKey(null);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Failed to delete object.");
      else toast.error("Failed to delete object.");
    },
  });

  const onDownload = (key: string) => {
    // Use a transient <a download> so the browser handles content-disposition
    // and we don't pollute history with a navigation.
    const a = document.createElement("a");
    a.href = downloadURL(bucket, key);
    a.rel = "noopener";
    a.download = "";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Breadcrumb bucket={bucket} prefix={prefix} onNavigate={setPrefix} />
        <Button type="button" onClick={() => setUploadOpen(true)}>
          Upload
        </Button>
      </div>

      {selectionCount > 0 && (
        <div
          className="flex items-center justify-between rounded border bg-accent/30 px-3 py-2 text-sm"
          data-testid="selection-toolbar"
        >
          <span>{selectionCount} selected</span>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={() =>
                setBulkTarget({
                  keys: Array.from(selectedKeys),
                  prefixes: Array.from(selectedPrefixes),
                })
              }
            >
              Delete selected
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={clearSelection}>
              Clear
            </Button>
          </div>
        </div>
      )}

      {query.isLoading ? (
        <div className="rounded border bg-background p-6 text-sm text-muted-foreground">
          Loading…
        </div>
      ) : query.isError ? (
        <div className="rounded border bg-background p-6 text-sm text-destructive">
          {query.error instanceof AppError ? query.error.message : "Failed to list objects."}
        </div>
      ) : (
        <VirtualizedObjectList
          items={items}
          hasNextPage={Boolean(query.hasNextPage)}
          isFetchingNextPage={query.isFetchingNextPage}
          fetchNextPage={() => {
            void query.fetchNextPage();
          }}
          onOpenPrefix={setPrefix}
          onDownload={onDownload}
          onDelete={(key) => setDeleteKey(key)}
          onShare={(key) => setShareKey(key)}
          onPreview={(key, contentType, size) => setPreview({ key, contentType, size })}
          onVersions={(key) => setVersionsKey(key)}
          selectedKeys={selectedKeys}
          selectedPrefixes={selectedPrefixes}
          onToggleKey={toggleKey}
          onTogglePrefix={togglePrefix}
          onDeletePrefix={(prefix) => setBulkTarget({ keys: [], prefixes: [prefix] })}
        />
      )}

      <UploadDialog
        open={uploadOpen}
        onOpenChange={setUploadOpen}
        bucket={bucket}
        prefix={prefix}
      />

      {shareKey !== null && (
        <ShareLinkDialog
          open={shareKey !== null}
          onOpenChange={(o) => {
            if (!o) setShareKey(null);
          }}
          bucket={bucket}
          objectKey={shareKey}
        />
      )}

      {preview !== null && (
        <PreviewPane
          open={preview !== null}
          onOpenChange={(o) => {
            if (!o) setPreview(null);
          }}
          bucket={bucket}
          objectKey={preview.key}
          contentType={preview.contentType}
          size={preview.size}
        />
      )}

      {versionsKey !== null && (
        <ObjectVersionsSheet
          bucket={bucket}
          objectKey={versionsKey}
          prefix={prefix}
          open={versionsKey !== null}
          onOpenChange={(o) => {
            if (!o) setVersionsKey(null);
          }}
        />
      )}

      {bulkTarget !== null && (
        <BulkDeleteDialog
          open={bulkTarget !== null}
          onOpenChange={(o) => {
            if (!o) setBulkTarget(null);
          }}
          bucket={bucket}
          listPrefix={prefix}
          keys={bulkTarget.keys}
          prefixes={bulkTarget.prefixes}
          onDeleted={() => {
            clearSelection();
            setBulkTarget(null);
          }}
        />
      )}

      <Dialog
        open={deleteKey !== null}
        onOpenChange={(o) => {
          if (!o) setDeleteKey(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete object</DialogTitle>
            <DialogDescription>
              This permanently removes <span className="font-mono">{deleteKey}</span> from{" "}
              <span className="font-mono">{bucket}</span>.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteKey(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (deleteKey) deleteMutation.mutate(deleteKey);
              }}
            >
              {deleteMutation.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
