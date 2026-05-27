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
import { usersKeys } from "@/lib/api/keys";
import { deleteUser } from "./api";

export type DeleteUserDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accessKey: string;
};

export function DeleteUserDialog({ open, onOpenChange, accessKey }: DeleteUserDialogProps) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [confirm, setConfirm] = useState("");

  useEffect(() => {
    if (open) setConfirm("");
  }, [open]);

  const mutation = useMutation({
    mutationFn: () => deleteUser(accessKey, confirm),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: usersKeys.all() });
      toast.success("User deleted.");
      onOpenChange(false);
      navigate("/users");
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        toast.error(err.message || "Failed to delete user.");
      } else {
        toast.error("Failed to delete user.");
      }
    },
  });

  const canSubmit = confirm === accessKey && !mutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete user</DialogTitle>
          <DialogDescription>
            This permanently removes the user from MinIO. Service accounts owned by this user will
            also be revoked. This cannot be undone.
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
          <div className="space-y-2">
            <Label htmlFor="delete-user-confirm">
              Type <span className="font-mono">{accessKey}</span> to confirm
            </Label>
            <Input
              id="delete-user-confirm"
              autoComplete="off"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" variant="destructive" disabled={!canSubmit}>
              {mutation.isPending ? "Deleting…" : "Delete user"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
