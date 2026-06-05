import { useEffect } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
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
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AppError } from "@/lib/api/errors";
import { lifecycleKeys } from "@/lib/api/keys";
import { createRule, type CreateRuleAttrs } from "./api";

// ---------------------------------------------------------------------------
// Zod discriminated-union schema — one branch per backend lifecycle kind.
// z.coerce.number() converts string values from <input type="number"> before
// Zod validation runs.
// ---------------------------------------------------------------------------
const ruleSchema = z.discriminatedUnion("kind", [
  z.object({
    kind: z.literal("expiration"),
    days: z.coerce
      .number({ invalid_type_error: "Enter a whole number." })
      .int("Days must be a whole number.")
      .min(1, "Days must be at least 1.")
      .max(10_000, "Days must be at most 10000."),
    prefix: z.string().max(1024, "Prefix is too long."),
  }),
  z.object({
    kind: z.literal("noncurrent-expiration"),
    noncurrent_days: z.coerce
      .number({ invalid_type_error: "Enter a whole number." })
      .int("Must be a whole number.")
      .min(1, "Must be at least 1.")
      .max(10_000, "Must be at most 10000."),
    newer_noncurrent_versions: z.coerce
      .number({ invalid_type_error: "Enter a whole number." })
      .int("Must be a whole number.")
      .min(0, "Must be at least 0.")
      .max(1000, "Must be at most 1000."),
    prefix: z.string().max(1024, "Prefix is too long."),
  }),
  z.object({
    kind: z.literal("abort-incomplete-multipart"),
    days_after_initiation: z.coerce
      .number({ invalid_type_error: "Enter a whole number." })
      .int("Must be a whole number.")
      .min(1, "Must be at least 1.")
      .max(10_000, "Must be at most 10000."),
    prefix: z.string().max(1024, "Prefix is too long."),
  }),
]);

type FormValues = z.infer<typeof ruleSchema>;

// Union of all valid field paths across every branch, used to type setError.
type AnyFieldPath =
  | "kind"
  | "days"
  | "prefix"
  | "noncurrent_days"
  | "newer_noncurrent_versions"
  | "days_after_initiation";

// Map the JSON:API pointer from a backend error to the matching form field.
function pointerToField(
  pointer: string | undefined,
  kind: FormValues["kind"],
): AnyFieldPath | null {
  if (!pointer) return null;
  const attr = pointer.replace(/^\/data\/attributes\//, "");
  const validFields: Record<FormValues["kind"], Set<string>> = {
    expiration: new Set(["days", "prefix"]),
    "noncurrent-expiration": new Set(["noncurrent_days", "newer_noncurrent_versions", "prefix"]),
    "abort-incomplete-multipart": new Set(["days_after_initiation", "prefix"]),
  };
  if (validFields[kind].has(attr)) return attr as AnyFieldPath;
  return null;
}

// Default values cover all branches so the wide spread satisfies the
// discriminated-union's shape at reset time.
const DEFAULT_VALUES = {
  kind: "expiration" as const,
  days: 30,
  noncurrent_days: 30,
  newer_noncurrent_versions: 0,
  days_after_initiation: 7,
  prefix: "",
};

export type CreateRuleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  /** When explicitly false, a non-blocking warning is shown for noncurrent-expiration. */
  versioningEnabled?: boolean;
};

export function CreateRuleDialog({
  open,
  onOpenChange,
  bucket,
  versioningEnabled,
}: CreateRuleDialogProps) {
  const qc = useQueryClient();
  const form = useForm({
    resolver: zodResolver(ruleSchema),
    defaultValues: DEFAULT_VALUES,
    mode: "onSubmit",
  });

  const kind = useWatch({ control: form.control, name: "kind" }) as FormValues["kind"];

  useEffect(() => {
    if (!open) form.reset(DEFAULT_VALUES);
  }, [open, form]);

  const mutation = useMutation({
    mutationFn: (values: FormValues) => createRule(bucket, values as CreateRuleAttrs),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: lifecycleKeys.list(bucket) });
      toast.success("Lifecycle rule created.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.status === 422) {
          const currentKind = form.getValues("kind") as FormValues["kind"];
          const field = pointerToField(err.pointer, currentKind);
          if (field) {
            form.setError(field, { type: "server", message: err.message });
            return;
          }
        }
        toast.error(err.message || "Failed to create rule.");
        return;
      }
      toast.error("Failed to create rule.");
    },
  });

  const showVersioningWarning = kind === "noncurrent-expiration" && versioningEnabled === false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add lifecycle rule</DialogTitle>
          <DialogDescription>Configure an object lifecycle rule for this bucket.</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={(e) => {
              void form.handleSubmit((values) => mutation.mutate(values as FormValues))(e);
            }}
            className="space-y-4"
            noValidate
          >
            {/* Kind selector */}
            <FormField
              control={form.control}
              name="kind"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Rule kind</FormLabel>
                  <Select
                    value={field.value as string}
                    onValueChange={(v) => {
                      field.onChange(v);
                      form.clearErrors();
                    }}
                  >
                    <FormControl>
                      <SelectTrigger aria-label="Rule kind">
                        <SelectValue placeholder="Select rule kind" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="expiration">Expiration</SelectItem>
                      <SelectItem value="noncurrent-expiration">Noncurrent versions</SelectItem>
                      <SelectItem value="abort-incomplete-multipart">
                        Abort incomplete multipart
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Non-blocking versioning warning for noncurrent-expiration */}
            {showVersioningWarning && (
              <div
                role="alert"
                className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-200"
              >
                Versioning is disabled on this bucket. Noncurrent version rules only apply when
                versioning is enabled.
              </div>
            )}

            {/* Expiration fields */}
            {kind === "expiration" && (
              <FormField
                control={form.control}
                name="days"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Days</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={1}
                        max={10000}
                        {...field}
                        value={field.value as string | number}
                        onChange={(e) => field.onChange(e.target.value)}
                      />
                    </FormControl>
                    <FormDescription>1 to 10000.</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {/* Noncurrent-expiration fields */}
            {kind === "noncurrent-expiration" && (
              <>
                <FormField
                  control={form.control}
                  name="noncurrent_days"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Noncurrent days</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={1}
                          max={10000}
                          {...field}
                          value={field.value as string | number}
                          onChange={(e) => field.onChange(e.target.value)}
                        />
                      </FormControl>
                      <FormDescription>
                        Expire noncurrent versions after this many days (1–10000).
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="newer_noncurrent_versions"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Newer noncurrent versions</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={0}
                          max={1000}
                          {...field}
                          value={field.value as string | number}
                          onChange={(e) => field.onChange(e.target.value)}
                        />
                      </FormControl>
                      <FormDescription>
                        Keep at least this many noncurrent versions (0–1000; 0 = unlimited).
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </>
            )}

            {/* Abort-incomplete-multipart fields */}
            {kind === "abort-incomplete-multipart" && (
              <FormField
                control={form.control}
                name="days_after_initiation"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Days after initiation</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={1}
                        max={10000}
                        {...field}
                        value={field.value as string | number}
                        onChange={(e) => field.onChange(e.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      Abort incomplete uploads after this many days (1–10000).
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {/* Prefix — common to all kinds */}
            <FormField
              control={form.control}
              name="prefix"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Prefix (optional)</FormLabel>
                  <FormControl>
                    <Input placeholder="logs/" autoComplete="off" {...field} />
                  </FormControl>
                  <FormDescription>Leave blank to apply to the entire bucket.</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? "Creating…" : "Add rule"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
