import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AppError } from "@/lib/api/errors";
import { bucketsKeys } from "@/lib/api/keys";
import { deleteBucket } from "./api";

export type DeleteBucketDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucketName: string;
  objectCount: number;
  onEmptyFirst: () => void;
};

export function DeleteBucketDialog({
  open,
  onOpenChange,
  bucketName,
  objectCount,
  onEmptyFirst,
}: DeleteBucketDialogProps) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [confirmName, setConfirmName] = useState("");
  const hasObjects = objectCount > 0;

  useEffect(() => {
    if (open) setConfirmName("");
  }, [open]);

  const mutation = useMutation({
    mutationFn: () => deleteBucket(bucketName, confirmName),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: bucketsKeys.all() });
      toast.success("Bucket deleted.");
      onOpenChange(false);
      navigate("/buckets");
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.status === 409 && err.code === "bucket_not_empty") {
          onOpenChange(false);
          onEmptyFirst();
          return;
        }
        toast.error(err.message || "Failed to delete bucket.");
        return;
      }
      toast.error("Failed to delete bucket.");
    },
  });

  const canSubmit = !hasObjects && confirmName === bucketName && !mutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete bucket</DialogTitle>
          <DialogDescription>
            This permanently removes the bucket from MinIO. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (canSubmit) mutation.mutate();
          }}
          className="space-y-4"
          noValidate
        >
          {hasObjects ? (
            <div className="space-y-2">
              <p className="text-sm">
                This bucket holds {objectCount.toLocaleString()} objects. You must empty it before
                it can be deleted.
              </p>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  onOpenChange(false);
                  onEmptyFirst();
                }}
              >
                Empty this bucket first
              </Button>
            </div>
          ) : (
            <div className="space-y-2">
              <Label htmlFor="delete-bucket-confirm">
                Type <span className="font-mono">{bucketName}</span> to confirm
              </Label>
              <Input
                id="delete-bucket-confirm"
                autoComplete="off"
                value={confirmName}
                onChange={(e) => setConfirmName(e.target.value)}
              />
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" variant="destructive" disabled={!canSubmit}>
              {mutation.isPending ? "Deleting…" : "Delete bucket"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
