import * as workTypeSelectionPublic from "./index";

describe("work-type-selection/public", () => {
  it("keeps the public runtime surface focused on WorkTypeDetailCard", () => {
    expect(Object.keys(workTypeSelectionPublic).sort()).toEqual([
      "WorkTypeDetailCard",
    ]);
    expect(workTypeSelectionPublic.WorkTypeDetailCard).toBeTypeOf("function");
  });
});
