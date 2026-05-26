import * as currentSelectionPublic from "./index";

describe("current-selection public barrel", () => {
  it("keeps the supported selection APIs public", () => {
    expect(currentSelectionPublic).toHaveProperty("CurrentSelectionWidget");
    expect(currentSelectionPublic).toHaveProperty("WorkstationDetailCard");
    expect(currentSelectionPublic).toHaveProperty("useCurrentSelection");
    expect(currentSelectionPublic).toHaveProperty("useCurrentSelectionDetails");
    expect(currentSelectionPublic).toHaveProperty(
      "useSelectedProviderSessionState",
    );
  });

  it("does not re-export workstation-detail message helpers", () => {
    expect(currentSelectionPublic).not.toHaveProperty(
      "getWorkstationDetailMessages",
    );
    expect(currentSelectionPublic).not.toHaveProperty(
      "workstationDetailMessagesByLocale",
    );
  });
});
