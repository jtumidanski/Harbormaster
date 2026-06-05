import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
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
import { policiesKeys, usersKeys } from "@/lib/api/keys";
import { listPolicies } from "@/features/policies/policiesApi";
import { updateUserPolicies } from "./api";
import type { User } from "./types";

export type EditCustomPoliciesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: User;
};

export function EditCustomPoliciesDialog({
  open,
  onOpenChange,
  user,
}: EditCustomPoliciesDialogProps) {
  const qc = useQueryClient();

  const policiesQ = useQuery({
    queryKey: policiesKeys.list(),
    queryFn: listPolicies,
    enabled: open,
  });

  const customPolicies = (policiesQ.data ?? []).filter((p) => p.origin === "custom");

  const [selected, setSelected] = useState<Set<string>>(() => new Set(user.attached_policies));

  useEffect(() => {
    if (open) setSelected(new Set(user.attached_policies));
  }, [open, user.attached_policies]);

  const mutation = useMutation({
    mutationFn: () => {
      const selectedNames = Array.from(selected);
      return updateUserPolicies(user.access_key, user.attached_templates, selectedNames);
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: usersKeys.detail(user.access_key) });
      await qc.invalidateQueries({ queryKey: usersKeys.list() });
      toast.success("Custom policies updated.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.code === "unknown_policy") {
          toast.error("One or more selected policies no longer exist.");
        } else {
          toast.error(err.message || "Failed to update custom policies.");
        }
      } else {
        toast.error("Failed to update custom policies.");
      }
    },
  });

  function toggle(name: string, checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(name);
      } else {
        next.delete(name);
      }
      return next;
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit custom policies</DialogTitle>
          <DialogDescription>
            Select custom policies to attach to {user.access_key}.
          </DialogDescription>
        </DialogHeader>

        {policiesQ.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading policies…</p>
        ) : policiesQ.isError ? (
          <p className="text-sm text-destructive">Failed to load policies.</p>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (!mutation.isPending) mutation.mutate();
            }}
            className="space-y-4"
            noValidate
          >
            {customPolicies.length === 0 ? (
              <p className="text-sm text-muted-foreground">No custom policies available.</p>
            ) : (
              <ul className="space-y-2">
                {customPolicies.map((policy) => (
                  <li key={policy.name} className="rounded-md border p-3">
                    <div className="flex items-start gap-2">
                      <input
                        id={`policy-${policy.name}`}
                        type="checkbox"
                        className="mt-1 h-4 w-4"
                        checked={selected.has(policy.name)}
                        onChange={(e) => toggle(policy.name, e.target.checked)}
                      />
                      <div className="flex-1 space-y-1">
                        <Label htmlFor={`policy-${policy.name}`} className="font-medium">
                          {policy.name}
                        </Label>
                        {policy.statement_summary && (
                          <p className="text-xs text-muted-foreground">
                            {policy.statement_summary}
                          </p>
                        )}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            )}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? "Saving…" : "Save policies"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
