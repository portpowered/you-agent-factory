import { describe, expect, it } from "vitest";

import { COMPONENTS_PACKAGE_NAME } from "./index";

describe("youagentfactory/components package root", () => {
  it("exports the stable package name", () => {
    expect(COMPONENTS_PACKAGE_NAME).toBe("youagentfactory/components");
  });
});
