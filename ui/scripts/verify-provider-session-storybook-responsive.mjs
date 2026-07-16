export async function verifyProviderSessionDetailSuccess({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  await verifyCollapsedSummary({ expectVisible, page });
  await verifyDefaultOpenTranscript({ expectVisible, page });
  await verifyExpandedSelectedSession({ expectVisible, page });
  await expectNoHorizontalOverflow(
    page,
    `Provider-session detail success at ${viewport.label}`,
  );
}

async function verifyCollapsedSummary({ expectVisible, page }) {
  const expectedPreviewLabels = [
    "Session ID",
    "Input Tokens",
    "Output Tokens",
    "Cached Tokens",
    "Source File",
  ];
  const expectedPreviewValues = [
    "019e44f4-580e-7f32-981e-1e54ec6907d6",
    "32",
    "18",
    "0",
    "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
  ];
  await page.waitForSelector(
    '[data-provider-session-verification-ready="true"]',
    { state: "attached" },
  );
  const providerSessionHeading = page.getByRole("heading", {
    name: "Selected session details",
  });
  await expectVisible(
    providerSessionHeading,
    "Selected session details heading",
  );
  await page
    .getByRole("button", { name: "Expand Selected Session Details" })
    .waitFor({ state: "visible" });
  const selectedSessionSection = providerSessionHeading.locator(
    "xpath=ancestor::section[1]",
  );
  await expectTextList(
    selectedSessionSection.locator("dt"),
    expectedPreviewLabels,
    "Selected-session preview labels",
  );
  await expectTextList(
    selectedSessionSection.locator("dd"),
    expectedPreviewValues,
    "Selected-session preview values",
  );
  await expectCount(
    page.getByRole("heading", { name: "Source file" }),
    0,
    "legacy Source file section heading",
  );
  await expectCount(
    page.getByRole("heading", { name: "Session analysis" }),
    0,
    "collapsed Session analysis heading",
  );
}

async function verifyDefaultOpenTranscript({ expectVisible, page }) {
  const transcriptToggle = page.getByRole("button", {
    name: "Collapse Transcript",
  });
  await expectExpanded(transcriptToggle, true, "Transcript disclosure");
  const entryToggles = page.getByRole("button", {
    name: /^Collapse (User|Assistant|Reasoning|read_file|call_1|task_started)$/,
  });
  await expectCount(entryToggles, 6, "default-open transcript entries");
  for (let index = 0; index < 6; index += 1) {
    await expectExpanded(
      entryToggles.nth(index),
      true,
      `Transcript entry ${index + 1}`,
    );
  }
  await expectVisible(
    page.getByText("Summarize the rollout state for this work item."),
    "Default-open transcript message body",
  );

  const userMessageToggle = page.getByRole("button", {
    name: "Collapse User",
  });
  await userMessageToggle.click();
  const collapsedUserMessageToggle = page.getByRole("button", {
    name: "Expand User",
  });
  await expectExpanded(
    collapsedUserMessageToggle,
    false,
    "User message disclosure",
  );
  await collapsedUserMessageToggle.click();
  await expectExpanded(
    page.getByRole("button", { name: "Collapse User" }),
    true,
    "User message disclosure",
  );
  await transcriptToggle.click();
  const collapsedTranscriptToggle = page.getByRole("button", {
    name: "Expand Transcript",
  });
  await expectExpanded(
    collapsedTranscriptToggle,
    false,
    "Transcript disclosure",
  );
  await collapsedTranscriptToggle.click();
  await expectExpanded(
    page.getByRole("button", { name: "Collapse Transcript" }),
    true,
    "Transcript disclosure",
  );
}

async function verifyExpandedSelectedSession({ expectVisible, page }) {
  const selectedSessionToggle = page.getByRole("button", {
    name: "Expand Selected Session Details",
  });
  await expectExpanded(
    selectedSessionToggle,
    false,
    "Selected-session disclosure",
  );
  await selectedSessionToggle.click();
  await expectExpanded(
    page.getByRole("button", { name: "Collapse Selected Session Details" }),
    true,
    "Selected-session disclosure",
  );
  await expectVisible(
    page.getByRole("heading", { name: "Source Metadata" }),
    "Consolidated source metadata heading",
  );
  await expectVisible(
    page.getByRole("heading", { name: "Session Analysis" }),
    "Consolidated session analysis heading",
  );
  const headingNames = await page.getByRole("heading").allTextContents();
  const analysisIndex = headingNames.indexOf("Session Analysis");
  const transcriptIndex = headingNames.indexOf("Transcript");
  if (
    analysisIndex === -1 ||
    transcriptIndex === -1 ||
    analysisIndex >= transcriptIndex
  ) {
    throw new Error(
      "Selected-session analysis did not appear before the Transcript section.",
    );
  }
}

async function expectCount(locator, expected, label) {
  const actual = await locator.count();
  if (actual !== expected) {
    throw new Error(`${label} count was ${actual}; expected ${expected}.`);
  }
}

async function expectExpanded(locator, expected, label) {
  const actual = await locator.getAttribute("aria-expanded");
  if (actual !== String(expected)) {
    throw new Error(
      `${label} aria-expanded was ${String(actual)}; expected ${String(expected)}.`,
    );
  }
}

async function expectTextList(locator, expected, label) {
  const actual = (await locator.allTextContents()).map((value) => value.trim());
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${label} were ${JSON.stringify(actual)}; expected ${JSON.stringify(expected)}.`,
    );
  }
}
