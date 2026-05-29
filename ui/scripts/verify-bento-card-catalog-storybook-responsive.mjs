const BENTO_CARD_NAMES = [
  "Work totals",
  "Factory graph",
  "Current selection",
  "Provider session",
  "Submit work",
  "Work outcome chart",
  "Trace drill-down",
  "Completed and failed work",
  "Add widget",
];

async function expectFocusable(locator, label) {
  await locator.focus();
  const isFocused = await locator.evaluate(
    (element) => element === document.activeElement,
  );

  if (!isFocused) {
    throw new Error(`${label} was not keyboard focusable.`);
  }
}

async function expectBentoGeometry(page, viewport) {
  const geometry = await page.locator("[data-bento-card-id]").evaluateAll(
    (elements) =>
      elements.map((element) => {
        const rect = element.getBoundingClientRect();
        return {
          height: rect.height,
          width: rect.width,
          x: rect.x,
        };
      }),
  );

  if (geometry.length < BENTO_CARD_NAMES.length) {
    throw new Error(
      `Expected ${BENTO_CARD_NAMES.length} bento cards, found ${geometry.length}.`,
    );
  }

  for (const [index, item] of geometry.entries()) {
    if (item.width > viewport.width + 4) {
      throw new Error(
        `Bento card ${index + 1} exceeded the ${viewport.label} viewport width.`,
      );
    }

    if (item.height <= 0) {
      throw new Error(`Bento card ${index + 1} did not render visible height.`);
    }
  }

  const distinctColumns = new Set(geometry.map((item) => Math.round(item.x)));

  if (viewport.label === "mobile" && distinctColumns.size !== 1) {
    throw new Error("Mobile bento catalog did not stack cards into one column.");
  }

  if (viewport.label === "desktop" && distinctColumns.size < 2) {
    throw new Error("Desktop bento catalog did not preserve multi-column sizing.");
  }
}

export async function verifyBentoCardCatalogResponsive({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const board = page.getByRole("region", {
    name: "you-agent-factory bento board",
  });
  await expectVisible(board, "Responsive bento board");

  for (const cardName of BENTO_CARD_NAMES) {
    await expectVisible(
      page.getByRole("article", { name: cardName }),
      `${cardName} bento card`,
    );
  }

  const submitWork = page.getByRole("article", { name: "Submit work" });
  await expectVisible(
    submitWork.getByRole("combobox", { name: "Work type" }),
    "Submit work type control",
  );
  await expectVisible(
    submitWork.getByRole("textbox", { name: "Request name" }),
    "Submit work request name control",
  );
  await expectFocusable(
    submitWork.getByRole("textbox", { name: "Request name" }),
    "Submit work request name control",
  );

  await expectVisible(
    page.getByRole("region", { name: "Work graph viewport" }),
    "Workflow graph viewport",
  );
  await expectVisible(
    page.getByRole("button", { name: "Select Implement workstation" }),
    "Workflow graph workstation selection",
  );
  await expectVisible(
    page.getByRole("img", { name: "Work outcome chart for Session" }),
    "Work outcome chart surface",
  );
  await expectVisible(page.getByText("trace-active-story"), "Trace ID value");
  await expectVisible(page.getByText("Transcript"), "Provider transcript tab");
  await expectVisible(
    page.getByRole("button", { name: "Failed Story" }),
    "Terminal work failed item",
  );

  const addWidgetButton = page
    .getByRole("article", { name: "Add widget" })
    .getByRole("button", { name: /^Add widget:/ });
  await expectVisible(addWidgetButton, "Inline add-widget action");
  await expectFocusable(addWidgetButton, "Inline add-widget action");

  await expectBentoGeometry(page, viewport);
  await expectNoHorizontalOverflow(
    page,
    `Bento card catalog at ${viewport.label}`,
  );
}
