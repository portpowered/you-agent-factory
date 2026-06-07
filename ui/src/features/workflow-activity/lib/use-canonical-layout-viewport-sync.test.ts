import { describe, expect, it } from "vitest";

import { canonicalLayoutViewportKey } from "./use-canonical-layout-viewport-sync";

describe("canonicalLayoutViewportKey", () => {
  it("returns a stable key for saved viewport values", () => {
    expect(
      canonicalLayoutViewportKey({ x: 120, y: 80, zoom: 1.25 }),
    ).toBe("120:80:1.25");
  });

  it("returns none when viewport metadata is absent", () => {
    expect(canonicalLayoutViewportKey(undefined)).toBe("none");
    expect(canonicalLayoutViewportKey(null)).toBe("none");
  });
});
