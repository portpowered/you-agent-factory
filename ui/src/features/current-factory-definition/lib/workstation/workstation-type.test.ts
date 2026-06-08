import { describe, expect, it } from "vitest";

import {
  DEFAULT_WORKSTATION_TYPE,
  resolveEditableWorkstationType,
  resolveEditableWorkstationTypeOptions,
  supportsEditableWorkstationTypeConversion,
} from "./workstation-type";

describe("workstation type helpers", () => {
  it("defaults missing workstation types to MODEL_WORKSTATION", () => {
    expect(resolveEditableWorkstationType({})).toBe(DEFAULT_WORKSTATION_TYPE);
  });

  it("limits LOGICAL_MOVE workstations to their current type", () => {
    expect(resolveEditableWorkstationTypeOptions("LOGICAL_MOVE")).toEqual([
      "LOGICAL_MOVE",
    ]);
    expect(supportsEditableWorkstationTypeConversion("LOGICAL_MOVE")).toBe(
      false,
    );
  });

  it("allows conversion between MODEL_WORKSTATION and MODEL_INVOKE", () => {
    expect(resolveEditableWorkstationTypeOptions("MODEL_WORKSTATION")).toEqual([
      "MODEL_WORKSTATION",
      "MODEL_INVOKE",
    ]);
    expect(supportsEditableWorkstationTypeConversion("MODEL_WORKSTATION")).toBe(
      true,
    );
    expect(supportsEditableWorkstationTypeConversion("MODEL_INVOKE")).toBe(true);
  });
});
