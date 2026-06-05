import { describe, it, expect } from "vitest";
import { parseErrorResponse, AppError } from "./errors";

function makeResponse(body: unknown, status: number, statusText = "Error"): Response {
  return new Response(JSON.stringify(body), {
    status,
    statusText,
    headers: { "Content-Type": "application/json" },
  });
}

describe("parseErrorResponse", () => {
  describe("JSON:API errors[] envelope", () => {
    it("parses code, message from detail, and pointer", async () => {
      const res = makeResponse(
        {
          errors: [
            {
              status: "422",
              code: "validation_failed",
              title: "Validation Failed",
              detail: "name is required",
              source: { pointer: "/data/attributes/name" },
            },
          ],
        },
        422,
      );
      const err = await parseErrorResponse(res);
      expect(err).toBeInstanceOf(AppError);
      expect(err.code).toBe("validation_failed");
      expect(err.message).toBe("name is required");
      expect(err.pointer).toBe("/data/attributes/name");
      expect(err.status).toBe(422);
    });

    it("parses meta into details — policy_in_use with attached_to.users", async () => {
      const res = makeResponse(
        {
          errors: [
            {
              status: "409",
              code: "policy_in_use",
              title: "Policy In Use",
              meta: {
                attached_to: {
                  users: ["alice"],
                  groups: [],
                },
              },
            },
          ],
        },
        409,
      );
      const err = await parseErrorResponse(res);
      expect(err).toBeInstanceOf(AppError);
      expect(err.code).toBe("policy_in_use");
      expect(err.status).toBe(409);
      expect(err.details).toBeDefined();
      const attachedTo = (err.details as Record<string, unknown>).attached_to as Record<
        string,
        unknown
      >;
      expect(attachedTo).toBeDefined();
      expect(attachedTo.users).toEqual(["alice"]);
      expect(attachedTo.groups).toEqual([]);
    });

    it("does not set details when meta is absent", async () => {
      const res = makeResponse(
        {
          errors: [
            {
              status: "404",
              code: "not_found",
              title: "Not Found",
            },
          ],
        },
        404,
      );
      const err = await parseErrorResponse(res);
      expect(err.code).toBe("not_found");
      expect(err.details).toBeUndefined();
    });
  });

  describe("{error:{}} action envelope", () => {
    it("parses code, message, and details", async () => {
      const res = makeResponse(
        {
          error: {
            code: "conflict",
            message: "bucket already exists",
            details: { bucket: "photos" },
          },
        },
        409,
      );
      const err = await parseErrorResponse(res);
      expect(err).toBeInstanceOf(AppError);
      expect(err.code).toBe("conflict");
      expect(err.message).toBe("bucket already exists");
      expect(err.details).toEqual({ bucket: "photos" });
    });
  });

  describe("fallback", () => {
    it("returns unknown AppError on empty/invalid JSON", async () => {
      const res = new Response("not json", {
        status: 500,
        statusText: "Internal Server Error",
        headers: { "Content-Type": "text/plain" },
      });
      const err = await parseErrorResponse(res);
      expect(err.code).toBe("unknown");
      expect(err.message).toBe("Internal Server Error");
      expect(err.status).toBe(500);
    });
  });
});
