import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { useAuth } from "@/context/AuthContext";
import { AppError } from "@/lib/api/errors";
import { loginSchema, type LoginInput } from "@/lib/schemas/auth";
import { login } from "./api";

export function LoginPage() {
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const form = useForm<LoginInput>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: "", password: "" },
    mode: "onSubmit",
  });

  const mutation = useMutation({
    mutationFn: (values: LoginInput) =>
      login({ username: values.username, password: values.password }),
    onSuccess: async () => {
      await refresh();
      navigate("/buckets");
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.status === 401 || err.code === "invalid_credentials") {
          toast.error("Invalid username or password.");
          return;
        }
        if (err.status === 429 || err.code === "too_many_attempts") {
          toast.error(err.message || "Too many attempts. Please try again later.");
          return;
        }
        toast.error(err.message || "Login failed. Please try again.");
        return;
      }
      toast.error("Login failed. Please try again.");
    },
  });

  return (
    <div className="mx-auto flex min-h-screen max-w-md items-center p-6">
      <Card className="w-full">
        <CardHeader>
          <CardTitle>Sign in</CardTitle>
          <CardDescription>Sign in to Harbormaster.</CardDescription>
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
                name="username"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Username</FormLabel>
                    <FormControl>
                      <Input
                        autoComplete="username"
                        autoCapitalize="none"
                        spellCheck={false}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Password</FormLabel>
                    <FormControl>
                      <Input type="password" autoComplete="current-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <div className="flex justify-end">
                <Button type="submit" disabled={mutation.isPending}>
                  {mutation.isPending ? "Signing in…" : "Sign in"}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
