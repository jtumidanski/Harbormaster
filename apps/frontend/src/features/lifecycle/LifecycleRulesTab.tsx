import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AppError } from "@/lib/api/errors";
import { lifecycleKeys } from "@/lib/api/keys";
import { cn } from "@/lib/utils";
import { CreateRuleDialog } from "./CreateRuleDialog";
import { deleteRule, listRules, type LifecycleRule } from "./api";

export type LifecycleRulesTabProps = {
  bucket: string;
  versioningEnabled?: boolean;
};

// ---------------------------------------------------------------------------
// Kind badge helpers
// ---------------------------------------------------------------------------

type KindBadgeProps = { kind: string | undefined };

function kindLabel(kind: string | undefined): string {
  switch (kind) {
    case "expiration":
      return "Expiration";
    case "noncurrent-expiration":
      return "Noncurrent";
    case "abort-incomplete-multipart":
      return "Abort MPU";
    default:
      return kind ?? "Unknown";
  }
}

function kindBadgeClass(kind: string | undefined): string {
  switch (kind) {
    case "expiration":
      return "bg-blue-100 text-blue-900 dark:bg-blue-900/30 dark:text-blue-200 border-blue-200 dark:border-blue-700";
    case "noncurrent-expiration":
      return "bg-violet-100 text-violet-900 dark:bg-violet-900/30 dark:text-violet-200 border-violet-200 dark:border-violet-700";
    case "abort-incomplete-multipart":
      return "bg-orange-100 text-orange-900 dark:bg-orange-900/30 dark:text-orange-200 border-orange-200 dark:border-orange-700";
    default:
      return "bg-muted text-muted-foreground";
  }
}

function KindBadge({ kind }: KindBadgeProps) {
  return (
    <Badge variant="outline" className={cn(kindBadgeClass(kind))}>
      {kindLabel(kind)}
    </Badge>
  );
}

// ---------------------------------------------------------------------------
// Kind-specific summary line
// ---------------------------------------------------------------------------

function ManagedRuleSummary({ rule }: { rule: LifecycleRule }) {
  const { kind, days, noncurrent_days, newer_noncurrent_versions, days_after_initiation, prefix } =
    rule;

  let detail: string;
  switch (kind) {
    case "expiration":
      detail = `Expire objects after ${days ?? "—"} day(s)`;
      break;
    case "noncurrent-expiration":
      detail = `Keep ${newer_noncurrent_versions ?? 0} newest, expire after ${noncurrent_days ?? "—"} day(s)`;
      break;
    case "abort-incomplete-multipart":
      detail = `Abort incomplete uploads after ${days_after_initiation ?? "—"} day(s)`;
      break;
    default:
      detail = "Unknown rule kind";
  }

  return (
    <div className="text-xs text-muted-foreground">
      {detail}
      {prefix ? (
        <>
          {" "}
          for prefix <span className="font-mono">{prefix}</span>
        </>
      ) : (
        <> (whole bucket)</>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tab component
// ---------------------------------------------------------------------------

export function LifecycleRulesTab({ bucket, versioningEnabled }: LifecycleRulesTabProps) {
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
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-mono">{r.id}</span>
                      <KindBadge kind={r.kind} />
                    </div>
                    <ManagedRuleSummary rule={r} />
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

      <CreateRuleDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        bucket={bucket}
        {...(versioningEnabled !== undefined ? { versioningEnabled } : {})}
      />
    </div>
  );
}
