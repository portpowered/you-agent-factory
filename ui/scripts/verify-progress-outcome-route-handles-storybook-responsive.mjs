export async function verifyProgressOutcomeRoutesWithoutStopWords(
  { expectVisible },
  page,
  viewport,
) {
  const successHandle = page.getByRole("button", {
    name: "Connect: draft Success",
  });
  const failureHandle = page.getByRole("button", {
    name: "Connect: draft Failure",
  });
  const continueHandle = page.getByRole("button", {
    name: "Connect: draft Continue",
  });
  const rejectHandle = page.getByRole("button", {
    name: "Connect: draft Reject",
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
}

export async function verifyProgressOutcomeRoutesWithStopWords(
  { expectVisible },
  page,
  _viewport,
) {
  const successHandle = page.getByRole("button", {
    name: "Connect: draft Success",
  });
  const failureHandle = page.getByRole("button", {
    name: "Connect: draft Failure",
  });
  const continueHandle = page.getByRole("button", {
    name: "Connect: draft Continue",
  });
  const rejectHandle = page.getByRole("button", {
    name: "Connect: draft Reject",
  });

  await expectVisible(successHandle, "Draft success handle");
  await expectVisible(failureHandle, "Draft failure handle");
  await expectVisible(continueHandle, "Draft continue handle");
  await expectVisible(rejectHandle, "Draft reject handle");
}
