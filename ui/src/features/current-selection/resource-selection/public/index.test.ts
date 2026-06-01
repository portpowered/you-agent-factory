import { describe, expect, it } from "vitest";

import * as resourceSelectionPublic from "./index";

describe("resource-selection/public", () => {
  it("keeps the public runtime surface focused on ResourceDetailCard", () => {
    expect(Object.keys(resourceSelectionPublic).sort()).toEqual([
      "EditableResourceConfigurationHeaderActions",
      "EditableResourceSaveHeaderAction",
      "ResourceDetailCard",
    ]);
    expect(resourceSelectionPublic.ResourceDetailCard).toBeTypeOf("function");
  });
});
