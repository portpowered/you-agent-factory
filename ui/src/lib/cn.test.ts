import { describe, expect, it } from "vitest";

import { cn } from "./cn";

describe("cn", () => {
  it("joins only truthy class segments in order", () => {
    expect(cn("alpha", undefined, false, "", null, "beta", "gamma")).toBe(
      "alpha beta gamma",
    );
  });
});
