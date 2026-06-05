import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { z } from "zod";
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
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { AppError } from "@/lib/api/errors";
import { policiesKeys } from "@/lib/api/keys";
import { createPolicy, getPolicy, updatePolicy } from "./policiesApi";

const editorSchema = z.object({
  name: z.string().min(1, "Name is required."),
  document: z.string().min(1, "Document is required."),
});

type FormValues = z.infer<typeof editorSchema>;

const DEFAULT_VALUES: FormValues = {
  name: "",
  document: "",
};

export type PolicyEditorDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: "create" | "edit";
  policyName?: string;
};

export function PolicyEditorDialog({
  open,
  onOpenChange,
  mode,
  policyName,
}: PolicyEditorDialogProps) {
  const qc = useQueryClient();

  const form = useForm<FormValues>({
    resolver: zodResolver(editorSchema),
    defaultValues: DEFAULT_VALUES,
    mode: "onSubmit",
  });

  // In edit mode, prefill the document from the server
  const detailQuery = useQuery({
    queryKey: policiesKeys.detail(policyName ?? ""),
    queryFn: () => getPolicy(policyName!),
    enabled: mode === "edit" && !!policyName && open,
  });

  useEffect(() => {
    if (!open) {
      form.reset(DEFAULT_VALUES);
    }
  }, [open, form]);

  useEffect(() => {
    if (mode === "edit" && detailQuery.data) {
      form.setValue("document", JSON.stringify(detailQuery.data.document, null, 2));
      if (policyName) {
        form.setValue("name", policyName);
      }
    }
  }, [mode, detailQuery.data, policyName, form]);

  const mutation = useMutation({
    mutationFn: async (payload: { name: string; parsed: unknown }) => {
      if (mode === "create") {
        await createPolicy(payload.name, payload.parsed);
      } else {
        await updatePolicy(policyName!, payload.parsed);
      }
    },
    onSuccess: async (_, payload) => {
      const name = mode === "edit" ? policyName! : payload.name;
      await qc.invalidateQueries({ queryKey: policiesKeys.list() });
      await qc.invalidateQueries({ queryKey: policiesKeys.detail(name) });
      toast.success(mode === "create" ? "Policy created." : "Policy updated.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.pointer === "/data/attributes/document" || err.pointer === "/document") {
          form.setError("document", { type: "server", message: err.message });
          return;
        }
        if (err.pointer === "/data/attributes/name") {
          form.setError("name", { type: "server", message: err.message });
          return;
        }
        toast.error(err.message || "An error occurred.");
        return;
      }
      toast.error("An error occurred.");
    },
  });

  const isLoading = mode === "edit" && detailQuery.isLoading;

  function handleSubmit(values: FormValues) {
    // Client-side JSON parse BEFORE calling the API
    let parsed: unknown;
    try {
      parsed = JSON.parse(values.document);
    } catch {
      form.setError("document", {
        type: "client",
        message: "Document is not valid JSON.",
      });
      return;
    }
    mutation.mutate({ name: values.name, parsed });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{mode === "create" ? "Create policy" : "Edit policy"}</DialogTitle>
          <DialogDescription>
            {mode === "create"
              ? "Define a new IAM policy in JSON format."
              : "Edit the JSON document for this policy."}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={(e) => {
              void form.handleSubmit(handleSubmit)(e);
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
                    <Input
                      placeholder="my-custom-policy"
                      autoComplete="off"
                      disabled={mode === "edit"}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="document"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Document</FormLabel>
                  <FormControl>
                    <Textarea
                      className="font-mono min-h-[200px]"
                      placeholder={'{\n  "Version": "2012-10-17",\n  "Statement": []\n}'}
                      disabled={isLoading}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutation.isPending || isLoading}>
                {mutation.isPending
                  ? mode === "create"
                    ? "Creating…"
                    : "Saving…"
                  : mode === "create"
                    ? "Create"
                    : "Save"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
