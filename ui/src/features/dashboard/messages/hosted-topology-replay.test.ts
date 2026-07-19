import { describe, expect, it } from "vitest";

import {
  getHostedTopologyReplayMessages,
  hostedTopologyReplayMessagesByLocale,
} from "./hosted-topology-replay";

describe("hosted topology replay messages", () => {
  it("provides the required operational topology formatters in English", () => {
    const messages = getHostedTopologyReplayMessages("en");

    expect(messages.formatTick(4)).toBe("Tick 4");
    expect(messages.timeline.position(2, 7)).toBe("2 of 7");
    expect(messages.topology.activeDispatches(1)).toBe("1 active Dispatch");
    expect(messages.topology.activeDispatches(3)).toBe("3 active Dispatches");
    expect(messages.topology.activeWorkDuration(5)).toBe("Active for 5 ticks");
    expect(messages.topology.activeWorkOverflow(2)).toBe("2 more active Work");
    expect(messages.topology.nodeLabel("Work", "Build")).toBe("Select Build Work");
    expect(messages.topology.resourceOccupancy(1, 4)).toBe(
      "1 of 4 resource units occupied",
    );
    expect(messages.topology.workStateCount(1)).toBe("1 Work");
    expect(messages.topology.workStateCount(2)).toBe("2 Work");
  });

  it("keeps topology formatters available in Simplified Chinese", () => {
    const messages = getHostedTopologyReplayMessages("zh-CN");

    expect(messages.formatTick(4)).toBe("时刻 4");
    expect(messages.timeline.position(2, 7)).toBe("2 / 7");
    expect(messages.topology.activeDispatches(3)).toBe("3 个活动派遣");
    expect(messages.topology.activeWorkDuration(5)).toBe("已活动 5 个时刻");
    expect(messages.topology.activeWorkOverflow(2)).toBe("还有 2 个活动工作");
    expect(messages.topology.nodeLabel("工作", "构建")).toBe("选择 构建 工作");
    expect(messages.topology.resourceOccupancy(1, 4)).toBe(
      "4 个资源单位中已占用 1 个",
    );
    expect(messages.topology.workStateCount(2)).toBe("2 个工作");
  });

  it("falls back to English for an unsupported locale", () => {
    expect(getHostedTopologyReplayMessages("fr")).toBe(
      hostedTopologyReplayMessagesByLocale.en,
    );
  });
});
