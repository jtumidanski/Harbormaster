import { useEffect, useState } from "react";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AppError } from "@/lib/api/errors";
import { bucketsKeys } from "@/lib/api/keys";
import { setBucketPublicAccess } from "./api";
import type { PublicAccess } from "./types";

export type EditPublicAccessDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucketName: string;
  currentMode: PublicAccess;
};

export function EditPublicAccessDialog({
  open,
  onOpenChange,
  bucketName,
  currentMode,
}: EditPublicAccessDialogProps) {
  const qc = useQueryClient();
  const [mode, setMode] = useState<PublicAccess>(currentMode);
  const [confirmName, setConfirmName] = useState("");

  useEffect(() => {
    if (open) {
      setMode(currentMode);
      setConfirmName("");
    }
  }, [open, currentMode]);

  const needsConfirm = mode === "public-read-write";
  const canSubmit = !needsConfirm || confirmName === bucketName;

  const mutation = useMutation({
    mutationFn: () =>
      setBucketPublicAccess(bucketName, mode, needsConfirm ? confirmName : undefined),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: bucketsKeys.detail(bucketName) });
      await qc.invalidateQueries({ queryKey: bucketsKeys.all() });
      toast.success("Public access updated.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        toast.error(err.message || "Failed to update public access.");
      } else {
        toast.error("Failed to update public access.");
      }
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit public access</DialogTitle>
          <DialogDescription>
            Changing public access affects who can read or write objects in this bucket.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (!canSubmit) return;
            mutation.mutate();
          }}
          className="space-y-4"
          noValidate
        >
          <div className="space-y-2">
            <Label htmlFor="public-access-mode">Mode</Label>
            <Select value={mode} onValueChange={(v) => setMode(v as PublicAccess)}>
              <SelectTrigger id="public-access-mode">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="private">Private</SelectItem>
                <SelectItem value="public-read">Public read</SelectItem>
                <SelectItem value="public-read-write">Public read &amp; write</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {needsConfirm && (
            <div className="space-y-2">
              <Label htmlFor="public-access-confirm">
                Type <span className="font-mono">{bucketName}</span> to confirm
              </Label>
              <Input
                id="public-access-confirm"
                autoComplete="off"
                value={confirmName}
                onChange={(e) => setConfirmName(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Public read &amp; write allows anonymous uploads. This is rarely what you want.
              </p>
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit || mutation.isPending}>
              {mutation.isPending ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
