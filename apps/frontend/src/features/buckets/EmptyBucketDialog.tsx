import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { bucketsKeys, objectsKeys } from "@/lib/api/keys";
import { useEmptyBucket } from "./useEmptyBucket";

export type EmptyBucketDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucketName: string;
  versioningEnabled: boolean;
  estimatedTotal?: number;
};

export function EmptyBucketDialog({
  open,
  onOpenChange,
  bucketName,
  versioningEnabled,
  estimatedTotal,
}: EmptyBucketDialogProps) {
  const qc = useQueryClient();
  const { start, reset, progress, done, errorMsg, stalled, isRunning } = useEmptyBucket(bucketName);
  const [confirmName, setConfirmName] = useState("");
  const [purgeVersions, setPurgeVersions] = useState(false);

  useEffect(() => {
    if (open) {
      setConfirmName("");
      setPurgeVersions(false);
      reset();
    }
  }, [open, reset]);

  useEffect(() => {
    if (!done) return;
    void qc.invalidateQueries({ queryKey: bucketsKeys.detail(bucketName) });
    void qc.invalidateQueries({ queryKey: objectsKeys.list(bucketName, "") });
    void qc.invalidateQueries({ queryKey: bucketsKeys.all() });
    toast.success(`Emptied bucket — ${done.deletedTotal.toLocaleString()} objects removed.`);
  }, [done, qc, bucketName]);

  const canSubmit = confirmName === bucketName && !isRunning && !done;

  const percent =
    estimatedTotal && estimatedTotal > 0
      ? Math.min(100, Math.round((progress / estimatedTotal) * 100))
      : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Empty bucket</DialogTitle>
          <DialogDescription>
            Delete all objects in <span className="font-mono">{bucketName}</span>.
          </DialogDescription>
        </DialogHeader>

        {done ? (
          <div className="space-y-3">
            <Alert>
              <AlertTitle>Done</AlertTitle>
              <AlertDescription>
                Deleted {done.deletedTotal.toLocaleString()} objects in{" "}
                {Math.round(done.durationMs / 100) / 10}s.
              </AlertDescription>
            </Alert>
            <DialogFooter>
              <Button type="button" onClick={() => onOpenChange(false)}>
                Close
              </Button>
            </DialogFooter>
          </div>
        ) : errorMsg ? (
          <div className="space-y-3">
            <Alert variant="destructive">
              <AlertTitle>Empty failed</AlertTitle>
              <AlertDescription>{errorMsg}</AlertDescription>
            </Alert>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Close
              </Button>
            </DialogFooter>
          </div>
        ) : isRunning ? (
          <div className="space-y-3">
            <div className="space-y-1">
              <div className="flex items-center justify-between text-sm">
                <span>Deleting…</span>
                <span className="font-mono text-xs text-muted-foreground">
                  {progress.toLocaleString()}
                  {estimatedTotal ? ` / ${estimatedTotal.toLocaleString()}` : ""}
                </span>
              </div>
              <div
                role="progressbar"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={percent ?? undefined}
                className="h-2 w-full overflow-hidden rounded bg-muted"
              >
                <div
                  className="h-full bg-primary transition-all"
                  style={{ width: percent !== null ? `${percent}%` : "30%" }}
                />
              </div>
            </div>
            {stalled && (
              <Alert variant="destructive">
                <AlertTitle>No progress for 30 seconds</AlertTitle>
                <AlertDescription>
                  The server hasn&apos;t reported progress recently. The job may still be running.
                </AlertDescription>
              </Alert>
            )}
          </div>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (canSubmit) void start(confirmName, purgeVersions);
            }}
            className="space-y-4"
            noValidate
          >
            <Alert variant="destructive">
              <AlertTitle>This cannot be undone</AlertTitle>
              <AlertDescription>
                {versioningEnabled && purgeVersions
                  ? "Permanent — no recovery."
                  : versioningEnabled
                    ? "Recoverable via version restore."
                    : "All objects will be permanently removed."}
              </AlertDescription>
            </Alert>

            {versioningEnabled && (
              <div className="flex items-start gap-2">
                <input
                  id="empty-purge-versions"
                  type="checkbox"
                  className="mt-1 h-4 w-4"
                  checked={purgeVersions}
                  onChange={(e) => setPurgeVersions(e.target.checked)}
                />
                <Label htmlFor="empty-purge-versions" className="font-normal">
                  Also permanently delete all object versions and delete-markers
                </Label>
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="empty-confirm">
                Type <span className="font-mono">{bucketName}</span> to confirm
              </Label>
              <Input
                id="empty-confirm"
                autoComplete="off"
                value={confirmName}
                onChange={(e) => setConfirmName(e.target.value)}
              />
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" variant="destructive" disabled={!canSubmit}>
                Empty bucket
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
