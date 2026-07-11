import { describe, expect, it } from "vitest";

import { getFactorySessionDetailMessages } from "./factory-session-detail";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: locale coverage stays in one shared catalog assertion file.
describe("factory session detail messages", () => {
  it("formats english replay and dispatch detail labels through the shared catalog", () => {
    const messages = getFactorySessionDetailMessages("en");

    expect(messages.eventReplayArtifactCountLabel(2)).toBe(
      "2 Factory Artifacts",
    );
    expect(messages.eventReplayArtifactLabel("artifact-7")).toBe(
      "Factory Artifact artifact-7",
    );
    expect(messages.eventReplayCheckpointLabel("checkpoint-4")).toBe(
      "Checkpoint checkpoint-4",
    );
    expect(messages.eventReplayDispatchLabel("dispatch-9")).toBe(
      "Dispatch dispatch-9",
    );
    expect(
      messages.eventReplayDispatchStatusDetail("FAILED_WITH_PARTIAL"),
    ).toBe("Dispatch status failed with partial");
    expect(messages.eventReplayLifecycleStatusDetail("AWAITING_APPROVAL")).toBe(
      "Lifecycle status awaiting approval",
    );
    expect(messages.eventReplayPhaseLabel("review")).toBe("Phase review");
    expect(messages.eventReplayQueuePositionLabel(3)).toBe("Queue position 3");
    expect(messages.eventReplayResultStatusDetail("FAILED_WITH_PARTIAL")).toBe(
      "Result status failed with partial",
    );
    expect(messages.eventReplaySequenceTickLabel(4, 9)).toBe(
      "Session event 4 · Tick 9",
    );
    expect(messages.eventReplaySequenceLabel(5)).toBe("Session event 5");
    expect(messages.eventReplaySummary(25, 140)).toBe(
      "Showing the latest 25 of 140 Factory Events.",
    );
    expect(messages.eventReplayVisibleSummary(1)).toBe(
      "Showing 1 Factory Event.",
    );
    expect(messages.eventReplayVisibleSummary(3)).toBe(
      "Showing 3 Factory Events.",
    );
    expect(messages.eventReplayWarningCountLabel(1)).toBe("1 warning");
    expect(messages.eventReplayWarningCountLabel(4)).toBe("4 warnings");
    expect(messages.eventReplayWorkLabel(1)).toBe("1 related work item");
    expect(messages.eventReplayWorkLabel(2)).toBe("2 related work items");
    expect(messages.expandDispatchDetailLabel("dispatch-9")).toBe(
      "Expand dispatch detail for dispatch-9",
    );
    expect(messages.collapseDispatchDetailLabel("dispatch-9")).toBe(
      "Collapse dispatch detail for dispatch-9",
    );
    expect(messages.lifecycleActionRetryDispatchLabel).toBe("Retry dispatch");
    expect(messages.lifecycleActionInterruptDispatchLabel).toBe(
      "Interrupt dispatch",
    );
    expect(messages.lifecycleOutcomeAcceptedTitle("Pause")).toBe(
      "Pause accepted",
    );
    expect(messages.lifecycleOutcomeConflictTitle("Pause")).toBe(
      "Pause is blocked by another lifecycle change.",
    );
    expect(messages.lifecycleOutcomeCurrentStatusDetail("Paused")).toBe(
      "Current durable status: Paused.",
    );
    expect(messages.lifecycleControlsSelectedDispatchLabel("dispatch-9")).toBe(
      "Selected dispatch: dispatch-9",
    );
  });

  it("formats zh-CN replay and dispatch detail labels through the shared catalog", () => {
    const messages = getFactorySessionDetailMessages("zh-CN");

    expect(messages.eventReplayArtifactCountLabel(2)).toBe("2 个工厂工件");
    expect(messages.eventReplayArtifactLabel("artifact-7")).toBe(
      "工厂工件 artifact-7",
    );
    expect(messages.eventReplayCheckpointLabel("checkpoint-4")).toBe(
      "检查点 checkpoint-4",
    );
    expect(messages.eventReplayDispatchLabel("dispatch-9")).toBe(
      "调度 dispatch-9",
    );
    expect(
      messages.eventReplayDispatchStatusDetail("FAILED_WITH_PARTIAL"),
    ).toBe("调度状态 failed with partial");
    expect(messages.eventReplayLifecycleStatusDetail("AWAITING_APPROVAL")).toBe(
      "生命周期状态 awaiting approval",
    );
    expect(messages.eventReplayPhaseLabel("review")).toBe("阶段 review");
    expect(messages.eventReplayQueuePositionLabel(3)).toBe("队列位置 3");
    expect(messages.eventReplayResultStatusDetail("FAILED_WITH_PARTIAL")).toBe(
      "结果状态 failed with partial",
    );
    expect(messages.eventReplaySequenceTickLabel(4, 9)).toBe(
      "会话事件 4 · Tick 9",
    );
    expect(messages.eventReplaySequenceLabel(5)).toBe("会话事件 5");
    expect(messages.eventReplaySummary(25, 140)).toBe(
      "显示最新 25 / 140 条工厂事件。",
    );
    expect(messages.eventReplayVisibleSummary(3)).toBe("显示 3 条工厂事件。");
    expect(messages.eventReplayWarningCountLabel(4)).toBe("4 条警告");
    expect(messages.eventReplayWorkLabel(2)).toBe("2 个关联工作项");
    expect(messages.expandDispatchDetailLabel("dispatch-9")).toBe(
      "展开 dispatch-9 的调度详情",
    );
    expect(messages.collapseDispatchDetailLabel("dispatch-9")).toBe(
      "收起 dispatch-9 的调度详情",
    );
    expect(messages.lifecycleActionRetryDispatchLabel).toBe("重试调度");
    expect(messages.lifecycleActionInterruptDispatchLabel).toBe("中断调度");
    expect(messages.lifecycleOutcomeAcceptedTitle("暂停")).toBe(
      "已接受“暂停”请求",
    );
    expect(messages.lifecycleOutcomeConflictTitle("暂停")).toBe(
      "“暂停”被另一个生命周期变更阻塞。",
    );
    expect(messages.lifecycleOutcomeCurrentStatusDetail("已暂停")).toBe(
      "当前持久化状态：已暂停。",
    );
    expect(messages.lifecycleControlsSelectedDispatchLabel("dispatch-9")).toBe(
      "已选择调度：dispatch-9",
    );
  });
});
