import { describe, expect, it } from "vitest";

/**
 * Tailwind v4 removed the v3 shorthand for reading a CSS variable inside an
 * arbitrary value. Naming a custom property directly between square brackets
 * used to compile to `width: var(--sidebar-width)`; it now compiles to
 * `width: --sidebar-width`, which browsers drop silently. The v4 spelling puts
 * the custom property in parentheses instead: `w-(--sidebar-width)`. An
 * explicit `var()` inside brackets also still works.
 *
 * Silent drops are hard to spot in review, so guard the whole source tree.
 */
const V3_ARBITRARY_VAR = /-\[--[a-zA-Z_]/;

const sources = import.meta.glob("../../**/*.{ts,tsx,css}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

describe("tailwind arbitrary CSS variable syntax", () => {
  // A glob that quietly stops matching would turn the guard below into a
  // no-op that still passes, so assert it reaches both file kinds.
  it("scans the source tree", () => {
    const paths = Object.keys(sources);

    expect(paths.some((path) => path.endsWith("sidebar.tsx"))).toBe(true);
    expect(paths.some((path) => path.endsWith("styles/index.css"))).toBe(true);
  });

  it("uses the v4 parenthesis form everywhere", () => {
    const offenders = Object.entries(sources).flatMap(([path, contents]) =>
      contents
        .split("\n")
        .flatMap((line, index) => (V3_ARBITRARY_VAR.test(line) ? [`${path}:${index + 1}`] : [])),
    );

    expect(offenders).toEqual([]);
  });
});
