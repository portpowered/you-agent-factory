import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import type { GraphSemanticIconKind } from "../components/graph-semantic-icon";
import type { WorkstationSemanticKind } from "../lib/workstation-icon-metadata";
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: semantic label matrices stay close to the locale helper they cover.
describe("getActivityGraphMessages", () => {
  it("resolves default graph labels and count helpers", () => {
    const messages = getActivityGraphMessages("en");
    const semanticIconLabels: Array<[GraphSemanticIconKind, string]> = [
      ["active-work", "Active work"],
      ["constraint", "Constraint"],
      ["cron", "Cron workstation"],
      ["exhaustion", "Exhaustion rule"],
      ["failed", "Failed state"],
      ["limit", "Limit"],
      ["poller", "Poller workstation"],
      ["processing", "Processing state"],
      ["queue", "Queue state"],
      ["repeater", "Repeater workstation"],
      ["resource", "Resource"],
      ["terminal", "Terminal state"],
      ["worker", "Worker"],
      ["workstation", "Workstation"],
      ["work-type", "Work type"],
    ];
    const workstationLabels: Array<[WorkstationSemanticKind, string]> = [
      ["CRON", "Cron workstation"],
      ["POLLER", "Poller workstation"],
      ["REPEATER", "Repeater workstation"],
      ["STANDARD", "Standard workstation"],
      ["exhaustion", "Exhaustion rule"],
    ];

    for (const [kind, label] of semanticIconLabels) {
      expect(messages.graphSemanticIconLabel(kind)).toBe(label);
    }
    for (const [kind, label] of workstationLabels) {
      expect(messages.workstationIconLabel(kind)).toBe(label);
    }
    expect(messages.placeKindLabel(place({ state_category: "TERMINAL" }))).toBe(
      "Terminal",
    );
    expect(messages.placeKindLabel(place({ state_category: "FAILED" }))).toBe(
      "Failed",
    );
    expect(messages.placeKindLabel(place({ kind: "resource" }))).toBe(
      "Resource",
    );
    expect(messages.placeKindLabel(place({ kind: "limit" }))).toBe("Limit");
    expect(messages.placeKindLabel(place({ kind: "constraint" }))).toBe(
      "Constraint",
    );
    expect(
      messages.placeSemanticIconLabel(place({ state_category: "PROCESSING" })),
    ).toBe("Processing state");
    expect(messages.tokenCountLabel(place({ kind: "resource" }), 2)).toBe(
      "2 resource tokens",
    );
    expect(messages.tokenCountLabel(place({ kind: "limit" }), 1)).toBe(
      "1 limit token",
    );
    expect(messages.tokenCountLabel(place({ kind: "constraint" }), 3)).toBe(
      "3 constraint tokens",
    );
    expect(messages.activeItemCountLabel(1)).toBe("1 active item");
    expect(messages.activeItemCountLabel(2)).toBe("2 active items");
  });

  it("resolves zh-CN graph labels and count helpers", () => {
    const messages = getActivityGraphMessages("zh-CN");
    const semanticIconLabels: Array<[GraphSemanticIconKind, string]> = [
      ["active-work", "活动工作"],
      ["constraint", "约束"],
      ["cron", "Cron 工作站"],
      ["exhaustion", "耗尽规则"],
      ["failed", "失败状态"],
      ["limit", "限制"],
      ["poller", "轮询器工作站"],
      ["processing", "处理中状态"],
      ["queue", "队列状态"],
      ["repeater", "重复器工作站"],
      ["resource", "资源"],
      ["terminal", "终止状态"],
      ["worker", "工作者"],
      ["workstation", "工作站"],
      ["work-type", "工作类型"],
    ];
    const workstationLabels: Array<[WorkstationSemanticKind, string]> = [
      ["CRON", "Cron 工作站"],
      ["POLLER", "轮询器工作站"],
      ["REPEATER", "重复器工作站"],
      ["STANDARD", "标准工作站"],
      ["exhaustion", "耗尽规则"],
    ];

    for (const [kind, label] of semanticIconLabels) {
      expect(messages.graphSemanticIconLabel(kind)).toBe(label);
    }
    for (const [kind, label] of workstationLabels) {
      expect(messages.workstationIconLabel(kind)).toBe(label);
    }
    expect(messages.placeKindLabel(place({ state_category: "TERMINAL" }))).toBe(
      "终止状态",
    );
    expect(messages.placeKindLabel(place({ state_category: "FAILED" }))).toBe(
      "失败状态",
    );
    expect(messages.placeKindLabel(place({ state_category: "QUEUED" }))).toBe(
      "队列",
    );
    expect(messages.placeKindLabel(place({ kind: "resource" }))).toBe("资源");
    expect(messages.placeKindLabel(place({ kind: "limit" }))).toBe("限制");
    expect(messages.placeKindLabel(place({ kind: "constraint" }))).toBe("约束");
    expect(
      messages.placeSemanticIconLabel(place({ state_category: "PROCESSING" })),
    ).toBe("处理中状态");
    expect(messages.tokenCountLabel(place({ kind: "resource" }), 2)).toBe(
      "2 个资源令牌",
    );
    expect(messages.tokenCountLabel(place({ kind: "constraint" }), 3)).toBe(
      "3 个约束令牌",
    );
    expect(messages.activeItemCountLabel(3)).toBe("3 个活动项");
  });
});
