export async function verifyResponsiveStateAndAnnotationMatrix(
  browserInstance,
  { assert, assertNoPageOverflow, openStory, tabTo },
) {
  const viewports = [
    { width: 360, height: 800 },
    { width: 720, height: 900 },
    { width: 1200, height: 900 },
  ];
  const stateStories = [
    "factory-visualizers-factoryrecordingtopologyreplay--loading",
    "factory-visualizers-factoryrecordingtopologyreplay--empty-recording",
    "factory-visualizers-factoryrecordingtopologyreplay--validated-recording",
    "factory-visualizers-factoryrecordingtopologyreplay--terminal-recording",
    "factory-visualizers-factoryrecordingtopologyreplay--projection-failure",
    "factory-visualizers-factoryemulatorview--full",
    "factory-visualizers-factoryemulatorview--loading-initial",
    "factory-visualizers-factoryemulatorview--empty",
    "factory-visualizers-factoryemulatorview--terminal",
    "factory-visualizers-factoryemulatorview--host-failure",
  ];

  for (const viewport of viewports) {
    const context = await browserInstance.newContext({ viewport });
    const page = await context.newPage();
    for (const storyId of stateStories) {
      await openStory(page, storyId);
      await assertNoPageOverflow(page, storyId);
    }
    await verifyResponsiveAnnotations(page, viewport.width, {
      assert,
      assertNoPageOverflow,
      openStory,
      tabTo,
    });
    await verifyRuntimeTelemetryPrecedence(page, viewport.width, {
      assert,
      assertNoPageOverflow,
      openStory,
    });
    await verifyDependencyNeutralRecording(page, viewport.width, {
      assert,
      assertNoPageOverflow,
      openStory,
    });
    await context.close();
  }
}

async function verifyResponsiveAnnotations(
  page,
  viewportWidth,
  { assert, assertNoPageOverflow, openStory, tabTo },
) {
  const storyId =
    "factory-visualizers-factorytopologyreplay--responsive-annotations-and-empty-state";
  await openStory(page, storyId);
  await assertNoPageOverflow(page, storyId);
  await page
    .getByText("Long review guidance wraps inside the topology.")
    .waitFor({ state: "visible", timeout: 5_000 });
  await page
    .getByRole("img", { name: "Review flow overview" })
    .waitFor({ state: "visible", timeout: 5_000 });
  await page
    .getByRole("img", { name: "Worker availability illustration" })
    .waitFor({ state: "visible", timeout: 5_000 });
  const topology = page.getByRole("region", {
    name: "Factory topology at selected tick",
  });
  assert(
    (await topology.evaluate(
      (element) => getComputedStyle(element).overflow,
    )) === "hidden",
    `The responsive topology does not contain its canvas at ${viewportWidth}px.`,
  );
  for (const annotation of await page
    .locator(".factory-topology-replay__annotation")
    .all()) {
    assert(
      await annotation.evaluate(
        (element) =>
          element.closest(".react-flow") !== null &&
          element.clientHeight > 0 &&
          element.clientWidth > 0 &&
          element.scrollWidth <= element.clientWidth + 1,
      ),
      `An annotation is unreadable or outside the contained canvas at ${viewportWidth}px.`,
    );
  }
  const workstation = page.getByRole("button", {
    name: "workstation: Review",
  });
  await tabTo(page, workstation, 100);
  assert(
    await workstation.getByText("No review work is waiting.").isVisible(),
    `The node empty state is not keyboard-reachable at ${viewportWidth}px.`,
  );
}

async function verifyRuntimeTelemetryPrecedence(
  page,
  viewportWidth,
  { assert, assertNoPageOverflow, openStory },
) {
  const storyId =
    "factory-visualizers-factorytopologyreplay--runtime-telemetry-precedence";
  await openStory(page, storyId);
  await assertNoPageOverflow(page, storyId);
  assert(
    (await page.getByText(/Configured empty content/).count()) === 0,
    `Configured empty content obscured runtime telemetry at ${viewportWidth}px.`,
  );
  assert(
    (await page.getByText("2 of 4 capacity occupied").count()) === 1,
    `Resource telemetry is missing at ${viewportWidth}px.`,
  );
  assert(
    (await page.getByText("7 Work in this state").count()) === 1,
    `Work State telemetry is missing at ${viewportWidth}px.`,
  );
  await page.waitForFunction(
    () => document.querySelectorAll(".react-flow__edge.animated").length > 0,
    { timeout: 5_000 },
  );
}

async function verifyDependencyNeutralRecording(
  page,
  viewportWidth,
  { assert, assertNoPageOverflow, openStory },
) {
  const storyId =
    "factory-visualizers-factoryrecordingtopologyreplay--dependency-neutral-recording";
  await openStory(page, storyId);
  await assertNoPageOverflow(page, storyId);
  await page.getByText("2 Work total").waitFor({ state: "visible" });
  assert(
    (await page.getByText(/depends_on|dependency|blocked/i).count()) === 0,
    `Dependency-specific copy rendered at ${viewportWidth}px.`,
  );
  assert(
    (await page
      .getByRole("button", { name: /relationship|dependency|connect/i })
      .count()) === 0,
    `Dependency-specific controls rendered at ${viewportWidth}px.`,
  );
  await page.waitForFunction(
    () => document.querySelectorAll(".react-flow__edge").length === 2,
    { timeout: 5_000 },
  );
  assert(
    (await page.locator(".react-flow__node").count()) === 3,
    `Dependency evidence reserved topology nodes or layout gaps at ${viewportWidth}px.`,
  );
}
