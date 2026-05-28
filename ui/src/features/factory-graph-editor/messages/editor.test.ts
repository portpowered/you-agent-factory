import {
  factoryGraphEditorMessagesByLocale,
  getFactoryGraphEditorMessages,
} from "./editor";

describe("getFactoryGraphEditorMessages", () => {
  it("supports the required factory-graph-editor locales", () => {
    expect(Object.keys(factoryGraphEditorMessagesByLocale).sort()).toEqual(
      ["en", "zh-CN"].sort(),
    );
  });

  it.each([
    ["en", "Factory graph editor tools", "Add entity", "Observe mode"],
    ["zh-CN", "工厂图编辑器工具", "添加实体", "观察模式"],
  ] as const)("resolves %s editor catalog copy", (locale, expectedToolbarLabel, expectedAddEntityAction, expectedObserveMode) => {
    const messages = getFactoryGraphEditorMessages(locale);

    expect(messages.toolbarAriaLabel).toBe(expectedToolbarLabel);
    expect(messages.addDialogAddEntityAction).toBe(expectedAddEntityAction);
    expect(messages.modeObserve).toBe(expectedObserveMode);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getFactoryGraphEditorMessages("en");

    expect(getFactoryGraphEditorMessages(undefined).toolbarAddLabel).toBe(
      defaultMessages.toolbarAddLabel,
    );
    expect(getFactoryGraphEditorMessages("fr").draftActionsTitle).toBe(
      defaultMessages.draftActionsTitle,
    );
  });

  it.each([
    [
      "workstation",
      "Add workstation",
      "Create a pending workstation in the current graph draft.",
    ],
    [
      "worker",
      "Add worker",
      "Create a pending worker in the current graph draft.",
    ],
    [
      "resource",
      "Add resource",
      "Create a pending resource in the current graph draft.",
    ],
    [
      "work-type",
      "Add work type",
      "Define a new work type and its first ordered state.",
    ],
    [
      "work-state",
      "Add work state",
      "Append a new ordered state to an existing work type.",
    ],
  ] as const)("describes English add-dialog copy for %s drafts", (kind, expectedTitle, expectedDescription) => {
    const messages = getFactoryGraphEditorMessages("en");

    expect(messages.addDialogTitle(kind)).toBe(expectedTitle);
    expect(messages.addDialogDescription(kind)).toBe(expectedDescription);
  });

  it.each([
    ["resource", "Resource"],
    ["worker", "Worker"],
    ["workstation", "Workstation"],
    ["work-type", "Work type"],
    ["work-state", "Work state"],
  ] as const)("describes English graph node kind %s", (kind, expectedLabel) => {
    expect(getFactoryGraphEditorMessages("en").kindLabel(kind)).toBe(
      expectedLabel,
    );
  });

  it.each([
    ["active", "Active"],
    ["errored", "Errored"],
    ["idle", "Idle"],
    ["unavailable", "Unavailable"],
  ] as const)("describes English worker status %s", (status, expectedLabel) => {
    expect(getFactoryGraphEditorMessages("en").workerStatusLabel(status)).toBe(
      expectedLabel,
    );
  });

  it("resolves localized function-backed labels", () => {
    const messages = getFactoryGraphEditorMessages("zh-CN");

    expect(messages.addDialogTitle("work-type")).toBe("添加工作类型");
    expect(messages.addDialogTitle("work-state")).toBe("添加工作状态");
    expect(messages.addDialogTitle("worker")).toBe("添加worker");
    expect(messages.addDialogDescription("workstation")).toBe(
      "在当前图草稿中创建一个待处理工作站。",
    );
    expect(messages.addDialogDescription("work-type")).toBe(
      "定义一个新的工作类型及其首个有序状态。",
    );
    expect(messages.addDialogDescription("work-state")).toBe(
      "向现有工作类型追加一个新的有序状态。",
    );
    expect(messages.addDialogDescription("resource")).toBe(
      "在当前图草稿中创建一个待处理的resource。",
    );
    expect(messages.kindLabel("resource")).toBe("资源");
    expect(messages.kindLabel("worker")).toBe("工作者");
    expect(messages.kindLabel("workstation")).toBe("工作站");
    expect(messages.kindLabel("work-type")).toBe("工作类型");
    expect(messages.kindLabel("work-state")).toBe("工作状态");
    expect(messages.visibilityPresetAllLabel).toBe("全部");
    expect(messages.visibilityPresetWorkflowLabel).toBe("工作流");
    expect(messages.visibilityPresetExecutionLabel).toBe("执行");
    expect(messages.visibilityPresetInfrastructureLabel).toBe("基础设施");
    expect(messages.workerStatusLabel("active")).toBe("活跃");
    expect(messages.workerStatusLabel("errored")).toBe("错误");
    expect(messages.workerStatusLabel("idle")).toBe("空闲");
    expect(messages.workerStatusLabel("unavailable")).toBe("不可用");
  });

  it("resolves localized add-menu, state, validation, connection, and removal helpers", () => {
    const messages = getFactoryGraphEditorMessages("zh-CN");

    expect(messages.addMenuAction("workstation")).toEqual({
      description: "创建一个待处理工作站并分配现有工作者。",
      label: "工作站",
    });
    expect(messages.stateTypeLabel("PROCESSING")).toBe("处理中");
    expect(
      messages.connectionIncompatibleNotice(
        "失败",
        "review",
        "继续",
        "story:queued",
      ),
    ).toBe("review 的失败连接不能连接到 story:queued 上的继续。");
    expect(messages.validationMissingWorkerAssignment("review")).toBe(
      "工作站“review”必须保留一个工作者分配。",
    );
    expect(
      messages.removalEdgeDescription("worker-assignment", "writer", "review"),
    ).toBe(
      "这会将 writer 从 review 取消分配。该工作站需要另一个工作者后拓扑保存才能成功。",
    );
    expect(
      messages.saveSummaryDescription({
        changedEdges: 1,
        createdEntities: 2,
        removedEntities: 1,
      }),
    ).toBe("此保存将应用 2 个新增实体、1 个删除实体 和 1 条更改边。");
  });

  it.each(["en", "zh-CN"] as const)(
    "exercises the full %s dynamic message catalog",
    (locale) => {
      const messages = getFactoryGraphEditorMessages(locale);
      const addKinds = [
        "workstation",
        "worker",
        "resource",
        "work-type",
        "work-state",
      ] as const;
      const anchorIds = [
        "worker-resource-source",
        "workstation-resource-source",
        "worker-input-target",
        "worker-assignment-source",
        "workstation-input-source",
        "workstation-input-target",
        "workstation-output-source",
        "work-state-input-target",
        "workstation-on-continue-source",
        "work-state-input-target",
        "workstation-on-failure-source",
        "work-state-input-target",
        "workstation-on-rejection-source",
        "work-state-input-target",
        "worker-assignment-target",
        "workstation-resource-target",
        "unknown-anchor",
      ] as const;
      const edgeKinds = [
        "worker-assignment",
        "worker-resource",
        "work-type-state",
        "workstation-input",
        "workstation-output",
        "workstation-on-continue",
        "workstation-on-failure",
        "workstation-on-rejection",
        "workstation-resource",
      ] as const;

      for (const kind of addKinds) {
        expect(messages.addDialogTitle(kind)).toEqual(expect.any(String));
        expect(messages.addDialogDescription(kind)).toEqual(expect.any(String));
        expect(messages.addMenuAction(kind)).toEqual({
          description: expect.any(String),
          label: expect.any(String),
        });
        expect(messages.kindLabel(kind)).toEqual(expect.any(String));
        expect(messages.validationMissingRequiredIdentifier(kind)).toEqual(
          expect.any(String),
        );
        expect(
          messages.removalDescription({
            connectedEdgeCount: kind === "worker" ? 0 : 2,
            impactedStateCount: 3,
            kind,
            label: "story",
          }),
        ).toEqual(expect.any(String));
        expect(messages.removalEntityConfirmLabel("story", kind)).toEqual(
          expect.any(String),
        );
        expect(messages.removalEntityTitle("story", kind)).toEqual(
          expect.any(String),
        );
      }

      for (const anchorId of anchorIds) {
        expect(messages.connectionAnchorDescription(anchorId)).toEqual(
          expect.any(String),
        );
        expect(messages.connectionAnchorLabel(anchorId)).toEqual(
          expect.any(String),
        );
      }

      for (const edgeKind of edgeKinds) {
        expect(messages.edgeKindLabel(edgeKind)).toEqual(expect.any(String));
        expect(messages.removalEdgeDescription(edgeKind, "source", "target")).toEqual(
          expect.any(String),
        );
        expect(messages.removalEdgeLabel(edgeKind, "source")).toEqual(
          expect.any(String),
        );
      }

      for (const stateType of [
        "INITIAL",
        "PROCESSING",
        "TERMINAL",
        "FAILED",
      ] as const) {
        expect(messages.stateTypeLabel(stateType)).toEqual(expect.any(String));
      }

      for (const status of [
        "active",
        "errored",
        "idle",
        "unavailable",
      ] as const) {
        expect(messages.workerStatusLabel(status)).toEqual(expect.any(String));
      }

      expect(messages.edgeAriaLabel("route", "source", "target")).toEqual(
        expect.any(String),
      );
      expect(messages.modeClassifierRoutesUnavailable("classifier")).toEqual(
        expect.any(String),
      );
      expect(
        messages.connectionIncompatibleNotice("output", "review", "input", "done"),
      ).toEqual(expect.any(String));
      expect(messages.validationDuplicateIdentifier("duplicate")).toEqual(
        expect.any(String),
      );
      expect(
        messages.validationIncompatibleEdge("route", "source", "target"),
      ).toEqual(expect.any(String));
      expect(messages.validationMissingWorkerAssignment("review")).toEqual(
        expect.any(String),
      );
      expect(messages.validationUnknownEdgeNode("route", "source")).toEqual(
        expect.any(String),
      );
      expect(messages.validationUnknownEdgeNode("route", "target")).toEqual(
        expect.any(String),
      );
      expect(messages.saveSummaryDescription({
        changedEdges: 0,
        createdEntities: 0,
        removedEntities: 0,
      })).toEqual(expect.any(String));
      expect(messages.saveSummaryDescription({
        changedEdges: 1,
        createdEntities: 0,
        removedEntities: 0,
      })).toEqual(expect.any(String));
      expect(messages.saveSummaryDescription({
        changedEdges: 1,
        createdEntities: 1,
        removedEntities: 1,
      })).toEqual(expect.any(String));
      expect(messages.removalEdgeConfirmLabel("route")).toEqual(expect.any(String));
      expect(messages.removalEdgeTitle("route")).toEqual(expect.any(String));
      expect(messages.removalWorkerAssignedReason(2, "writer")).toEqual(
        expect.any(String),
      );
    },
  );
});
