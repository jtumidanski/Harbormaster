import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { minioSchema, type MinIOInput } from "@/lib/schemas/setup";
import { authKeys } from "@/lib/api/keys";
import { fetchMcAliases, type McAlias } from "./api";

export type MinIOStepSubmit = {
  values: MinIOInput;
  /** Name of mc alias to use server-side; only set when alias was selected and not edited away from. */
  fromMcAlias: string | null;
};

export type MinIOStepProps = {
  onBack: () => void;
  onSubmit: (result: MinIOStepSubmit) => void;
  submitting?: boolean;
};

const EMPTY_DEFAULTS: MinIOInput = {
  endpointUrl: "",
  accessKey: "",
  secretKey: "",
  tlsSkipVerify: false,
};

export function MinIOStep({ onBack, onSubmit, submitting }: MinIOStepProps) {
  const aliasesQuery = useQuery({
    queryKey: authKeys.mcAliases(),
    queryFn: fetchMcAliases,
    retry: false,
  });

  const form = useForm<MinIOInput>({
    resolver: zodResolver(minioSchema),
    defaultValues: EMPTY_DEFAULTS,
    mode: "onSubmit",
  });

  // Track the alias the operator picked and the values we prefilled from it so
  // we can detect when they edit any of those fields and fall back to explicit
  // credentials at submit time.
  const [selectedAlias, setSelectedAlias] = useState<McAlias | null>(null);
  const prefillRef = useRef<{
    endpointUrl: string;
    accessKey: string;
    tlsSkipVerify: boolean;
  } | null>(null);

  const aliases: McAlias[] = aliasesQuery.data?.aliases ?? [];
  const showAliasSelect = !aliasesQuery.isError && aliases.length > 0;

  function applyAlias(alias: McAlias) {
    setSelectedAlias(alias);
    prefillRef.current = {
      endpointUrl: alias.endpoint,
      accessKey: alias.access_key,
      tlsSkipVerify: alias.tls_skip_verify,
    };
    form.setValue("endpointUrl", alias.endpoint, { shouldDirty: true, shouldValidate: false });
    form.setValue("accessKey", alias.access_key, { shouldDirty: true, shouldValidate: false });
    form.setValue("tlsSkipVerify", alias.tls_skip_verify, {
      shouldDirty: true,
      shouldValidate: false,
    });
    // Clear secret so the operator must (re)enter it, since aliases don't carry secrets.
    form.setValue("secretKey", "", { shouldDirty: false, shouldValidate: false });
  }

  // Watch the alias-critical fields; clear `selectedAlias` once they diverge from the prefill.
  const endpointUrlValue = form.watch("endpointUrl");
  const accessKeyValue = form.watch("accessKey");
  const tlsSkipVerifyValue = form.watch("tlsSkipVerify");
  useEffect(() => {
    if (!selectedAlias || !prefillRef.current) return;
    const p = prefillRef.current;
    if (
      endpointUrlValue !== p.endpointUrl ||
      accessKeyValue !== p.accessKey ||
      tlsSkipVerifyValue !== p.tlsSkipVerify
    ) {
      setSelectedAlias(null);
      prefillRef.current = null;
    }
  }, [endpointUrlValue, accessKeyValue, tlsSkipVerifyValue, selectedAlias]);

  function handleAliasChange(name: string) {
    const alias = aliases.find((a) => a.name === name);
    if (alias) applyAlias(alias);
  }

  function handleSubmit(values: MinIOInput) {
    onSubmit({
      values,
      fromMcAlias: selectedAlias ? selectedAlias.name : null,
    });
  }

  return (
    <div className="space-y-4">
      {showAliasSelect ? (
        <div className="space-y-2">
          <Label htmlFor="mc-alias">Import from mc alias</Label>
          <Select onValueChange={handleAliasChange} value={selectedAlias?.name ?? ""}>
            <SelectTrigger id="mc-alias" aria-label="Import from mc alias">
              <SelectValue placeholder="Select an mc alias…" />
            </SelectTrigger>
            <SelectContent>
              {aliases.map((a) => (
                <SelectItem key={a.name} value={a.name}>
                  {a.name} — {a.endpoint}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-sm text-muted-foreground">
            Aliases pre-fill the endpoint and access key. You must still enter the secret key.
          </p>
        </div>
      ) : null}

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

          <div className="flex justify-between">
            <Button type="button" variant="outline" onClick={onBack} disabled={submitting}>
              Back
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? "Finishing setup…" : "Finish setup"}
            </Button>
          </div>
        </form>
      </Form>
    </div>
  );
}
