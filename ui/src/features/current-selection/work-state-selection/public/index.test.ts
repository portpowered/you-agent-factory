import { describe, expect, it } from "vitest";

import * as workStateSelectionPublic from "./index";

describe("work-state-selection/public", () => {
  it("exports state node detail card and prop types", () => {
    expect(workStateSelectionPublic.StateNodeDetailCard).toBeTypeOf("function");
  });
});
