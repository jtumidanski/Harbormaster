import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { EditCustomPoliciesDialog } from "./EditCustomPoliciesDialog";
import type { User } from "./types";

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

function installFetch(stubs: StubResponse[]) {
  const spy = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    for (const s of stubs) {
      if (s.match(url, init)) return Promise.resolve(s.response());
    }
    return Promise.resolve(
      new Response(JSON.stringify({ errors: [{ code: "not_found" }] }), {
        status: 404,
        headers: { "Content-Type": "application/vnd.api+json" },
      }),
    );
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function jsonapi(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function policiesStub(): StubResponse {
  return {
    match: (u, init) => u.includes("/api/v1/policies") && (init?.method ?? "GET") === "GET",
    response: () =>
      jsonapi({
        data: [
          {
            type: "policies",
            id: "my-custom-policy",
            attributes: {
              name: "my-custom-policy",
              origin: "custom",
              editable: true,
              statement_summary: "Custom policy A",
            },
          },
          {
            type: "policies",
            id: "another-custom",
            attributes: {
              name: "another-custom",
              origin: "custom",
              editable: true,
              statement_summary: "Custom policy B",
            },
          },
          {
            type: "policies",
            id: "readonly",
            attributes: {
              name: "readonly",
              origin: "minio-builtin",
              editable: false,
              statement_summary: "Built-in readonly",
            },
          },
        ],
      }),
  };
}

function updatePoliciesStub(): StubResponse {
  return {
    match: (u, init) =>
      u.includes("/api/v1/users/") && u.includes("/policies") && (init?.method ?? "GET") === "PUT",
    response: () => new Response(null, { status: 204 }),
  };
}

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        {children}
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

const baseUser: User = {
  access_key: "alice",
  status: "enabled",
  attached_templates: [{ name: "readonly", params: null }],
  attached_policies: ["my-custom-policy"],
  other_policies: [],
};

describe("EditCustomPoliciesDialog", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders only custom-origin policies as checkboxes", async () => {
    installFetch([policiesStub(), updatePoliciesStub()]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <EditCustomPoliciesDialog open={true} onOpenChange={() => {}} user={baseUser} />
      </Wrapper>,
    );

    // Custom policies appear
    await waitFor(() => {
      expect(screen.getByLabelText("my-custom-policy")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("another-custom")).toBeInTheDocument();

    // Built-in policy does NOT appear
    expect(screen.queryByLabelText("readonly")).not.toBeInTheDocument();
  });

  it("pre-checks the user's current attached_policies", async () => {
    installFetch([policiesStub(), updatePoliciesStub()]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <EditCustomPoliciesDialog open={true} onOpenChange={() => {}} user={baseUser} />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByLabelText("my-custom-policy")).toBeInTheDocument();
    });

    const checkedBox = screen.getByLabelText<HTMLInputElement>("my-custom-policy");
    const uncheckedBox = screen.getByLabelText<HTMLInputElement>("another-custom");
    expect(checkedBox.checked).toBe(true);
    expect(uncheckedBox.checked).toBe(false);
  });

  it("calls updateUserPolicies with current templates refs and selected custom policy names on submit", async () => {
    const fetchSpy = installFetch([policiesStub(), updatePoliciesStub()]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <EditCustomPoliciesDialog open={true} onOpenChange={() => {}} user={baseUser} />
      </Wrapper>,
    );

    // Wait for policies to load
    await waitFor(() => {
      expect(screen.getByLabelText("my-custom-policy")).toBeInTheDocument();
    });

    // Also check "another-custom"
    await user.click(screen.getByLabelText("another-custom"));

    await user.click(screen.getByRole("button", { name: /save policies/i }));

    type PoliciesBody = {
      templates: { name: string; params: Record<string, string> | null }[];
      policies: string[];
    };

    await waitFor(() => {
      const putCall = fetchSpy.mock.calls.find(([input, init]) => {
        const url =
          typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
        return url.includes("/api/v1/users/alice/policies") && init?.method === "PUT";
      });
      expect(putCall).toBeDefined();
      const body = JSON.parse(String(putCall![1]!.body)) as PoliciesBody;
      // Templates should be preserved from user.attached_templates
      expect(body.templates).toEqual([{ name: "readonly", params: null }]);
      // Both custom policies selected
      expect(body.policies).toContain("my-custom-policy");
      expect(body.policies).toContain("another-custom");
    });
  });
});
