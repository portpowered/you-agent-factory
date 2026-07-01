import { describe, expect, it } from "vitest";

import { dashboardComponentsPackageName } from "./components-package-resolution";

describe("dashboard youagentfactory/components package resolution", () => {
  it("imports the package root through the configured package path", () => {
    expect(dashboardComponentsPackageName).toBe("youagentfactory/components");
  });
});
