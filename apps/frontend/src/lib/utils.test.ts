import { describe, it, expect } from "vitest";
import { capitalize, cn } from "./utils";

describe("capitalize", () => {
  it("upper-cases the first character", () => {
    expect(capitalize("success")).toBe("Success");
    expect(capitalize("online")).toBe("Online");
  });

  it("leaves the remainder untouched", () => {
    expect(capitalize("bucket.create")).toBe("Bucket.create");
    expect(capitalize("ALL_CAPS")).toBe("ALL_CAPS");
  });

  it("returns an empty string unchanged", () => {
    expect(capitalize("")).toBe("");
  });
});

describe("cn", () => {
  it("merges conflicting tailwind classes, last wins", () => {
    expect(cn("p-2", "p-4")).toBe("p-4");
  });

  it("drops falsy values", () => {
    expect(cn("a", false, undefined, "b")).toBe("a b");
  });
});
