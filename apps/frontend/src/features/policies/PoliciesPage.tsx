import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { AppError } from "@/lib/api/errors";
import { policiesKeys } from "@/lib/api/keys";
import type { Policy } from "./types";
import { deletePolicy, listPolicies } from "./policiesApi";
import { PolicyEditorDialog } from "./PolicyEditorDialog";

type EditorState =
  | { open: false }
  | { open: true; mode: "create" }
  | { open: true; mode: "edit"; policyName: string };

type DeleteState =
  | { open: false }
  | {
      open: true;
      policy: Policy;
      inUseDetails?: { users: string[]; groups: string[] };
    };

function OriginBadge({ origin }: { origin: Policy["origin"] }) {
  const label: Record<Policy["origin"], string> = {
    "minio-builtin": "MinIO built-in",
    "harbormaster-template": "Template",
    custom: "Custom",
  };
  return <Badge variant="outline">{label[origin]}</Badge>;
}

export function PoliciesPage() {
  const qc = useQueryClient();

  const [editorState, setEditorState] = useState<EditorState>({ open: false });
  const [deleteState, setDeleteState] = useState<DeleteState>({ open: false });

  const policiesQuery = useQuery({
    queryKey: policiesKeys.list(),
    queryFn: listPolicies,
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deletePolicy(name),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: policiesKeys.list() });
      toast.success("Policy deleted.");
      setDeleteState({ open: false });
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.code === "policy_in_use") {
          const details = err.details as
            | { attached_to?: { users?: string[]; groups?: string[] } }
            | undefined;
          const users = details?.attached_to?.users ?? [];
          const groups = details?.attached_to?.groups ?? [];
          // Show the in-use details inside the confirm dialog
          setDeleteState((prev) =>
            prev.open ? { ...prev, inUseDetails: { users, groups } } : prev,
          );
          return;
        }
        if (err.code === "policy_read_only") {
          toast.error("This policy cannot be deleted.");
          setDeleteState({ open: false });
          return;
        }
        toast.error(err.message || "Failed to delete policy.");
        return;
      }
      toast.error("Failed to delete policy.");
    },
  });

  const openEditor = (mode: "create" | "edit", policyName?: string) => {
    if (mode === "create") {
      setEditorState({ open: true, mode: "create" });
    } else if (policyName) {
      setEditorState({ open: true, mode: "edit", policyName });
    }
  };

  const openDelete = (policy: Policy) => {
    setDeleteState({ open: true, policy });
  };

  const policies = policiesQuery.data ?? [];

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Policies</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage IAM policies. Built-in and template policies are read-only; custom policies can
            be created, edited, and deleted.
          </p>
        </div>
        <Button onClick={() => openEditor("create")}>New policy</Button>
      </div>

      {policiesQuery.isLoading ? (
        <p className="text-muted-foreground">Loading…</p>
      ) : policiesQuery.isError ? (
        <p className="text-destructive">
          {policiesQuery.error instanceof AppError
            ? policiesQuery.error.message
            : "Failed to load policies."}
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Origin</TableHead>
              <TableHead>Summary</TableHead>
              <TableHead className="w-[140px]">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {policies.length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="text-center text-muted-foreground py-8">
                  No policies found.
                </TableCell>
              </TableRow>
            ) : (
              policies.map((policy) => (
                <TableRow key={policy.name}>
                  <TableCell className="font-mono">{policy.name}</TableCell>
                  <TableCell>
                    <OriginBadge origin={policy.origin} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground max-w-xs truncate">
                    {policy.statement_summary}
                  </TableCell>
                  <TableCell>
                    {policy.editable && (
                      <div className="flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openEditor("edit", policy.name)}
                        >
                          Edit
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => openDelete(policy)}
                        >
                          Delete
                        </Button>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      )}

      {/* Editor dialog */}
      {editorState.open && editorState.mode === "create" && (
        <PolicyEditorDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setEditorState({ open: false });
          }}
          mode="create"
        />
      )}
      {editorState.open && editorState.mode === "edit" && (
        <PolicyEditorDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setEditorState({ open: false });
          }}
          mode="edit"
          policyName={editorState.policyName}
        />
      )}

      {/* Delete confirm dialog */}
      {deleteState.open && (
        <Dialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setDeleteState({ open: false });
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Delete policy</DialogTitle>
              <DialogDescription>
                Are you sure you want to delete{" "}
                <span className="font-mono font-semibold">{deleteState.policy.name}</span>? This
                action cannot be undone.
              </DialogDescription>
            </DialogHeader>

            {deleteState.inUseDetails && (
              <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm space-y-2">
                <p className="font-semibold text-destructive">
                  Policy is still in use and cannot be deleted.
                </p>
                {deleteState.inUseDetails.users.length > 0 && (
                  <div>
                    <p className="font-medium">Attached to users:</p>
                    <ul className="list-disc list-inside text-muted-foreground">
                      {deleteState.inUseDetails.users.map((u) => (
                        <li key={u} className="font-mono">
                          {u}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                {deleteState.inUseDetails.groups.length > 0 && (
                  <div>
                    <p className="font-medium">Attached to groups:</p>
                    <ul className="list-disc list-inside text-muted-foreground">
                      {deleteState.inUseDetails.groups.map((g) => (
                        <li key={g} className="font-mono">
                          {g}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )}

            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setDeleteState({ open: false })}
                disabled={deleteMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => deleteMutation.mutate(deleteState.policy.name)}
                disabled={deleteMutation.isPending || !!deleteState.inUseDetails}
              >
                {deleteMutation.isPending ? "Deleting…" : "Delete"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
