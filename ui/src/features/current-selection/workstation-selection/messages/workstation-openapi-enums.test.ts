import { describe, expect, it } from "vitest";
import {
  localizeWorkstationKindValue,
  localizeWorkstationTypeValue,
} from "./workstation-openapi-enums";

describe("workstation OpenAPI enum localization", () => {
  it("localizes WorkstationKind values with unknown fallback", () => {
    expect(localizeWorkstationKindValue("STANDARD", "en")).toBe("Standard");
    expect(localizeWorkstationKindValue("REPEATER", "zh-CN")).toBe("重复器");
    expect(localizeWorkstationKindValue("future-kind", "zh-CN")).toBe(
      "未知种类：future-kind",
    );
  });

  it("localizes WorkstationType values with unknown fallback", () => {
    expect(localizeWorkstationTypeValue("MODEL_WORKSTATION", "en")).toBe(
      "Model workstation",
    );
    expect(localizeWorkstationTypeValue("LOGICAL_MOVE", "ja")).toBe("論理移動");
    expect(localizeWorkstationTypeValue("FUTURE_TYPE", "en")).toBe(
      "Unknown type: FUTURE_TYPE",
    );
  });
});
