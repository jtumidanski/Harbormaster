import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { z } from "zod";
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
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { AppError } from "@/lib/api/errors";
import { policyTemplatesKeys, usersKeys } from "@/lib/api/keys";
import { listPolicyTemplates } from "@/features/policies/api";
import type { PolicyTemplate } from "@/features/policies/types";
import { createUser } from "./api";
import { SecretRevealCard } from "./SecretRevealCard";
import type { CreateUserResponseAttrs, TemplateRef } from "./types";

const createUserSchema = z.object({
  access_key: z
    .string()
    .min(3, "Access key must be at least 3 characters.")
    .max(64, "Access key must be at most 64 characters."),
});

type FormValues = z.infer<typeof createUserSchema>;

type Selection = { selected: boolean; params: Record<string, string> };
type SelectionState = Record<string, Selection>;

function requiredParamsFor(tpl: PolicyTemplate): string[] {
  return tpl.params_schema?.required ?? [];
}

function pointerToField(pointer: string | undefined): keyof FormValues | null {
  if (!pointer) return null;
  if (pointer.endsWith("/access_key")) return "access_key";
  return null;
}

export type CreateUserDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateUserDialog({ open, onOpenChange }: CreateUserDialogProps) {
  const qc = useQueryClient();
  const [created, setCreated] = useState<CreateUserResponseAttrs | null>(null);
  const [selection, setSelection] = useState<SelectionState>({});

  const tplQ = useQuery({
    queryKey: policyTemplatesKeys.list(),
    queryFn: listPolicyTemplates,
    enabled: open && created === null,
  });

  const form = useForm<FormValues>({
    resolver: zodResolver(createUserSchema),
    defaultValues: { access_key: "" },
    mode: "onSubmit",
  });

  useEffect(() => {
    if (!open) {
      form.reset();
      setCreated(null);
      setSelection({});
    }
  }, [open, form]);

  const templates = tplQ.data ?? [];

  const paramErrors = useMemo(() => {
    const errs: Record<string, string> = {};
    for (const tpl of templates) {
      const sel = selection[tpl.name];
      if (!sel?.selected) continue;
      for (const req of requiredParamsFor(tpl)) {
        const v = sel.params[req];
        if (!v || v.trim().length === 0) {
          errs[`${tpl.name}.${req}`] = `${req} is required`;
        }
      }
    }
    return errs;
  }, [templates, selection]);

  const mutation = useMutation({
    mutationFn: (values: FormValues) => {
      const payloadTemplates: TemplateRef[] = templates
        .filter((tpl) => selection[tpl.name]?.selected)
        .map((tpl) => {
          const sel = selection[tpl.name];
          const required = requiredParamsFor(tpl);
          if (required.length === 0) return { name: tpl.name };
          const params: Record<string, string> = {};
          for (const k of required) params[k] = sel?.params[k] ?? "";
          return { name: tpl.name, params };
        });
      return createUser({
        access_key: values.access_key,
        templates: payloadTemplates,
      });
    },
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: usersKeys.all() });
      setCreated(data);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.status === 422) {
          const field = pointerToField(err.pointer);
          if (field) {
            form.setError(field, { type: "server", message: err.message });
            return;
          }
        }
        toast.error(err.message || "Failed to create user.");
        return;
      }
      toast.error("Failed to create user.");
    },
  });

  function acknowledge() {
    onOpenChange(false);
  }

  const canSubmit = Object.keys(paramErrors).length === 0 && !mutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>User created: {created.access_key}</DialogTitle>
              <DialogDescription>
                Save the secret key below before closing this dialog.
              </DialogDescription>
            </DialogHeader>
            <SecretRevealCard secret={created.secret_key} onAcknowledge={acknowledge} />
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Create user</DialogTitle>
              <DialogDescription>
                Create a new MinIO IAM user with optional policy templates.
              </DialogDescription>
            </DialogHeader>
            <Form {...form}>
              <form
                onSubmit={(e) => {
                  void form.handleSubmit((values) => {
                    if (canSubmit) mutation.mutate(values);
                  })(e);
                }}
                className="space-y-4"
                noValidate
              >
                <FormField
                  control={form.control}
                  name="access_key"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Access key</FormLabel>
                      <FormControl>
                        <Input autoComplete="off" {...field} />
                      </FormControl>
                      <FormDescription>The MinIO IAM username (3–64 characters).</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className="space-y-2">
                  <div className="text-sm font-medium">Attached templates</div>
                  {tplQ.isLoading ? (
                    <p className="text-sm text-muted-foreground">Loading templates…</p>
                  ) : tplQ.isError ? (
                    <p className="text-sm text-destructive">Failed to load templates.</p>
                  ) : (
                    <ul className="space-y-2">
                      {templates.map((tpl) => {
                        const sel = selection[tpl.name] ?? { selected: false, params: {} };
                        const required = requiredParamsFor(tpl);
                        return (
                          <li key={tpl.name} className="rounded-md border p-3">
                            <div className="flex items-start gap-2">
                              <input
                                id={`new-tpl-${tpl.name}`}
                                type="checkbox"
                                className="mt-1 h-4 w-4"
                                checked={sel.selected}
                                onChange={(e) =>
                                  setSelection((s) => ({
                                    ...s,
                                    [tpl.name]: {
                                      selected: e.target.checked,
                                      params: s[tpl.name]?.params ?? {},
                                    },
                                  }))
                                }
                              />
                              <div className="flex-1">
                                <Label htmlFor={`new-tpl-${tpl.name}`} className="font-medium">
                                  {tpl.name}
                                </Label>
                                <p className="text-xs text-muted-foreground">{tpl.description}</p>
                              </div>
                            </div>
                            {sel.selected && required.length > 0 && (
                              <div className="mt-2 space-y-2 pl-6">
                                {required.map((paramName) => {
                                  const id = `new-tpl-${tpl.name}-${paramName}`;
                                  const errKey = `${tpl.name}.${paramName}`;
                                  return (
                                    <div key={paramName} className="space-y-1">
                                      <Label htmlFor={id}>{paramName}</Label>
                                      <Input
                                        id={id}
                                        autoComplete="off"
                                        value={sel.params[paramName] ?? ""}
                                        onChange={(e) =>
                                          setSelection((s) => ({
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
                                      {paramErrors[errKey] && (
                                        <p className="text-xs text-destructive">
                                          {paramErrors[errKey]}
                                        </p>
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
                  )}
                </div>

                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                    Cancel
                  </Button>
                  <Button type="submit" disabled={!canSubmit}>
                    {mutation.isPending ? "Creating…" : "Create user"}
                  </Button>
                </DialogFooter>
              </form>
            </Form>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
