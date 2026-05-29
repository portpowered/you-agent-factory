import { describe, expect, it } from "vitest";

import * as dispatchSelectionPublic from "./index";

describe("dispatch-selection/public", () => {
  it("exports dispatch detail cards, history section, and helper accessors", () => {
    expect(dispatchSelectionPublic.WorkstationRequestDetailCard).toBeTypeOf(
      "function",
    );
    expect(dispatchSelectionPublic.SelectedWorkDispatchHistorySection).toBeTypeOf(
      "function",
    );
    expect(dispatchSelectionPublic.requestInferenceAttempts).toBeTypeOf(
      "function",
    );
    expect(dispatchSelectionPublic.dedupeWorkItems).toBeTypeOf("function");
  });
});
