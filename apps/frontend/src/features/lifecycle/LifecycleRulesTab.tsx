import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AppError } from "@/lib/api/errors";
import { lifecycleKeys } from "@/lib/api/keys";
import { CreateRuleDialog } from "./CreateRuleDialog";
import { deleteRule, listRules, type LifecycleRule } from "./api";

export type LifecycleRulesTabProps = {
  bucket: string;
};

export function LifecycleRulesTab({ bucket }: LifecycleRulesTabProps) {
  const qc = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);

  const q = useQuery({
    queryKey: lifecycleKeys.list(bucket),
    queryFn: () => listRules(bucket),
    enabled: bucket.length > 0,
  });

  const delMutation = useMutation({
    mutationFn: (id: string) => deleteRule(bucket, id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: lifecycleKeys.list(bucket) });
      toast.success("Rule deleted.");
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Failed to delete rule.");
      else toast.error("Failed to delete rule.");
    },
  });

  if (q.isLoading) {
    return (
      <Card>
        <CardContent className="p-6 text-sm text-muted-foreground">Loading…</CardContent>
      </Card>
    );
  }
  if (q.isError) {
    return (
      <Card>
        <CardContent className="p-6 text-sm text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load lifecycle rules."}
        </CardContent>
      </Card>
    );
  }

  const rules: LifecycleRule[] = q.data ?? [];
  const managed = rules.filter((r) => r.managed);
  const unmanaged = rules.filter((r) => !r.managed);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-end">
        <Button type="button" onClick={() => setCreateOpen(true)}>
          Add rule
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Managed rules</CardTitle>
        </CardHeader>
        <CardContent>
          {managed.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No managed rules. Click &quot;Add rule&quot; to create one.
            </p>
          ) : (
            <ul className="divide-y" data-testid="managed-rule-list">
              {managed.map((r) => (
                <li
                  key={r.id}
                  className="flex items-center justify-between py-2 text-sm"
                  data-testid="managed-rule"
                >
                  <div className="space-y-0.5">
                    <div>
                      <span className="font-mono">{r.id}</span>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      Expire after {r.days ?? "—"} day(s)
                      {r.prefix ? (
                        <>
                          {" "}
                          for prefix <span className="font-mono">{r.prefix}</span>
                        </>
                      ) : (
                        <> (whole bucket)</>
                      )}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={delMutation.isPending}
                    onClick={() => delMutation.mutate(r.id)}
                  >
                    Delete
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Unmanaged rules</CardTitle>
        </CardHeader>
        <CardContent>
          {unmanaged.length === 0 ? (
            <p className="text-sm text-muted-foreground">No unmanaged rules.</p>
          ) : (
            <ul className="divide-y" data-testid="unmanaged-rule-list">
              {unmanaged.map((r) => (
                <li key={r.id} className="py-2 text-sm" data-testid="unmanaged-rule">
                  <div>
                    <span className="font-mono">{r.id}</span>
                  </div>
                  <div className="text-xs text-muted-foreground">{r.summary ?? ""}</div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <CreateRuleDialog open={createOpen} onOpenChange={setCreateOpen} bucket={bucket} />
    </div>
  );
}
