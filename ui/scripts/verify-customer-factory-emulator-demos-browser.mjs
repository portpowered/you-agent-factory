import assert from "node:assert/strict";

import { chromium } from "playwright";

import {
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const storybookUrl =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const interactiveStory = "agent-factory-emulator-customer-demos--interactive";
const errorStory =
  "agent-factory-emulator-customer-demos--setup-error-isolation";
const lifecycleStory =
  "agent-factory-emulator-customer-demos--lifecycle-isolation";
const viewports = [
  { height: 844, width: 390 },
  { height: 900, width: 1440 },
];

async function installPlaybackControls(context) {
  await context.addInitScript(() => {
    const observers = [];
    let reducedMotion = false;
    const motionListeners = new Set();
    window.matchMedia = (query) => ({
      addEventListener: (_type, listener) => motionListeners.add(listener),
      addListener: (listener) => motionListeners.add(listener),
      dispatchEvent: () => true,
      get matches() {
        return query === "(prefers-reduced-motion: reduce)" && reducedMotion;
      },
      media: query,
      onchange: null,
      removeEventListener: (_type, listener) =>
        motionListeners.delete(listener),
      removeListener: (listener) => motionListeners.delete(listener),
    });
    window.IntersectionObserver = class {
      constructor(callback) {
        this.callback = callback;
        this.targets = new Set();
        observers.push(this);
      }
      disconnect() {
        this.targets.clear();
      }
      observe(target) {
        this.targets.add(target);
      }
      unobserve(target) {
        this.targets.delete(target);
      }
    };
    window.__setCustomerDemoVisibility = (demoID, isIntersecting) => {
      for (const observer of observers) {
        for (const target of observer.targets) {
          if (target.dataset.demoId === demoID) {
            observer.callback([{ isIntersecting, target }], observer);
          }
        }
      }
    };
    window.__setCustomerDemoReducedMotion = (matches) => {
      reducedMotion = matches;
      for (const listener of motionListeners) listener({ matches });
    };
    window.__customerDemoHostMetrics = () => ({
      motionListeners: motionListeners.size,
      observedTargets: observers.reduce(
        (count, observer) => count + observer.targets.size,
        0,
      ),
    });
  });
}

async function setVisibility(page, demoID, isIntersecting) {
  await page.evaluate(
    ({ id, visible }) => window.__setCustomerDemoVisibility(id, visible),
    { id: demoID, visible: isIntersecting },
  );
}

async function setReducedMotion(page, matches) {
  await page.evaluate(
    (reduced) => window.__setCustomerDemoReducedMotion(reduced),
    matches,
  );
}

async function openStory(page, storyID) {
  await page.goto(storyUrl(storybookUrl, storyID), {
    timeout: 30_000,
    waitUntil: "domcontentloaded",
  });
  await waitForStoryRender(page);
}

async function assertResponsiveLayout(page, viewport, storyID) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  );
  assert.equal(
    overflow,
    false,
    `${storyID} has page-level overflow at ${viewport.width}px.`,
  );
  for (const control of await page.getByRole("button").all()) {
    const box = await control.boundingBox();
    if (!box) continue;
    assert.ok(
      box.x >= -1 && box.x + box.width <= viewport.width + 1,
      `${storyID} has a clipped control at ${viewport.width}px.`,
    );
  }
  for (const demo of await page.getByRole("article").all()) {
    const controls = demo.locator(
      'section[aria-label="Factory emulator controls"] button, section[aria-label="Factory emulator controls"] input, section[aria-label="Factory emulator controls"] select',
    );
    const boxes = (
      await Promise.all(
        (await controls.all()).map((control) => control.boundingBox()),
      )
    ).filter(Boolean);
    for (let first = 0; first < boxes.length; first += 1) {
      for (let second = first + 1; second < boxes.length; second += 1) {
        const horizontalIntersection =
          Math.min(
            boxes[first].x + boxes[first].width,
            boxes[second].x + boxes[second].width,
          ) - Math.max(boxes[first].x, boxes[second].x);
        const verticalIntersection =
          Math.min(
            boxes[first].y + boxes[first].height,
            boxes[second].y + boxes[second].height,
          ) - Math.max(boxes[first].y, boxes[second].y);
        assert.ok(
          horizontalIntersection <= 1 || verticalIntersection <= 1,
          `${storyID} has overlapping controls at ${viewport.width}px.`,
        );
      }
    }
  }
}

async function step(demo) {
  const button = demo.getByRole("button", { name: "Step" });
  await button.focus();
  await button.press("Enter");
}

async function verifyLiveSubmission(success, failure) {
  await step(success);
  await success
    .getByText(
      "Execute: Preparing the launch summary (1.5 seconds virtual time)",
    )
    .waitFor();
  const submission = success.getByRole("textbox", { name: "Submit text" });
  await submission.fill("Customer request");
  await submission.press("Shift+Enter");
  await submission.type("Second line");
  assert.equal(
    await submission.inputValue(),
    "Customer request\nSecond line",
    "Shift+Enter did not preserve a multiline customer-demo draft.",
  );
  await submission.press("Enter");
  await success.getByText("2 Work total").waitFor();
  assert.equal(
    await submission.inputValue(),
    "",
    "An accepted customer-demo submission did not clear its draft.",
  );
  assert.equal(
    await submission.evaluate((element) => element === document.activeElement),
    true,
    "An accepted customer-demo submission moved focus unexpectedly.",
  );
  await failure.getByText("1 Work total").waitFor();
  await success.getByRole("button", { name: "Restart" }).click();
  await success.getByText("1 Work total").waitFor();
  await step(success);
  return submission;
}

async function verifyInteractive(page, viewport) {
  await openStory(page, interactiveStory);
  const success = page.getByRole("article", {
    name: "Straightforward success",
  });
  const failure = page.getByRole("article", {
    name: "Review, rework, and failure",
  });
  await success.getByText("1 Work total").waitFor();
  await failure.getByText("1 Work total").waitFor();
  await assertResponsiveLayout(page, viewport, interactiveStory);

  const successSubmission = await verifyLiveSubmission(success, failure);
  const successSlider = success.getByRole("slider", {
    name: "Select replay tick",
  });
  await successSlider.fill("0");
  await success.getByText("Viewing history", { exact: true }).waitFor();
  assert.equal(
    await successSubmission.isDisabled(),
    true,
    "Historical customer-demo inspection did not disable submission.",
  );
  await success
    .getByText("Return to the current tick before submitting.")
    .waitFor();
  await setVisibility(page, "success", true);
  await success.getByRole("button", { name: "Play" }).click();
  await success.getByText("Playing", { exact: true }).waitFor();
  assert.equal(
    await successSubmission.isEnabled(),
    true,
    "Returning to the live customer-demo head did not restore submission.",
  );
  await success.getByRole("button", { name: "Pause" }).click();
  await successSlider.fill("0");
  await success.getByRole("button", { name: "Step" }).focus();
  await page.keyboard.press("Enter");
  await success
    .getByRole("region", { name: "Successful completion" })
    .waitFor();
  await success
    .getByText("Restart the completed emulator to submit more Work.")
    .waitFor();
  assert.equal(
    await successSubmission.isDisabled(),
    true,
    "A closed customer demo still allowed text submission.",
  );
  await success
    .getByRole("combobox", { name: "Playback speed" })
    .selectOption("2");
  assert.equal(
    await success
      .getByRole("combobox", { name: "Playback speed" })
      .inputValue(),
    "2",
    "Playback speed did not expose its controlled value.",
  );
  await successSlider.focus();
  await page.keyboard.press("ArrowLeft");
  await success.getByText("Viewing history", { exact: true }).waitFor();
  await success.getByRole("button", { name: "Follow current" }).click();
  await success
    .getByRole("region", { name: "Successful completion" })
    .waitFor();
  await failure
    .getByRole("status", { name: "Runtime status" })
    .getByText("Ready", { exact: true })
    .waitFor();

  for (let index = 0; index < 6; index += 1) await step(failure);
  await step(failure);
  await failure
    .getByText(
      "Execute: Polishing the revised launch plan (1.5 seconds virtual time)",
    )
    .waitFor();
  for (let index = 0; index < 3; index += 1) await step(failure);
  await failure.getByRole("region", { name: "Terminal failure" }).waitFor();
  await failure.getByText("1 failed").waitFor();

  const slider = failure.getByRole("slider", { name: "Select replay tick" });
  await slider.focus();
  await page.keyboard.press("ArrowLeft");
  await failure
    .getByText("Review: Working at Review (1 seconds virtual time)")
    .waitFor();
  assert.equal(
    await failure.getByText(/Running final review/).count(),
    0,
    "Historical replay retained transient activity copy.",
  );
  await failure.getByRole("button", { name: "Follow current" }).click();
  await failure.getByRole("region", { name: "Terminal failure" }).waitFor();
  await assertResponsiveLayout(page, viewport, interactiveStory);
}

async function verifySetupErrorIsolation(page, viewport) {
  await openStory(page, errorStory);
  await page.getByText("1 Work total").waitFor();
  await page
    .getByRole("alert")
    .getByText(/This demo could not be prepared/)
    .waitFor();
  await page
    .getByRole("article", { name: "Straightforward success" })
    .getByRole("button", { name: "Step" })
    .waitFor();
  await assertResponsiveLayout(page, viewport, errorStory);
}

async function verifyLifecycleIsolation(page, viewport) {
  await openStory(page, lifecycleStory);
  await page.clock.install();
  const success = page.getByRole("article", {
    name: "Straightforward success",
  });
  let failure = page.getByRole("article", {
    name: "Review, rework, and failure",
  });
  await success.getByText("1 Work total").waitFor();
  await failure.getByText("1 Work total").waitFor();
  const mountedHostMetrics = await page.evaluate(() =>
    window.__customerDemoHostMetrics(),
  );
  assert.equal(mountedHostMetrics.observedTargets, 2);
  assert.ok(mountedHostMetrics.motionListeners > 0);

  await setVisibility(page, "success", true);
  await setVisibility(page, "repeat-review-failure", true);
  await success.getByText("Playing", { exact: true }).waitFor();
  await failure.getByText("Playing", { exact: true }).waitFor();
  await success.getByRole("button", { name: "Pause" }).click();
  await success
    .getByRole("combobox", { name: "Playback speed" })
    .selectOption("4");
  const isolatedSubmission = success.getByRole("textbox", {
    name: "Submit text",
  });
  await isolatedSubmission.fill("Isolated request");
  await isolatedSubmission.press("Enter");
  await success.getByText("2 Work total").waitFor();
  await failure.getByText("1 Work total").waitFor();
  await success.getByRole("slider", { name: "Select replay tick" }).fill("0");
  await success.getByText("Viewing history", { exact: true }).waitFor();
  await failure.getByText("Playing", { exact: true }).waitFor();
  await success.getByRole("button", { name: "Restart" }).click();
  await success.getByText("1 Work total").waitFor();
  await failure.getByText("Playing", { exact: true }).waitFor();

  await page.getByRole("button", { name: "Unmount failure demo" }).click();
  await failure.waitFor({ state: "detached" });
  const unmountedHostMetrics = await page.evaluate(() =>
    window.__customerDemoHostMetrics(),
  );
  assert.equal(unmountedHostMetrics.observedTargets, 1);
  assert.ok(
    unmountedHostMetrics.motionListeners < mountedHostMetrics.motionListeners,
    "Unmounting one demo did not release its motion-preference listener.",
  );
  assert.equal(
    await success
      .getByRole("combobox", { name: "Playback speed" })
      .inputValue(),
    "1",
    "Restart did not reset the still-mounted success demo independently.",
  );

  await page.getByRole("button", { name: "Remount failure demo" }).click();
  failure = page.getByRole("article", {
    name: "Review, rework, and failure",
  });
  await failure.getByText("1 Work total").waitFor();
  await failure.getByText("Ready", { exact: true }).waitFor();
  assert.equal(
    await failure
      .getByRole("slider", { name: "Select replay tick" })
      .inputValue(),
    "0",
    "Remounted failure demo retained its previous execution state.",
  );
  assert.deepEqual(
    await page.evaluate(() => window.__customerDemoHostMetrics()),
    mountedHostMetrics,
  );
  await assertResponsiveLayout(page, viewport, lifecycleStory);
}

async function verifyMotionSafePlayback(page) {
  await openStory(page, interactiveStory);
  await page.clock.install();
  const success = page.getByRole("article", {
    name: "Straightforward success",
  });
  await success.getByText("1 Work total").waitFor();

  await setVisibility(page, "success", true);
  await success.getByText("Playing", { exact: true }).waitFor();
  await setVisibility(page, "success", false);
  await success.getByText("Ready", { exact: true }).waitFor();
  await setVisibility(page, "success", true);
  await success.getByText("Playing", { exact: true }).waitFor();

  await success.getByRole("button", { name: "Pause" }).click();
  await setVisibility(page, "success", false);
  await setVisibility(page, "success", true);
  await success.getByText("Ready", { exact: true }).waitFor();

  await success.getByRole("button", { name: "Play" }).click();
  await setReducedMotion(page, true);
  await success.getByText("Ready", { exact: true }).waitFor();
  await setReducedMotion(page, false);
  await setVisibility(page, "success", false);
  await setVisibility(page, "success", true);
  await success.getByText("Ready", { exact: true }).waitFor();

  await success.getByRole("button", { name: "Play" }).click();
  await page.clock.fastForward(2_000);
  await success
    .getByRole("region", { name: "Successful completion" })
    .waitFor();
  const completedTick = await success
    .getByRole("slider", { name: "Select replay tick" })
    .getAttribute("max");
  await page.clock.fastForward(10_000);
  assert.equal(
    await success
      .getByRole("slider", { name: "Select replay tick" })
      .getAttribute("max"),
    completedTick,
    "Completed autoplay looped or scheduled more work.",
  );

  await success.getByRole("button", { name: "Restart" }).click();
  await success.getByText("1 Work total").waitFor();
  assert.equal(
    await success
      .getByRole("region", { name: "Successful completion" })
      .count(),
    0,
    "Restart retained the terminal projection.",
  );
  await success.getByText("Playing", { exact: true }).waitFor();
}

async function verifyCustomerFactoryEmulatorDemos() {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const viewport of viewports) {
      const context = await browser.newContext({ viewport });
      await installPlaybackControls(context);
      const page = await context.newPage();
      await verifyInteractive(page, viewport);
      await verifySetupErrorIsolation(page, viewport);
      await verifyLifecycleIsolation(page, viewport);
      await verifyMotionSafePlayback(page);
      await context.close();
    }
  } finally {
    await browser.close();
  }
}

await verifyCustomerFactoryEmulatorDemos();
console.log("Customer Factory emulator demo browser checks passed.");
