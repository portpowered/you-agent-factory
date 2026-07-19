import { describe, expect, it } from "vitest";

import { getHostedTopologyReplayMessages } from "./hosted-topology-replay";

describe("hosted topology replay messages", () => {
  it("localizes the required active Work and chrome labels", () => {
    const english = getHostedTopologyReplayMessages("en").topology;
    const chinese = getHostedTopologyReplayMessages("zh-CN").topology;

    expect(english.activeWorkRows(2)).toBe("2 active Work");
    expect(english.activeWorkDuration(4)).toBe("4 logical ticks");
    expect(english.activeWorkOverflow(3)).toBe("+3 more active Work");
    expect(english.legendLabel).toBe("Factory topology legend");
    expect(english.hideNodeKinds).toBe("Hide node kinds");
    expect(english.showNodeKinds).toBe("Show node kinds");

    expect(chinese.activeWorkRows(2)).toBe("2 个活动工作");
    expect(chinese.activeWorkDuration(4)).toBe("4 个逻辑时刻");
    expect(chinese.activeWorkOverflow(3)).toBe("还有 3 个活动工作");
    expect(chinese.legendLabel).toBe("工厂拓扑图例");
    expect(chinese.hideNodeKinds).toBe("隐藏节点类型");
    expect(chinese.showNodeKinds).toBe("显示节点类型");
  });
});
