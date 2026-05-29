import { describe, expect, it } from "vitest";

import * as currentSelectionBasePublic from "./index";

describe("current-selection base public barrel", () => {
  it("exports shared selection contracts and layout primitives", () => {
    expect(currentSelectionBasePublic.resolveDashboardSelection).toBeTypeOf("function");
    expect(currentSelectionBasePublic.selectDefaultSelection).toBeTypeOf("function");
    expect(currentSelectionBasePublic.resetSelectionHistoryStore).toBeTypeOf("function");
    expect(currentSelectionBasePublic.SelectionDetailLayout).toBeTypeOf("function");
    expect(currentSelectionBasePublic.NoSelectionDetailCard).toBeTypeOf("function");
    expect(currentSelectionBasePublic.PROVIDER_SESSION_CARD_CLASS).toBeTypeOf("string");
  });
});
