export async function verifyLocalizedSubmitWorkCard({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const card = page.getByRole("article", { name: "提交工作" });
  await card.waitFor({ state: "visible" });
  await expectVisible(card, "Localized submit work card");
  await expectVisible(
    card.getByRole("combobox", { name: "工作类型" }),
    "Localized work type select",
  );
  await expectVisible(
    card.getByRole("textbox", { name: "请求名称" }),
    "Localized request name input",
  );
  await expectVisible(
    card.getByRole("textbox", { exact: true, name: "请求" }),
    "Localized request textarea",
  );
  await expectVisible(
    card.getByRole("button", { exact: true, name: "提交工作" }),
    "Localized submit work button",
  );
  await expectVisible(
    card.getByText("先选择工作类型并填写请求名称，然后即可继续。"),
    "Localized submit work guidance",
  );
  await expectNoHorizontalOverflow(
    page,
    `Localized submit work card at ${viewport.label}`,
  );
}

export async function verifyLocalizedTraceGrid({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const card = page.getByRole("article", { name: "追踪下钻" });
  await card.waitFor({ state: "visible" });
  await expectVisible(card, "Localized trace drilldown card");
  await expectVisible(card.getByText("追踪分派表"), "Localized trace summary");
  await expectVisible(
    card.getByText("分派流"),
    "Localized dispatch flow label",
  );
  await expectVisible(
    card.getByText("追踪 ID"),
    "Localized trace id label",
  );
  await expectVisible(
    card.getByText("工作站"),
    "Localized workstation column label",
  );
  await expectVisible(
    card.getByText("trace-active-story"),
    "Trace ID data value",
  );
  await expectNoHorizontalOverflow(
    page,
    `Localized trace drilldown card at ${viewport.label}`,
  );
}

export async function verifyLocalizedWorkOutcomeChart({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const card = page.getByRole("article", { name: "工作结果图表" });
  await card.waitFor({ state: "visible" });
  await expectVisible(card, "Localized work outcome chart card");
  await expectVisible(
    card.getByRole("img", { name: "15m 的工作结果图表" }),
    "Localized work outcome chart",
  );
  await expectVisible(card.getByText("排队中"), "Localized queued label");
  await expectVisible(card.getByText("进行中"), "Localized in-flight label");
  await expectVisible(card.getByText("已完成"), "Localized completed label");
  await expectVisible(card.getByText("刻度"), "Localized chart x-axis label");
  await expectVisible(
    card.getByText("工作计数"),
    "Localized chart y-axis label",
  );
  await expectNoHorizontalOverflow(
    page,
    `Localized work outcome chart at ${viewport.label}`,
  );
}

export async function verifyLocalizedWorkflowActivity({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  await expectVisible(
    page.getByRole("region", { name: "当前活动" }),
    "Localized workflow activity region",
  );
  await expectVisible(
    page.getByRole("region", { name: "工作图视口" }),
    "Localized workflow activity viewport",
  );
  await expectVisible(
    page.getByRole("button", { name: "选择 Review 工作站" }),
    "Localized workflow activity workstation button",
  );
  await expectVisible(
    page.getByText("Active Story"),
    "Active Story data value",
  );
  await expectNoHorizontalOverflow(
    page,
    `Localized workflow activity card at ${viewport.label}`,
  );
}

export async function verifyLocalizedCurrentSelection({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const controls = page.getByRole("group", {
    name: "Locale verification controls",
  });
  await controls.waitFor({ state: "visible" });
  await controls.getByRole("button", { name: "Switch to zh-CN" }).click();

  const currentSelection = page.getByRole("article", { name: "当前选择" });
  await currentSelection.waitFor({ state: "visible" });
  await expectVisible(currentSelection, "Localized current-selection card");
  await expectVisible(
    currentSelection.getByRole("button", { name: "撤销所选内容" }),
    "Localized current-selection undo control",
  );
  await expectVisible(
    currentSelection.getByRole("heading", { name: "活动工作" }),
    "Localized current-selection active-work heading",
  );
  await expectVisible(
    currentSelection.getByRole("heading", { name: "运行历史" }),
    "Localized current-selection run-history heading",
  );
  await expectVisible(
    currentSelection.getByText("Review", { exact: true }).first(),
    "Current-selection initial workstation data value",
  );

  const implementWorkstation = page.getByRole("button", {
    name: "选择 Implement 工作站",
  });
  await implementWorkstation.focus();
  await page.keyboard.press("Enter");

  const selectedWorkstation = page.getByRole("article", { name: "当前选择" });
  await expectVisible(
    selectedWorkstation.getByText("Implement", { exact: true }).first(),
    "Keyboard-selected workstation data value",
  );
  await expectNoHorizontalOverflow(
    page,
    `Localized current-selection card at ${viewport.label}`,
  );
}
