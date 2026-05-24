import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
import { changePasswordSchema, type ChangePasswordInput } from "@/lib/schemas/auth";
import { changePassword } from "./api";

function describeWeakPassword(details: Record<string, unknown> | undefined): string {
  if (!details) return "Password is too weak.";
  const reasons: string[] = [];
  for (const v of Object.values(details)) {
    if (typeof v === "string") reasons.push(v);
  }
  if (reasons.length === 0) return "Password is too weak.";
  return reasons.join(" ");
}

export function ChangePasswordPage() {
  const form = useForm<ChangePasswordInput>({
    resolver: zodResolver(changePasswordSchema),
    defaultValues: {
      currentPassword: "",
      newPassword: "",
      newPasswordConfirm: "",
    },
    mode: "onSubmit",
  });

  const mutation = useMutation({
    mutationFn: (values: ChangePasswordInput) =>
      changePassword({
        current_password: values.currentPassword,
        new_password: values.newPassword,
      }),
    onSuccess: () => {
      toast.success("Password updated.");
      form.reset();
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.status === 401) {
          toast.error("Current password incorrect.");
          return;
        }
        if (err.status === 422 && err.code === "weak_password") {
          toast.error(describeWeakPassword(err.details));
          return;
        }
        toast.error(err.message || "Failed to change password.");
        return;
      }
      toast.error("Failed to change password.");
    },
  });

  return (
    <div className="mx-auto max-w-xl p-6">
      <Card className="w-full">
        <CardHeader>
          <CardTitle>Account password</CardTitle>
          <CardDescription>Change the password for your Harbormaster account.</CardDescription>
        </CardHeader>
        <CardContent>
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
                name="currentPassword"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Current password</FormLabel>
                    <FormControl>
                      <Input type="password" autoComplete="current-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="newPassword"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>New password</FormLabel>
                    <FormControl>
                      <Input type="password" autoComplete="new-password" {...field} />
                    </FormControl>
                    <FormDescription>At least 12 characters.</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="newPasswordConfirm"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Confirm new password</FormLabel>
                    <FormControl>
                      <Input type="password" autoComplete="new-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <div className="flex justify-end">
                <Button type="submit" disabled={mutation.isPending}>
                  {mutation.isPending ? "Updating…" : "Update password"}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
