import { useState } from "react";
import { useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { authKeys } from "@/lib/api/keys";
import { AppError } from "@/lib/api/errors";
import { AdminStep } from "./AdminStep";
import { ObjectStoreStep, type ObjectStoreStepSubmit } from "./ObjectStoreStep";
import { submitSetup, type SetupPayload } from "./api";
import type { AdminInput } from "@/lib/schemas/setup";

type Step = "admin" | "objectStore";

function buildPayload(admin: AdminInput, objectStore: ObjectStoreStepSubmit): SetupPayload {
  const customCaPem =
    objectStore.values.customCaPem && objectStore.values.customCaPem.length > 0
      ? objectStore.values.customCaPem
      : null;
  const tlsSkipVerify = objectStore.values.tlsSkipVerify;

  if (objectStore.fromMcAlias) {
    return {
      admin: { username: admin.username, password: admin.password },
      minio: {
        from_mc_alias: objectStore.fromMcAlias,
        tls_skip_verify: tlsSkipVerify,
        custom_ca_pem: customCaPem,
      },
    };
  }
  return {
    admin: { username: admin.username, password: admin.password },
    minio: {
      endpoint_url: objectStore.values.endpointUrl,
      access_key: objectStore.values.accessKey,
      secret_key: objectStore.values.secretKey,
      tls_skip_verify: tlsSkipVerify,
      custom_ca_pem: customCaPem,
    },
  };
}

export function SetupWizard() {
  const [step, setStep] = useState<Step>("admin");
  const [admin, setAdmin] = useState<AdminInput | null>(null);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: (payload: SetupPayload) => submitSetup(payload),
    onSuccess: async () => {
      toast.success("Setup complete. Please sign in.");
      await queryClient.invalidateQueries({ queryKey: authKeys.setupStatus() });
      await navigate("/login");
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        toast.error(err.message || err.code, { description: `code: ${err.code}` });
      } else {
        toast.error("Setup failed. Please try again.");
      }
    },
  });

  function handleAdminSubmit(values: AdminInput) {
    setAdmin(values);
    setStep("objectStore");
  }

  function handleObjectStoreSubmit(result: ObjectStoreStepSubmit) {
    if (!admin) {
      setStep("admin");
      return;
    }
    mutation.mutate(buildPayload(admin, result));
  }

  return (
    <div className="mx-auto flex min-h-screen max-w-xl items-center p-6">
      <Card className="w-full">
        <CardHeader>
          <CardTitle>Setup wizard</CardTitle>
          <CardDescription>
            {step === "admin"
              ? "Step 1 of 2 — Create the Harbormaster admin account."
              : "Step 2 of 2 — Connect Harbormaster to your object store."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {step === "admin" ? (
            <AdminStep {...(admin ? { defaultValues: admin } : {})} onSubmit={handleAdminSubmit} />
          ) : (
            <ObjectStoreStep
              onBack={() => setStep("admin")}
              onSubmit={handleObjectStoreSubmit}
              submitting={mutation.isPending}
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
