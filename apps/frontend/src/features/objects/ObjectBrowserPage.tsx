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
import { PreviewPane } from "./PreviewPane";
import { ShareLinkDialog } from "./ShareLinkDialog";
import { UploadDialog } from "./UploadDialog";
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

  const setPrefix = (next: string) => {
    const sp = new URLSearchParams(searchParams);
    if (next) sp.set("prefix", next);
    else sp.delete("prefix");
    setSearchParams(sp, { replace: false });
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
