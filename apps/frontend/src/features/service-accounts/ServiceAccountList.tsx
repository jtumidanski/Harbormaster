import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { AppError } from "@/lib/api/errors";
import { serviceAccountsKeys } from "@/lib/api/keys";
import { listServiceAccounts, revokeServiceAccount, type ServiceAccount } from "./api";
import { CreateServiceAccountDialog } from "./CreateServiceAccountDialog";

export type ServiceAccountListProps = {
  parentAccessKey: string;
};

export function ServiceAccountList({ parentAccessKey }: ServiceAccountListProps) {
  const qc = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [confirmKey, setConfirmKey] = useState<string | null>(null);

  const q = useQuery({
    queryKey: serviceAccountsKeys.forUser(parentAccessKey),
    queryFn: () => listServiceAccounts(parentAccessKey),
  });

  const revoke = useMutation({
    mutationFn: (saKey: string) => revokeServiceAccount(saKey),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: serviceAccountsKeys.forUser(parentAccessKey) });
      toast.success("Service account revoked.");
      setConfirmKey(null);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        toast.error(err.message || "Failed to revoke service account.");
      } else {
        toast.error("Failed to revoke service account.");
      }
    },
  });

  const accounts: ServiceAccount[] = q.data ?? [];

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-medium">Service accounts</h2>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          New service account
        </Button>
      </div>

      {q.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : q.isError ? (
        <p className="text-sm text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load service accounts."}
        </p>
      ) : accounts.length === 0 ? (
        <div className="rounded-md border p-6 text-center text-sm text-muted-foreground">
          No service accounts for this user.
        </div>
      ) : (
        <ul className="space-y-2">
          {accounts.map((sa) => {
            const isConfirming = confirmKey === sa.access_key;
            return (
              <li
                key={sa.access_key}
                className="flex items-start justify-between gap-3 rounded-md border p-3"
              >
                <div className="space-y-1">
                  <div className="font-mono text-sm">{sa.access_key}</div>
                  {sa.name && <div className="text-sm font-medium">{sa.name}</div>}
                  {sa.description && (
                    <div className="text-xs text-muted-foreground">{sa.description}</div>
                  )}
                  {sa.attached_template && (
                    <div className="text-xs">
                      <span className="text-muted-foreground">Template: </span>
                      <span className="font-mono">{sa.attached_template.name}</span>
                      {sa.attached_template.params && (
                        <span className="text-muted-foreground">
                          {" "}
                          ({JSON.stringify(sa.attached_template.params)})
                        </span>
                      )}
                    </div>
                  )}
                </div>
                {isConfirming ? (
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setConfirmKey(null)}
                    >
                      Cancel
                    </Button>
                    <Button
                      type="button"
                      variant="destructive"
                      size="sm"
                      disabled={revoke.isPending}
                      onClick={() => revoke.mutate(sa.access_key)}
                    >
                      {revoke.isPending ? "Revoking…" : "Confirm revoke"}
                    </Button>
                  </div>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setConfirmKey(sa.access_key)}
                  >
                    Revoke
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
      )}

      <CreateServiceAccountDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        parentAccessKey={parentAccessKey}
      />
    </div>
  );
}
