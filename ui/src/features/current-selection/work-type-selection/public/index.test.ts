import * as workTypeSelectionPublic from "./index";

describe("work-type-selection/public", () => {
  it("exports the work type detail card and save controls", () => {
    expect(Object.keys(workTypeSelectionPublic).sort()).toEqual([
      "EditableWorkTypeSaveDialog",
      "EditableWorkTypeSaveHeaderAction",
      "WorkTypeDetailCard",
      "getWorkTypeDetailMessages",
    ]);
    expect(workTypeSelectionPublic.WorkTypeDetailCard).toBeTypeOf("function");
    expect(
      workTypeSelectionPublic.EditableWorkTypeSaveHeaderAction,
    ).toBeTypeOf("function");
  });
});
