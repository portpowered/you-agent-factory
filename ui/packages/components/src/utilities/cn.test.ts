import { describe, expect, it } from "vitest";

import { cn } from "./cn";

describe("cn", () => {
  it("joins only truthy class segments in order", () => {
    expect(cn("alpha", undefined, false, "", null, "beta", "gamma")).toBe(
      "alpha beta gamma",
    );
  });

  it("returns an empty string when every segment is falsy", () => {
    expect(cn(undefined, false, null, "")).toBe("");
  });

  it("preserves conditional class segments without inserting separators for falsy values", () => {
    const isActive = true;
    const isDisabled = false;

    expect(
      cn(
        "base",
        isActive && "is-active",
        isDisabled && "is-disabled",
        "trailing",
      ),
    ).toBe("base is-active trailing");
  });

  it("keeps later duplicate utility classes so callers can override via argument order", () => {
    expect(cn("p-2", "p-4")).toBe("p-2 p-4");
  });
});
