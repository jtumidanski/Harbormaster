import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
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
import { Input } from "@/components/ui/input";
import { AppError } from "@/lib/api/errors";
import { connectionKeys } from "@/lib/api/keys";
import { connectionSchema, type ConnectionInput } from "@/lib/schemas/connection";
import {
  fetchConnection,
  testConnection,
  updateConnection,
  type ConnectionCheck,
  type ConnectionDetail,
  type ConnectionPayload,
  type ConnectionTestResult,
} from "./api";

function toPayload(values: ConnectionInput): ConnectionPayload {
  const customCaPem =
    values.customCaPem && values.customCaPem.length > 0 ? values.customCaPem : null;
  return {
    endpoint_url: values.endpointUrl,
    access_key: values.accessKey,
    secret_key: values.secretKey,
    tls_skip_verify: values.tlsSkipVerify,
    custom_ca_pem: customCaPem,
  };
}

function checkLabel(c: ConnectionCheck): string {
  if (c === "ok") return "ok";
  if (c === null) return "not run";
  return `failed: ${c.failed}`;
}

function CheckRow({ name, value }: { name: string; value: ConnectionCheck }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="font-medium">{name}</span>
      <span data-testid={`check-${name}`}>{checkLabel(value)}</span>
    </div>
  );
}

function DetailView({ detail, onEdit }: { detail: ConnectionDetail; onEdit: () => void }) {
  return (
    <dl className="space-y-3 text-sm">
      <div>
        <dt className="text-muted-foreground">Endpoint URL</dt>
        <dd>{detail.endpoint_url}</dd>
      </div>
      <div>
        <dt className="text-muted-foreground">Access key</dt>
        <dd>{detail.access_key_masked}</dd>
      </div>
      <div>
        <dt className="text-muted-foreground">Secret key</dt>
        <dd>{detail.secret_key_present ? "present" : "not set"}</dd>
      </div>
      <div>
        <dt className="text-muted-foreground">TLS skip verify</dt>
        <dd>{detail.tls_skip_verify ? "yes" : "no"}</dd>
      </div>
      <div>
        <dt className="text-muted-foreground">Custom CA PEM</dt>
        <dd>{detail.custom_ca_pem_present ? "present" : "not set"}</dd>
      </div>
      <div className="flex justify-end pt-2">
        <Button type="button" onClick={onEdit}>
          Edit
        </Button>
      </div>
    </dl>
  );
}

type EditFormProps = {
  detail: ConnectionDetail;
  onCancel: () => void;
  onSaved: () => void;
};

function EditForm({ detail, onCancel, onSaved }: EditFormProps) {
  const queryClient = useQueryClient();
  const form = useForm<ConnectionInput>({
    resolver: zodResolver(connectionSchema),
    defaultValues: {
      endpointUrl: detail.endpoint_url,
      accessKey: "",
      secretKey: "",
      tlsSkipVerify: detail.tls_skip_verify,
      customCaPem: "",
    },
    mode: "onSubmit",
  });

  const [testResult, setTestResult] = useState<ConnectionTestResult | null>(null);

  const updateMutation = useMutation({
    mutationFn: (values: ConnectionInput) => updateConnection(toPayload(values)),
    onSuccess: async () => {
      toast.success("Connection updated.");
      await queryClient.invalidateQueries({ queryKey: connectionKeys.detail() });
      onSaved();
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        toast.error(err.message || err.code, { description: `code: ${err.code}` });
        return;
      }
      toast.error("Failed to update connection.");
    },
  });

  const testMutation = useMutation({
    mutationFn: (values: ConnectionInput) => testConnection(toPayload(values)),
    onSuccess: (result) => {
      setTestResult(result);
    },
    onError: (err: unknown) => {
      setTestResult(null);
      if (err instanceof AppError) {
        toast.error(err.message || err.code, { description: `code: ${err.code}` });
        return;
      }
      toast.error("Test failed.");
    },
  });

  async function handleTest() {
    const valid = await form.trigger();
    if (!valid) return;
    testMutation.mutate(form.getValues());
  }

  return (
    <Form {...form}>
      <form
        onSubmit={(e) => {
          void form.handleSubmit((values) => updateMutation.mutate(values))(e);
        }}
        className="space-y-4"
        noValidate
      >
        <FormField
          control={form.control}
          name="endpointUrl"
          render={({ field }) => (
            <FormItem>
              <FormLabel>MinIO endpoint URL</FormLabel>
              <FormControl>
                <Input
                  type="url"
                  placeholder="https://minio.lan:9000"
                  autoComplete="off"
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
          name="accessKey"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Access key</FormLabel>
              <FormControl>
                <Input autoComplete="off" spellCheck={false} {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="secretKey"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Secret key</FormLabel>
              <FormControl>
                <Input type="password" autoComplete="off" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="tlsSkipVerify"
          render={({ field }) => (
            <FormItem className="flex flex-row items-start space-x-3 space-y-0">
              <FormControl>
                <input
                  type="checkbox"
                  className="mt-1 h-4 w-4"
                  checked={field.value}
                  onChange={(e) => field.onChange(e.target.checked)}
                  onBlur={field.onBlur}
                  name={field.name}
                  ref={field.ref}
                />
              </FormControl>
              <div className="space-y-1 leading-none">
                <FormLabel>Skip TLS verification</FormLabel>
                <FormDescription>
                  Only enable for self-signed certs in trusted networks.
                </FormDescription>
              </div>
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="customCaPem"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Custom CA (PEM, optional)</FormLabel>
              <FormControl>
                <textarea
                  className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                  placeholder="-----BEGIN CERTIFICATE-----"
                  value={field.value ?? ""}
                  onChange={field.onChange}
                  onBlur={field.onBlur}
                  name={field.name}
                  ref={field.ref}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {testResult ? (
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Test results</CardTitle>
              <CardDescription>
                {testResult.minio_version
                  ? `MinIO version: ${testResult.minio_version}`
                  : "MinIO version: unknown"}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              <CheckRow name="tcp_connect" value={testResult.tcp_connect} />
              <CheckRow name="list_buckets" value={testResult.list_buckets} />
              <CheckRow name="admin_ping" value={testResult.admin_ping} />
            </CardContent>
          </Card>
        ) : null}

        <div className="flex justify-between">
          <Button type="button" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                void handleTest();
              }}
              disabled={testMutation.isPending}
            >
              {testMutation.isPending ? "Testing…" : "Test connection"}
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? "Saving…" : "Save"}
            </Button>
          </div>
        </div>
      </form>
    </Form>
  );
}

export function ConnectionSettingsPage() {
  const [editing, setEditing] = useState(false);
  const query = useQuery({
    queryKey: connectionKeys.detail(),
    queryFn: fetchConnection,
  });

  // If the underlying detail data disappears (e.g., error), make sure we don't get stuck in edit mode.
  useEffect(() => {
    if (!query.data && editing && query.isError) {
      setEditing(false);
    }
  }, [query.data, query.isError, editing]);

  return (
    <div className="mx-auto max-w-xl p-6">
      <Card className="w-full">
        <CardHeader>
          <CardTitle>MinIO connection</CardTitle>
          <CardDescription>
            View and update Harbormaster&apos;s connection to your MinIO cluster.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {query.isLoading ? (
            <div className="text-sm text-muted-foreground">Loading…</div>
          ) : query.isError ? (
            <div className="text-sm text-destructive">Failed to load connection.</div>
          ) : query.data ? (
            editing ? (
              <EditForm
                detail={query.data}
                onCancel={() => setEditing(false)}
                onSaved={() => setEditing(false)}
              />
            ) : (
              <DetailView detail={query.data} onEdit={() => setEditing(true)} />
            )
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
