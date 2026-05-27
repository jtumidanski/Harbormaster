import { useQuery } from "@tanstack/react-query";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AppError } from "@/lib/api/errors";
import { policyTemplatesKeys } from "@/lib/api/keys";
import { listPolicyTemplates } from "./api";
import type { PolicyTemplate } from "./types";

function ParamsSchemaPreview({ tpl }: { tpl: PolicyTemplate }) {
  if (!tpl.params_schema) {
    return <p className="text-xs text-muted-foreground">No parameters.</p>;
  }
  const required = tpl.params_schema.required ?? [];
  const properties = tpl.params_schema.properties ?? {};
  const keys = Object.keys(properties);
  if (keys.length === 0 && required.length === 0) {
    return <p className="text-xs text-muted-foreground">No parameters.</p>;
  }
  return (
    <div className="space-y-2">
      <div className="text-xs uppercase text-muted-foreground">Parameters</div>
      <ul className="space-y-1">
        {keys.map((k) => {
          const prop = properties[k];
          const isRequired = required.includes(k);
          const constraints: string[] = [prop.type];
          if (typeof prop.minLength === "number") constraints.push(`min ${prop.minLength}`);
          if (typeof prop.maxLength === "number") constraints.push(`max ${prop.maxLength}`);
          return (
            <li key={k} className="text-xs">
              <span className="font-mono">{k}</span>
              {isRequired && <span className="ml-1 text-destructive">*</span>}
              <span className="ml-2 text-muted-foreground">({constraints.join(", ")})</span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export function PolicyTemplatesPage() {
  const q = useQuery({
    queryKey: policyTemplatesKeys.list(),
    queryFn: listPolicyTemplates,
  });

  return (
    <div className="p-6 space-y-4">
      <div>
        <h1 className="text-2xl font-semibold">Policy templates</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Harbormaster bundles a small set of policy templates that compile to MinIO IAM policies
          when attached to users and service accounts. This page is read-only — templates are
          managed in code.
        </p>
      </div>

      <Alert>
        <AlertTitle>consoleAdmin is intentionally excluded</AlertTitle>
        <AlertDescription>
          MinIO&apos;s built-in <span className="font-mono">consoleAdmin</span> policy grants full
          administrative access and is intentionally excluded from Harbormaster. Manage admin users
          directly through MinIO if you need that scope.
        </AlertDescription>
      </Alert>

      {q.isLoading ? (
        <p className="text-muted-foreground">Loading…</p>
      ) : q.isError ? (
        <p className="text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load policy templates."}
        </p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-1 lg:grid-cols-2">
          {(q.data ?? []).map((tpl) => (
            <Card key={tpl.name}>
              <CardHeader>
                <CardTitle className="font-mono text-base">{tpl.name}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <p className="text-sm">{tpl.description}</p>
                <ParamsSchemaPreview tpl={tpl} />
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
