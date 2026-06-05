import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
import { policyTemplatesKeys, usersKeys } from "@/lib/api/keys";
import { listPolicyTemplates } from "@/features/policies/api";
import type { PolicyTemplate } from "@/features/policies/types";
import { updateUserPolicies } from "./api";
import type { TemplateRef } from "./types";

type Selection = {
  selected: boolean;
  params: Record<string, string>;
};

type SelectionState = Record<string, Selection>;

function templatesToState(templates: TemplateRef[]): SelectionState {
  const out: SelectionState = {};
  for (const t of templates) {
    out[t.name] = {
      selected: true,
      params: { ...(t.params ?? {}) },
    };
  }
  return out;
}

function requiredParamsFor(tpl: PolicyTemplate): string[] {
  return tpl.params_schema?.required ?? [];
}

export type EditPoliciesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accessKey: string;
  current: TemplateRef[];
  currentPolicies?: string[];
};

export function EditPoliciesDialog({
  open,
  onOpenChange,
  accessKey,
  current,
  currentPolicies = [],
}: EditPoliciesDialogProps) {
  const qc = useQueryClient();
  const tplQ = useQuery({
    queryKey: policyTemplatesKeys.list(),
    queryFn: listPolicyTemplates,
    enabled: open,
  });

  const [state, setState] = useState<SelectionState>(() => templatesToState(current));

  useEffect(() => {
    if (open) setState(templatesToState(current));
  }, [open, current]);

  const templates = tplQ.data ?? [];

  const errors = useMemo(() => {
    const errs: Record<string, string> = {};
    for (const tpl of templates) {
      const sel = state[tpl.name];
      if (!sel?.selected) continue;
      for (const req of requiredParamsFor(tpl)) {
        const v = sel.params[req];
        if (!v || v.trim().length === 0) {
          errs[`${tpl.name}.${req}`] = `${req} is required`;
        }
      }
    }
    return errs;
  }, [templates, state]);

  const mutation = useMutation({
    mutationFn: () => {
      const payload: TemplateRef[] = templates
        .filter((tpl) => state[tpl.name]?.selected)
        .map((tpl) => {
          const sel = state[tpl.name];
          const required = requiredParamsFor(tpl);
          if (required.length === 0) {
            return { name: tpl.name };
          }
          const params: Record<string, string> = {};
          for (const k of required) params[k] = sel?.params[k] ?? "";
          return { name: tpl.name, params };
        });
      return updateUserPolicies(accessKey, payload, currentPolicies);
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: usersKeys.detail(accessKey) });
      await qc.invalidateQueries({ queryKey: usersKeys.list() });
      toast.success("Policies updated.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        toast.error(err.message || "Failed to update policies.");
      } else {
        toast.error("Failed to update policies.");
      }
    },
  });

  const canSubmit = Object.keys(errors).length === 0 && !mutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit attached templates</DialogTitle>
          <DialogDescription>
            Select bundled policy templates to attach to {accessKey}.
          </DialogDescription>
        </DialogHeader>

        {tplQ.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading templates…</p>
        ) : tplQ.isError ? (
          <p className="text-sm text-destructive">Failed to load templates.</p>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (canSubmit) mutation.mutate();
            }}
            className="space-y-4"
            noValidate
          >
            <ul className="space-y-3">
              {templates.map((tpl) => {
                const sel = state[tpl.name] ?? { selected: false, params: {} };
                const required = requiredParamsFor(tpl);
                return (
                  <li key={tpl.name} className="rounded-md border p-3">
                    <div className="flex items-start gap-2">
                      <input
                        id={`tpl-${tpl.name}`}
                        type="checkbox"
                        className="mt-1 h-4 w-4"
                        checked={sel.selected}
                        onChange={(e) =>
                          setState((s) => ({
                            ...s,
                            [tpl.name]: {
                              selected: e.target.checked,
                              params: s[tpl.name]?.params ?? {},
                            },
                          }))
                        }
                      />
                      <div className="flex-1 space-y-1">
                        <Label htmlFor={`tpl-${tpl.name}`} className="font-medium">
                          {tpl.name}
                        </Label>
                        <p className="text-xs text-muted-foreground">{tpl.description}</p>
                      </div>
                    </div>

                    {sel.selected && required.length > 0 && (
                      <div className="mt-3 space-y-2 pl-6">
                        {required.map((paramName) => {
                          const fieldId = `tpl-${tpl.name}-${paramName}`;
                          const errKey = `${tpl.name}.${paramName}`;
                          return (
                            <div key={paramName} className="space-y-1">
                              <Label htmlFor={fieldId}>{paramName}</Label>
                              <Input
                                id={fieldId}
                                autoComplete="off"
                                value={sel.params[paramName] ?? ""}
                                onChange={(e) =>
                                  setState((s) => ({
                                    ...s,
                                    [tpl.name]: {
                                      selected: true,
                                      params: {
                                        ...(s[tpl.name]?.params ?? {}),
                                        [paramName]: e.target.value,
                                      },
                                    },
                                  }))
                                }
                              />
                              {errors[errKey] && (
                                <p className="text-xs text-destructive">{errors[errKey]}</p>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!canSubmit}>
                {mutation.isPending ? "Saving…" : "Save policies"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
