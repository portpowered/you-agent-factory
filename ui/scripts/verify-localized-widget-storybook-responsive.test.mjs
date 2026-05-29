import { describe, expect, test, vi } from "vitest";

import {
  verifyLocalizedCurrentSelection,
  verifyLocalizedSubmitWorkCard,
  verifyLocalizedTraceGrid,
  verifyLocalizedWorkOutcomeChart,
  verifyLocalizedWorkflowActivity,
} from "./verify-localized-widget-storybook-responsive.mjs";

function createVisibleLocator() {
  return {
    first: vi.fn(function first() {
      return this;
    }),
    isVisible: vi.fn().mockResolvedValue(true),
    waitFor: vi.fn().mockResolvedValue(undefined),
  };
}

function createViewport() {
  return { height: 844, label: "mobile", width: 390 };
}

function createSubmitWorkHarness() {
  const workType = createVisibleLocator();
  const requestName = createVisibleLocator();
  const submissionItems = createVisibleLocator();
  const requestBody = createVisibleLocator();
  const submitButton = createVisibleLocator();
  const guidance = createVisibleLocator();
  const card = {
    ...createVisibleLocator(),
    getByRole: vi.fn((role, options) => {
      if (role === "combobox") {
        return workType;
      }
      if (options?.name === "请求名称") {
        return requestName;
      }
      if (role === "list") {
        return submissionItems;
      }
      if (role === "textbox") {
        return requestBody;
      }
      return submitButton;
    }),
    getByText: vi.fn().mockReturnValue(guidance),
  };

  return {
    card,
    expectNoHorizontalOverflow: vi.fn().mockResolvedValue(undefined),
    expectVisible: vi.fn().mockResolvedValue(undefined),
    page: {
      getByRole: vi.fn().mockReturnValue(card),
    },
    submissionItems,
  };
}

function createTraceGridHarness() {
  const traceSummary = createVisibleLocator();
  const dispatchFlow = createVisibleLocator();
  const traceIdLabel = createVisibleLocator();
  const workstationLabel = createVisibleLocator();
  const traceIdValue = createVisibleLocator();
  const card = {
    ...createVisibleLocator(),
    getByText: vi.fn((text) => {
      if (text === "追踪分派表") {
        return traceSummary;
      }
      if (text === "分派流") {
        return dispatchFlow;
      }
      if (text === "追踪 ID") {
        return traceIdLabel;
      }
      if (text === "工作站") {
        return workstationLabel;
      }
      return traceIdValue;
    }),
  };

  return {
    card,
    expectNoHorizontalOverflow: vi.fn().mockResolvedValue(undefined),
    expectVisible: vi.fn().mockResolvedValue(undefined),
    page: {
      getByRole: vi.fn().mockReturnValue(card),
    },
  };
}

function createWorkOutcomeHarness() {
  const chart = createVisibleLocator();
  const queued = createVisibleLocator();
  const inFlight = createVisibleLocator();
  const completed = createVisibleLocator();
  const xAxis = createVisibleLocator();
  const yAxis = createVisibleLocator();
  const card = {
    ...createVisibleLocator(),
    getByRole: vi.fn().mockReturnValue(chart),
    getByText: vi.fn((text) => {
      if (text === "排队中") {
        return queued;
      }
      if (text === "进行中") {
        return inFlight;
      }
      if (text === "已完成") {
        return completed;
      }
      if (text === "刻度") {
        return xAxis;
      }
      return yAxis;
    }),
  };

  return {
    card,
    expectNoHorizontalOverflow: vi.fn().mockResolvedValue(undefined),
    expectVisible: vi.fn().mockResolvedValue(undefined),
    page: {
      getByRole: vi.fn().mockReturnValue(card),
    },
    yAxis,
  };
}

function createWorkflowActivityHarness() {
  const heading = createVisibleLocator();
  const eyebrow = createVisibleLocator();
  const viewportRegion = createVisibleLocator();
  const workstationButton = createVisibleLocator();
  const dataValue = createVisibleLocator();

  return {
    expectNoHorizontalOverflow: vi.fn().mockResolvedValue(undefined),
    expectVisible: vi.fn().mockResolvedValue(undefined),
    page: {
      getByRole: vi.fn((role, _options) => {
        if (role === "heading") {
          return heading;
        }
        if (role === "region") {
          return viewportRegion;
        }
        return workstationButton;
      }),
      getByText: vi.fn((text) => (text === "操作员视图" ? eyebrow : dataValue)),
    },
  };
}

function createCurrentSelectionHarness() {
  const controls = {
    ...createVisibleLocator(),
    getByRole: vi.fn().mockReturnValue({
      click: vi.fn().mockResolvedValue(undefined),
    }),
  };
  const undoButton = createVisibleLocator();
  const activeWorkHeading = createVisibleLocator();
  const runHistoryHeading = createVisibleLocator();
  const workstationData = createVisibleLocator();
  const workstationDataQuery = {
    first: vi.fn().mockReturnValue(workstationData),
  };
  const selectionCard = {
    ...createVisibleLocator(),
    getByRole: vi.fn((_role, options) => {
      if (options?.name === "撤销所选内容") {
        return undoButton;
      }
      if (options?.name === "活动工作") {
        return activeWorkHeading;
      }
      return runHistoryHeading;
    }),
    getByText: vi.fn().mockReturnValue(workstationDataQuery),
  };
  const workstationButton = {
    focus: vi.fn().mockResolvedValue(undefined),
  };

  return {
    controls,
    expectNoHorizontalOverflow: vi.fn().mockResolvedValue(undefined),
    expectVisible: vi.fn().mockResolvedValue(undefined),
    page: {
      getByRole: vi.fn((role, options) => {
        if (role === "group") {
          return controls;
        }
        if (role === "button" && options?.name === "选择 Implement 工作站") {
          return workstationButton;
        }
        return selectionCard;
      }),
      keyboard: {
        press: vi.fn().mockResolvedValue(undefined),
      },
    },
    selectionCard,
    workstationButton,
  };
}

describe("verify-localized-widget-storybook-responsive", () => {
  test("verifyLocalizedSubmitWorkCard checks the localized submit-work controls", async () => {
    const {
      card,
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      submissionItems,
    } =
      createSubmitWorkHarness();

    await verifyLocalizedSubmitWorkCard({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: createViewport(),
    });

    expect(page.getByRole).toHaveBeenCalledWith("article", { name: "提交工作" });
    expect(card.waitFor).toHaveBeenCalledWith({ state: "visible" });
    expect(card.getByRole).toHaveBeenCalledWith("combobox", { name: "工作类型" });
    expect(card.getByRole).toHaveBeenCalledWith("textbox", { name: "请求名称" });
    expect(card.getByRole).toHaveBeenCalledWith("list", { name: "提交项" });
    expect(card.getByRole).toHaveBeenCalledWith("textbox", {
      name: "文本项 1",
    });
    expect(card.getByRole).toHaveBeenCalledWith("button", {
      exact: true,
      name: "提交工作",
    });
    expect(expectVisible).toHaveBeenCalledWith(
      submissionItems,
      "Localized submission-items list",
    );
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Localized submit work card at mobile",
    );
  });

  test("verifyLocalizedTraceGrid checks the localized trace drilldown surface", async () => {
    const { card, expectNoHorizontalOverflow, expectVisible, page } =
      createTraceGridHarness();

    await verifyLocalizedTraceGrid({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: createViewport(),
    });

    expect(page.getByRole).toHaveBeenCalledWith("article", { name: "追踪下钻" });
    expect(card.waitFor).toHaveBeenCalledWith({ state: "visible" });
    expect(card.getByText).toHaveBeenCalledWith("追踪分派表");
    expect(card.getByText).toHaveBeenCalledWith("分派流");
    expect(card.getByText).toHaveBeenCalledWith("追踪 ID");
    expect(card.getByText).toHaveBeenCalledWith("工作站");
    expect(card.getByText).toHaveBeenCalledWith("trace-active-story");
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Localized trace drilldown card at mobile",
    );
  });

  test("verifyLocalizedWorkOutcomeChart checks the localized chart labels", async () => {
    const { card, expectNoHorizontalOverflow, expectVisible, page, yAxis } =
      createWorkOutcomeHarness();

    await verifyLocalizedWorkOutcomeChart({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: createViewport(),
    });

    expect(page.getByRole).toHaveBeenCalledWith("article", { name: "工作结果图表" });
    expect(card.getByRole).toHaveBeenCalledWith("img", {
      name: "15m 的工作结果图表",
    });
    expect(card.getByText).toHaveBeenCalledWith("排队中");
    expect(card.getByText).toHaveBeenCalledWith("进行中");
    expect(card.getByText).toHaveBeenCalledWith("已完成");
    expect(card.getByText).toHaveBeenCalledWith("刻度");
    expect(card.getByText).toHaveBeenCalledWith("工作计数");
    expect(yAxis.first).toHaveBeenCalledTimes(1);
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Localized work outcome chart at mobile",
    );
  });

  test("verifyLocalizedWorkflowActivity checks the localized workflow activity surface", async () => {
    const { expectNoHorizontalOverflow, expectVisible, page } =
      createWorkflowActivityHarness();

    await verifyLocalizedWorkflowActivity({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: createViewport(),
    });

    expect(page.getByRole).toHaveBeenCalledWith("region", { name: "当前活动" });
    expect(page.getByRole).toHaveBeenCalledWith("region", {
      name: "工作图视口",
    });
    expect(page.getByRole).toHaveBeenCalledWith("button", {
      name: "进入工厂图编辑器",
    });
    expect(page.getByText).toHaveBeenCalledWith("观察模式");
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Localized workflow activity card at mobile",
    );
  });
});

describe("verifyLocalizedCurrentSelection", () => {
  test("verifyLocalizedCurrentSelection checks localized card copy and keyboard selection", async () => {
    const {
      controls,
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      selectionCard,
    } = createCurrentSelectionHarness();

    await verifyLocalizedCurrentSelection({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: createViewport(),
    });

    expect(page.getByRole).toHaveBeenCalledWith("group", {
      name: "Locale verification controls",
    });
    expect(controls.getByRole).toHaveBeenCalledWith("button", {
      name: "Switch to zh-CN",
    });
    expect(page.getByRole).toHaveBeenCalledWith("article", {
      name: "当前选择",
    });
    expect(selectionCard.getByRole).toHaveBeenCalledWith("button", {
      name: "撤销所选内容",
    });
    expect(selectionCard.getByRole).toHaveBeenCalledWith("heading", {
      name: "活动工作",
    });
    expect(selectionCard.getByRole).toHaveBeenCalledWith("heading", {
      name: "运行历史",
    });
    expect(selectionCard.getByText).toHaveBeenCalledWith("Review", {
      exact: true,
    });
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Localized current-selection card at mobile",
    );
  });
});
