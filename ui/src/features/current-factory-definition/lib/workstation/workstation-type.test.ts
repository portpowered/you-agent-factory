import { describe, expect, it } from "vitest";

import { WorkstationType } from "../../../../api/generated/openapi";
import {
  DEFAULT_WORKSTATION_TYPE,
  resolveEditableWorkstationType,
  resolveEditableWorkstationTypeOptions,
  supportsEditableWorkstationTypeConversion,
} from "./workstation-type";

describe("workstation type helpers", () => {
  it("defaults missing workstation types to AGENT_RUN", () => {
    expect(resolveEditableWorkstationType({})).toBe(DEFAULT_WORKSTATION_TYPE);
    expect(DEFAULT_WORKSTATION_TYPE).toBe(WorkstationType.AGENT_RUN);
  });

  it("limits LOGICAL_MOVE workstations to their current type", () => {
    expect(resolveEditableWorkstationTypeOptions("LOGICAL_MOVE")).toEqual([
      "LOGICAL_MOVE",
    ]);
    expect(supportsEditableWorkstationTypeConversion("LOGICAL_MOVE")).toBe(
      false,
    );
  });

  it("allows conversion between AGENT_RUN and INFERENCE_RUN", () => {
    expect(resolveEditableWorkstationTypeOptions("AGENT_RUN")).toEqual([
      "AGENT_RUN",
      "INFERENCE_RUN",
    ]);
    expect(supportsEditableWorkstationTypeConversion("AGENT_RUN")).toBe(true);
    expect(supportsEditableWorkstationTypeConversion("INFERENCE_RUN")).toBe(
      true,
    );
  });

  it("preserves legacy runnable workstation types in conversion options", () => {
    expect(resolveEditableWorkstationTypeOptions("MODEL_WORKSTATION")).toEqual([
      "MODEL_WORKSTATION",
      "AGENT_RUN",
      "INFERENCE_RUN",
    ]);
    expect(resolveEditableWorkstationTypeOptions("MODEL_INVOKE")).toEqual([
      "MODEL_INVOKE",
      "AGENT_RUN",
      "INFERENCE_RUN",
    ]);
  });
});
