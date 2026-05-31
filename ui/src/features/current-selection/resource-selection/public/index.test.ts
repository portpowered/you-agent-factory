import * as resourceSelectionPublic from "./index";

describe("resource-selection public exports", () => {
  it("keeps the public runtime surface focused on ResourceDetailCard", () => {
    expect(Object.keys(resourceSelectionPublic).sort()).toEqual([
      "EditableResourceSaveHeaderAction",
      "ResourceDetailCard",
    ]);
    expect(resourceSelectionPublic.ResourceDetailCard).toBeTypeOf("function");
  });
});
