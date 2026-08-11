import { chromium } from "playwright";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const storyID =
  "you-agent-factory-submit-work-story-0-regression-fixture--session-scoped-delayed-submission";
const viewports = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

async function selectWorkType(page, card) {
  await card.getByRole("combobox", { name: /Work type/ }).click();
  await page.getByRole("option", { name: "story" }).click();
}

async function verifyViewport(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);

  try {
    await page.goto(
      new URL(
        `/iframe.html?id=${storyID}&viewMode=story`,
        storybookURL,
      ).toString(),
      { timeout: 30_000, waitUntil: "domcontentloaded" },
    );
    const card = page.getByRole("article", { name: "Submit work" });
    await card.waitFor({ state: "visible" });

    await selectWorkType(page, card);
    await card
      .getByRole("textbox", { name: /Request name/ })
      .fill("Session A request");
    await card
      .getByRole("textbox", { name: "Text item 1" })
      .fill("Keep this request with session A.");
    await card.getByRole("button", { name: "Submit work" }).click();
    await page.getByRole("button", { name: "Submitting..." }).waitFor({
      state: "visible",
    });

    await page.getByRole("button", { name: "Select session B" }).click();
    await page
      .getByRole("button", { name: "Select session B" })
      .waitFor({ state: "visible" });
    await card
      .getByRole("textbox", { name: /Request name/ })
      .waitFor({ state: "visible" });
    const bRequestName = card.getByRole("textbox", {
      name: /Request name/,
    });
    const bRequestText = card.getByRole("textbox", { name: "Text item 1" });
    const bRequestNameValue = await bRequestName.inputValue();
    const bRequestTextValue = await bRequestText.inputValue();
    if (bRequestNameValue !== "" || bRequestTextValue !== "") {
      throw new Error(
        `Session B inherited session A's draft at ${viewport.label}: name=${JSON.stringify(bRequestNameValue)} text=${JSON.stringify(bRequestTextValue)}.`,
      );
    }
    if (
      !(
        await card.locator("[data-submit-work-destination]").textContent()
      ).includes("019e0000-0000-7000-8000-000000000043")
    ) {
      throw new Error("Session B did not expose its canonical destination.");
    }

    await selectWorkType(page, card);
    await bRequestName.fill("Session B request");
    await bRequestText.fill("Keep this request with session B.");
    await card.getByRole("button", { name: "Submit work" }).click();
    await page.getByRole("button", { name: "Submitting..." }).waitFor({
      state: "visible",
    });

    await page
      .getByRole("button", { name: "Complete session A success" })
      .click();
    if (
      (await page
        .getByText("Your request was submitted. Trace ID: trace-session-a.")
        .count()) > 0
    ) {
      throw new Error("Session A's late success was shown in session B.");
    }

    await page
      .getByRole("button", { name: "Complete session B failure" })
      .click();
    const bError = page.getByRole("alert");
    await bError.waitFor({ state: "visible" });
    if (
      !(await bError.textContent()).includes("Session B submission failed.")
    ) {
      throw new Error("Session B did not retain its own submission failure.");
    }

    const metrics = await card.evaluate((element) => {
      const status = element.querySelector("[data-submit-work-status]");
      const form = element.querySelector("form");
      const cardRect = element.getBoundingClientRect();
      const statusRect = status?.getBoundingClientRect();
      const formRect = form?.getBoundingClientRect();
      return {
        pageOverflows: document.documentElement.scrollWidth > window.innerWidth,
        statusContained:
          statusRect !== undefined &&
          statusRect.left >= cardRect.left - 1 &&
          statusRect.right <= cardRect.right + 1,
        statusMatchesFormWidth:
          statusRect !== undefined &&
          formRect !== undefined &&
          statusRect.width >= formRect.width - 2,
        statusWidth: statusRect?.width ?? 0,
      };
    });
    if (
      metrics.pageOverflows ||
      !metrics.statusContained ||
      !metrics.statusMatchesFormWidth ||
      metrics.statusWidth <= 0
    ) {
      throw new Error(
        `Submit Work feedback was not full-width and contained at ${viewport.label}.`,
      );
    }

    await page.getByRole("button", { name: "Select session A" }).click();
    await page
      .getByText("Your request was submitted. Trace ID: trace-session-a.")
      .waitFor({ state: "visible" });
    console.log(`Verified Submit Work session scoping at ${viewport.label}.`);
  } finally {
    await context.close();
  }
}

const browser = await chromium.launch({ headless: true });
try {
  for (const viewport of viewports) {
    await verifyViewport(browser, viewport);
  }
  console.log("Submit Work session scoping browser verification passed.");
} finally {
  await browser.close();
}
