const denseTopologyModes = [
  [
    "factory-visualizers-factorytopologyreplay--dense-prepared-projection",
    true,
  ],
  ["factory-visualizers-factorytopologyreplay--dense-minimal-chrome", true],
  ["factory-visualizers-factorytopologyreplay--dense-no-chrome", false],
];

export async function verifyDenseTopologyChromeModes({
  assert,
  page,
  tabTo,
  verifyLayout,
  width,
}) {
  for (const [storyId, controls] of denseTopologyModes) {
    await verifyLayout(page, storyId);
    const activeWorkRows = page.getByRole("group", {
      name: "5 active Work rows",
    });
    await activeWorkRows.waitFor({ state: "visible", timeout: 5_000 });
    assert(
      await activeWorkRows.isVisible(),
      `${storyId} hides dense active Work rows at ${width}px.`,
    );
    assert(
      await page.getByText("+2 active Work").isVisible(),
      `${storyId} hides dense active Work overflow at ${width}px.`,
    );
    const workstation = page.getByRole("button", {
      name: "workstation: Review",
    });
    await tabTo(page, workstation, 50);
    assert(
      (await workstation.evaluate(
        (element) => getComputedStyle(element).outlineWidth,
      )) !== "0px",
      `${storyId} hides topology-node focus at ${width}px.`,
    );
    if (!controls) {
      assert(
        (await page.locator(".react-flow__controls").count()) === 0,
        `${storyId} mounts disabled viewport controls at ${width}px.`,
      );
      continue;
    }
    const zoomIn = page.getByRole("button", { name: "Zoom in" });
    await tabTo(page, zoomIn, 50);
    assert(
      (await zoomIn.evaluate(
        (element) => getComputedStyle(element).outlineStyle,
      )) !== "none",
      `${storyId} hides graph-control focus at ${width}px.`,
    );
    const beforeZoom = await page
      .locator(".react-flow__viewport")
      .getAttribute("style");
    await zoomIn.click();
    assert(
      (await page.locator(".react-flow__viewport").getAttribute("style")) !==
        beforeZoom,
      `${storyId} viewport controls do not zoom at ${width}px.`,
    );
  }
}
