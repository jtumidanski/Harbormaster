import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
import { bulkDelete, previewBulkDelete } from "./api";

export type BulkDeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  // The prefix the object list is currently showing — used to invalidate
  // the right list query after the delete completes.
  listPrefix?: string;
  keys: string[];
  prefixes: string[];
  onDeleted: () => void;
};

function formatCount(objectCount: number, truncated: boolean): string {
  if (truncated) return "10,000+";
  return objectCount.toLocaleString();
}

export function BulkDeleteDialog({
  open,
  onOpenChange,
  bucket,
  listPrefix = "",
  keys,
  prefixes,
  onDeleted,
}: BulkDeleteDialogProps) {
  const qc = useQueryClient();

  // Sort the selection arrays so reordering selection doesn't refetch the
  // preview needlessly (stable query key).
  const sortedKeys = [...keys].sort();
  const sortedPrefixes = [...prefixes].sort();
  const selectedCount = keys.length + prefixes.length;

  const preview = useQuery({
    queryKey: ["objects", bucket, "bulk-delete-preview", sortedKeys, sortedPrefixes],
    queryFn: () => previewBulkDelete(bucket, { keys, prefixes }),
    enabled: open,
  });

  const mutation = useMutation({
    mutationFn: () => bulkDelete(bucket, { keys, prefixes }),
    onSuccess: async (res) => {
      await qc.invalidateQueries({ queryKey: objectsKeys.list(bucket, listPrefix) });
      if (res.failures.length === 0) {
        toast.success(`Deleted ${res.deleted_count.toLocaleString()} objects.`);
      } else {
        toast.warning(
          `Deleted ${res.deleted_count.toLocaleString()} · ${res.failures.length} failed.`,
        );
      }
      onDeleted();
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Bulk delete failed.");
      else toast.error("Bulk delete failed.");
    },
  });

  const isSinglePrefix = prefixes.length === 1 && keys.length === 0;
  const countLabel = preview.data
    ? formatCount(preview.data.object_count, preview.data.truncated)
    : "…";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete objects</DialogTitle>
          <DialogDescription>
            {preview.isLoading ? (
              <>Counting objects…</>
            ) : preview.isError ? (
              <>Could not determine how many objects this affects.</>
            ) : isSinglePrefix ? (
              <>
                Delete <span className="font-semibold">{countLabel}</span> objects under{" "}
                <span className="font-mono">{prefixes[0]}</span>?
              </>
            ) : (
              <>
                Delete <span className="font-semibold">{countLabel}</span> objects (
                {selectedCount} selected item{selectedCount === 1 ? "" : "s"})?
              </>
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={preview.isLoading || preview.isError || mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending ? "Deleting…" : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
