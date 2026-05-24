import { useEffect } from "react";
import { useForm } from "react-hook-form";
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
import { AppError } from "@/lib/api/errors";
import { lifecycleKeys } from "@/lib/api/keys";
import { createRule } from "./api";

const ruleSchema = z.object({
  days: z
    .number({ invalid_type_error: "Enter a whole number." })
    .int("Days must be a whole number.")
    .min(1, "Days must be at least 1.")
    .max(10_000, "Days must be at most 10000."),
  prefix: z.string().max(1024, "Prefix is too long."),
});

type FormValues = z.infer<typeof ruleSchema>;

export type CreateRuleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
};

export function CreateRuleDialog({ open, onOpenChange, bucket }: CreateRuleDialogProps) {
  const qc = useQueryClient();
  const form = useForm<FormValues>({
    resolver: zodResolver(ruleSchema),
    defaultValues: { days: 30, prefix: "" },
    mode: "onSubmit",
  });

  useEffect(() => {
    if (!open) form.reset({ days: 30, prefix: "" });
  }, [open, form]);

  const mutation = useMutation({
    mutationFn: (values: FormValues) => createRule(bucket, values.days, values.prefix || undefined),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: lifecycleKeys.list(bucket) });
      toast.success("Lifecycle rule created.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Failed to create rule.");
      else toast.error("Failed to create rule.");
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add lifecycle rule</DialogTitle>
          <DialogDescription>
            Objects matching the prefix are deleted after the configured number of days.
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
              name="days"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Days</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      min={1}
                      max={10000}
                      value={field.value}
                      onChange={(e) => field.onChange(Number(e.target.value))}
                    />
                  </FormControl>
                  <FormDescription>1 to 10000.</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
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
