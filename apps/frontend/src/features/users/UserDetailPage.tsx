import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { AppError } from "@/lib/api/errors";
import { usersKeys } from "@/lib/api/keys";
import { ServiceAccountList } from "@/features/service-accounts/ServiceAccountList";
import { getUser, setUserStatus } from "./api";
import type { TemplateRef, User } from "./types";
import { DeleteUserDialog } from "./DeleteUserDialog";
import { EditPoliciesDialog } from "./EditPoliciesDialog";

function TemplateChip({ tpl }: { tpl: TemplateRef }) {
  const params =
    tpl.params && Object.keys(tpl.params).length > 0
      ? ` (${Object.entries(tpl.params)
          .map(([k, v]) => `${k}=${v}`)
          .join(", ")})`
      : "";
  return (
    <span className="inline-flex items-center rounded-full border bg-muted/40 px-2 py-0.5 font-mono text-xs">
      {tpl.name}
      {params}
    </span>
  );
}

export function UserDetailPage() {
  const { accessKey: rawAccessKey = "" } = useParams<{ accessKey: string }>();
  const accessKey = decodeURIComponent(rawAccessKey);
  const qc = useQueryClient();
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const q = useQuery({
    queryKey: usersKeys.detail(accessKey),
    queryFn: () => getUser(accessKey),
    enabled: accessKey.length > 0,
  });

  const statusMutation = useMutation({
    mutationFn: (enabled: boolean) => setUserStatus(accessKey, enabled),
    onMutate: async (enabled: boolean) => {
      await qc.cancelQueries({ queryKey: usersKeys.detail(accessKey) });
      const prev = qc.getQueryData<User>(usersKeys.detail(accessKey));
      if (prev) {
        qc.setQueryData<User>(usersKeys.detail(accessKey), {
          ...prev,
          status: enabled ? "enabled" : "disabled",
        });
      }
      return { prev };
    },
    onError: (err: unknown, _enabled, ctx) => {
      if (ctx?.prev) qc.setQueryData(usersKeys.detail(accessKey), ctx.prev);
      if (err instanceof AppError) {
        toast.error(err.message || "Failed to update status.");
      } else {
        toast.error("Failed to update status.");
      }
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: usersKeys.detail(accessKey) });
      await qc.invalidateQueries({ queryKey: usersKeys.list() });
      toast.success("Status updated.");
    },
  });

  if (!accessKey) return <div className="p-6">Missing access key.</div>;
  if (q.isLoading) return <div className="p-6 text-muted-foreground">Loading…</div>;
  if (q.isError || !q.data) {
    return (
      <div className="p-6 space-y-3">
        <p className="text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load user."}
        </p>
        <Link to="/users" className="text-sm text-primary hover:underline">
          Back to users
        </Link>
      </div>
    );
  }

  const user = q.data;
  const enabled = user.status === "enabled";

  return (
    <div className="p-6 space-y-4">
      <div>
        <Link to="/users" className="text-sm text-muted-foreground hover:underline">
          ← Users
        </Link>
        <div className="mt-1 flex items-center justify-between gap-3">
          <h1 className="font-mono text-2xl font-semibold">{user.access_key}</h1>
          <div className="flex items-center gap-2">
            <input
              id="user-status"
              type="checkbox"
              className="h-4 w-4"
              checked={enabled}
              disabled={statusMutation.isPending}
              onChange={(e) => statusMutation.mutate(e.target.checked)}
            />
            <Label htmlFor="user-status" className="font-normal">
              {enabled ? "Enabled" : "Disabled"}
            </Label>
          </div>
        </div>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle className="text-base">Attached templates</CardTitle>
          <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
            Edit templates
          </Button>
        </CardHeader>
        <CardContent>
          {user.attached_templates.length === 0 ? (
            <p className="text-sm text-muted-foreground">No templates attached.</p>
          ) : (
            <div className="flex flex-wrap gap-1">
              {user.attached_templates.map((t) => (
                <TemplateChip key={t.name} tpl={t} />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Other policies</CardTitle>
        </CardHeader>
        <CardContent>
          {user.other_policies.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No other MinIO policies attached outside of Harbormaster templates.
            </p>
          ) : (
            <>
              <p className="mb-2 text-xs text-muted-foreground">
                These policies were attached outside Harbormaster and are read-only here.
              </p>
              <ul className="space-y-1">
                {user.other_policies.map((p) => (
                  <li key={p} className="font-mono text-sm">
                    {p}
                  </li>
                ))}
              </ul>
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <ServiceAccountList parentAccessKey={user.access_key} />
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button variant="destructive" onClick={() => setDeleteOpen(true)}>
          Delete user
        </Button>
      </div>

      {/* eslint-disable-next-line jsx-a11y/no-access-key */}
      <EditPoliciesDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        accessKey={user.access_key}
        current={user.attached_templates}
      />
      {/* eslint-disable-next-line jsx-a11y/no-access-key */}
      <DeleteUserDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        accessKey={user.access_key}
      />
    </div>
  );
}
