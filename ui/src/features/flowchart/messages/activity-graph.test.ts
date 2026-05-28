import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import { getActivityGraphMessages } from "./activity-graph";

function place(overrides: Partial<DashboardPlaceRef>): DashboardPlaceRef {
  return {
    kind: "work_state",
    place_id: "story:queued",
    state_category: "QUEUED",
    state_value: "queued",
    type_id: "story",
    ...overrides,
  } as DashboardPlaceRef;
}

describe("getActivityGraphMessages", () => {
  it("resolves default graph labels and count helpers", () => {
    const messages = getActivityGraphMessages("en");

    expect(messages.graphSemanticIconLabel("active-work")).toBe("Active work");
    expect(messages.placeKindLabel(place({ state_category: "TERMINAL" }))).toBe(
      "Terminal",
    );
    expect(
      messages.placeSemanticIconLabel(place({ state_category: "PROCESSING" })),
    ).toBe("Processing state");
    expect(messages.tokenCountLabel(place({ kind: "resource" }), 2)).toBe(
      "2 resource tokens",
    );
    expect(messages.activeItemCountLabel(1)).toBe("1 active item");
  });

  it("resolves zh-CN graph labels and count helpers", () => {
    const messages = getActivityGraphMessages("zh-CN");

    expect(messages.graphSemanticIconLabel("active-work")).toBe("活动工作");
    expect(messages.workstationIconLabel("REPEATER")).toBe("重复器工作站");
    expect(messages.placeKindLabel(place({ state_category: "FAILED" }))).toBe(
      "失败状态",
    );
    expect(messages.tokenCountLabel(place({ kind: "resource" }), 2)).toBe(
      "2 个资源令牌",
    );
    expect(messages.activeItemCountLabel(3)).toBe("3 个活动项");
  });
});
