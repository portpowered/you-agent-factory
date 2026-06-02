export async function verifyProgressOutcomeRoutesWithoutStopWords(
  { expectVisible },
  page,
  viewport,
) {
  const successHandle = page.getByRole("button", {
    name: "Connect tool: draft Success",
  });
  const failureHandle = page.getByRole("button", {
    name: "Connect tool: draft Failure",
  });
  const continueHandle = page.getByRole("button", {
    name: "Connect tool: draft Continue",
  });
  const rejectHandle = page.getByRole("button", {
    name: "Connect tool: draft Reject",
  });

  await expectVisible(successHandle, "Draft success handle");
  await expectVisible(failureHandle, "Draft failure handle");

  if (await continueHandle.count()) {
    throw new Error(
      `Continue handle should stay hidden without stopWords at ${viewport.label}.`,
    );
  }
  if (await rejectHandle.count()) {
    throw new Error(
      `Reject handle should stay hidden without stopWords at ${viewport.label}.`,
    );
  }

  const hintCount = await page.locator("[data-z-axis-incomplete-hint]").count();
  if (hintCount !== 0) {
    throw new Error(
      `Expected no z-axis incomplete hints without stopWords at ${viewport.label}, found ${hintCount}.`,
    );
  }
}

export async function verifyProgressOutcomeRoutesWithStopWords(
  { expectVisible },
  page,
  _viewport,
) {
  const successHandle = page.getByRole("button", {
    name: "Connect tool: draft Success",
  });
  const failureHandle = page.getByRole("button", {
    name: "Connect tool: draft Failure",
  });
  const continueHandle = page.getByRole("button", {
    name: "Connect tool: draft Continue",
  });
  const rejectHandle = page.getByRole("button", {
    name: "Connect tool: draft Reject",
  });

  await expectVisible(successHandle, "Draft success handle");
  await expectVisible(failureHandle, "Draft failure handle");
  await expectVisible(continueHandle, "Draft continue handle");
  await expectVisible(rejectHandle, "Draft reject handle");

  const hintCount = await page.locator("[data-z-axis-incomplete-hint]").count();
  if (hintCount !== 0) {
    throw new Error(
      `Z-axis incomplete hints should be absent with stopWords configured, found ${hintCount}.`,
    );
  }
}
