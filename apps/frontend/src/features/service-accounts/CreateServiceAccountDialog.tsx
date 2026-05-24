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
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AppError } from "@/lib/api/errors";
import { policyTemplatesKeys, serviceAccountsKeys } from "@/lib/api/keys";
import { listPolicyTemplates } from "@/features/policies/api";
import { SecretRevealCard } from "@/features/users/SecretRevealCard";
import type { TemplateRef } from "@/features/users/types";
import {
  createServiceAccount,
  type CreateServiceAccountAttrs,
  type CreateServiceAccountInput,
} from "./api";

const formSchema = z.object({
  name: z.string().min(1, "Name is required."),
  description: z.string(),
  template_name: z.string(),
});

type FormValues = z.infer<typeof formSchema>;

const INHERIT_VALUE = "__inherit__";

export type CreateServiceAccountDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentAccessKey: string;
};

export function CreateServiceAccountDialog({
  open,
  onOpenChange,
  parentAccessKey,
}: CreateServiceAccountDialogProps) {
  const qc = useQueryClient();
  const [created, setCreated] = useState<CreateServiceAccountAttrs | null>(null);
  const [overrideParams, setOverrideParams] = useState<Record<string, string>>({});

  const tplQ = useQuery({
    queryKey: policyTemplatesKeys.list(),
    queryFn: listPolicyTemplates,
    enabled: open && created === null,
  });

  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: { name: "", description: "", template_name: INHERIT_VALUE },
    mode: "onSubmit",
  });

  useEffect(() => {
    if (!open) {
      form.reset();
      setCreated(null);
      setOverrideParams({});
    }
  }, [open, form]);

  const templates = tplQ.data ?? [];
  const selectedTplName = form.watch("template_name");
  const selectedTpl = templates.find((t) => t.name === selectedTplName);
  const requiredParams = selectedTpl?.params_schema?.required ?? [];

  const paramErrors = useMemo(() => {
    const errs: Record<string, string> = {};
    if (selectedTplName === INHERIT_VALUE) return errs;
    for (const p of requiredParams) {
      const v = overrideParams[p];
      if (!v || v.trim().length === 0) errs[p] = `${p} is required`;
    }
    return errs;
  }, [selectedTplName, requiredParams, overrideParams]);

  const mutation = useMutation({
    mutationFn: (values: FormValues) => {
      let template_override: TemplateRef | null = null;
      if (values.template_name !== INHERIT_VALUE) {
        if (requiredParams.length === 0) {
          template_override = { name: values.template_name };
        } else {
          const params: Record<string, string> = {};
          for (const p of requiredParams) params[p] = overrideParams[p] ?? "";
          template_override = { name: values.template_name, params };
        }
      }
      const payload: CreateServiceAccountInput = {
        name: values.name,
        description: values.description,
        template_override,
      };
      return createServiceAccount(parentAccessKey, payload);
    },
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: serviceAccountsKeys.forUser(parentAccessKey) });
      setCreated(data);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        toast.error(err.message || "Failed to create service account.");
      } else {
        toast.error("Failed to create service account.");
      }
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
              <DialogTitle>Service account created: {created.access_key}</DialogTitle>
              <DialogDescription>
                Save the secret key below before closing this dialog.
              </DialogDescription>
            </DialogHeader>
            <SecretRevealCard secret={created.secret_key} onAcknowledge={acknowledge} />
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Create service account</DialogTitle>
              <DialogDescription>
                Issue a long-term credential owned by {parentAccessKey}.
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
                  name="name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Name</FormLabel>
                      <FormControl>
                        <Input autoComplete="off" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="description"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Description</FormLabel>
                      <FormControl>
                        <Input autoComplete="off" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="template_name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Template override</FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value={INHERIT_VALUE}>Inherit from parent user</SelectItem>
                          {templates.map((tpl) => (
                            <SelectItem key={tpl.name} value={tpl.name}>
                              {tpl.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {selectedTplName !== INHERIT_VALUE && requiredParams.length > 0 && (
                  <div className="space-y-2">
                    {requiredParams.map((p) => {
                      const id = `sa-override-${p}`;
                      return (
                        <div key={p} className="space-y-1">
                          <Label htmlFor={id}>{p}</Label>
                          <Input
                            id={id}
                            autoComplete="off"
                            value={overrideParams[p] ?? ""}
                            onChange={(e) =>
                              setOverrideParams((s) => ({ ...s, [p]: e.target.value }))
                            }
                          />
                          {paramErrors[p] && (
                            <p className="text-xs text-destructive">{paramErrors[p]}</p>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}

                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                    Cancel
                  </Button>
                  <Button type="submit" disabled={!canSubmit}>
                    {mutation.isPending ? "Creating…" : "Create service account"}
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
