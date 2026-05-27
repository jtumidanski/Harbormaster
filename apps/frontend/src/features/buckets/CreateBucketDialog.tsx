import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { AppError } from "@/lib/api/errors";
import { bucketsKeys } from "@/lib/api/keys";
import { createBucket, type CreateBucketRequest } from "./api";
import type { PublicAccess, QuotaKind } from "./types";

const UNIT_MULTIPLIERS = {
  MiB: 1024n * 1024n,
  GiB: 1024n * 1024n * 1024n,
  TiB: 1024n * 1024n * 1024n * 1024n,
} as const;

type Unit = keyof typeof UNIT_MULTIPLIERS;

const createBucketSchema = z
  .object({
    name: z
      .string()
      .min(3, "Name must be at least 3 characters.")
      .max(63, "Name must be at most 63 characters."),
    versioning_enabled: z.boolean(),
    public_access: z.enum(["private", "public-read", "public-read-write"]),
    quota_enabled: z.boolean(),
    quota_kind: z.enum(["hard", "fifo"]),
    quota_value: z.string(),
    quota_unit: z.enum(["MiB", "GiB", "TiB"]),
    lifecycle_template: z.literal("none"),
  })
  .superRefine((values, ctx) => {
    if (values.quota_enabled) {
      const n = Number(values.quota_value);
      if (!Number.isFinite(n) || n <= 0) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["quota_value"],
          message: "Enter a positive number.",
        });
      }
    }
  });

type FormValues = z.infer<typeof createBucketSchema>;

function quotaToBytes(value: string, unit: Unit): number {
  const n = Number(value);
  const big = BigInt(Math.trunc(n)) * UNIT_MULTIPLIERS[unit];
  // Clamp to JS Number safe range — bucket quotas in practice are well below this.
  const cap = BigInt(Number.MAX_SAFE_INTEGER);
  const clamped = big > cap ? cap : big;
  return Number(clamped);
}

function pointerToField(pointer: string | undefined): keyof FormValues | null {
  if (!pointer) return null;
  // e.g. "/data/attributes/name" -> "name"
  const parts = pointer.split("/");
  const last = parts[parts.length - 1];
  if (!last) return null;
  if (last === "name") return "name";
  if (last === "versioning_enabled") return "versioning_enabled";
  if (last === "public_access") return "public_access";
  return null;
}

export type CreateBucketDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateBucketDialog({ open, onOpenChange }: CreateBucketDialogProps) {
  const qc = useQueryClient();
  const form = useForm<FormValues>({
    resolver: zodResolver(createBucketSchema),
    defaultValues: {
      name: "",
      versioning_enabled: false,
      public_access: "private",
      quota_enabled: false,
      quota_kind: "hard",
      quota_value: "",
      quota_unit: "GiB",
      lifecycle_template: "none",
    },
    mode: "onSubmit",
  });

  useEffect(() => {
    if (!open) form.reset();
  }, [open, form]);

  const mutation = useMutation({
    mutationFn: (values: FormValues) => {
      const payload: CreateBucketRequest = {
        name: values.name,
        versioning_enabled: values.versioning_enabled,
        public_access: values.public_access,
        lifecycle_template: null,
      };
      if (values.quota_enabled) {
        payload.quota = {
          kind: values.quota_kind,
          bytes: quotaToBytes(values.quota_value, values.quota_unit),
        };
      }
      return createBucket(payload);
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: bucketsKeys.all() });
      toast.success("Bucket created.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.status === 422) {
          const field = pointerToField(err.pointer);
          if (field) {
            form.setError(field, { type: "server", message: err.message });
            return;
          }
          if (err.code === "invalid_bucket_name") {
            form.setError("name", { type: "server", message: err.message });
            return;
          }
        }
        toast.error(err.message || "Failed to create bucket.");
        return;
      }
      toast.error("Failed to create bucket.");
    },
  });

  const quotaEnabled = form.watch("quota_enabled");
  const versioningEnabled = form.watch("versioning_enabled");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create bucket</DialogTitle>
          <DialogDescription>
            Create a new bucket on the configured MinIO endpoint.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={(e) => {
              void form.handleSubmit((values) => mutation.mutate(values))(e);
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
                  <FormDescription>3 to 63 characters; lowercase, digits, dashes.</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="versioning_enabled"
              render={({ field }) => (
                <FormItem>
                  <div className="flex items-center gap-2">
                    <FormControl>
                      <input
                        id="create-bucket-versioning"
                        type="checkbox"
                        checked={field.value}
                        onChange={(e) => field.onChange(e.target.checked)}
                        className="h-4 w-4"
                      />
                    </FormControl>
                    <Label htmlFor="create-bucket-versioning">Enable versioning</Label>
                  </div>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="public_access"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Public access</FormLabel>
                  <Select
                    value={field.value}
                    onValueChange={(v) => field.onChange(v as PublicAccess)}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="private">Private</SelectItem>
                      <SelectItem value="public-read">Public read</SelectItem>
                      <SelectItem value="public-read-write">Public read &amp; write</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="quota_enabled"
              render={({ field }) => (
                <FormItem>
                  <div className="flex items-center gap-2">
                    <FormControl>
                      <input
                        id="create-bucket-quota-enabled"
                        type="checkbox"
                        checked={field.value}
                        onChange={(e) => field.onChange(e.target.checked)}
                        className="h-4 w-4"
                      />
                    </FormControl>
                    <Label htmlFor="create-bucket-quota-enabled">Set quota</Label>
                  </div>
                </FormItem>
              )}
            />

            {quotaEnabled && (
              <div className="grid grid-cols-3 gap-2">
                <FormField
                  control={form.control}
                  name="quota_kind"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Kind</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={(v) => field.onChange(v as QuotaKind)}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="hard">Hard</SelectItem>
                          <SelectItem value="fifo" disabled={versioningEnabled}>
                            FIFO
                          </SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="quota_value"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Value</FormLabel>
                      <FormControl>
                        <Input type="number" min="0" step="any" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="quota_unit"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Unit</FormLabel>
                      <Select value={field.value} onValueChange={(v) => field.onChange(v as Unit)}>
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="MiB">MiB</SelectItem>
                          <SelectItem value="GiB">GiB</SelectItem>
                          <SelectItem value="TiB">TiB</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            )}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? "Creating…" : "Create bucket"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
