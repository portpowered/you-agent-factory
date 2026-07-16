import { expect, userEvent, within } from "storybook/test";
import { formatDateTime } from "../../../../i18n/formatters";

export async function verifyMixedTranscriptStory(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const selectedSessionSection = await verifyCollapsedSummary(canvas);
  const selectedSessionToggle = await verifyDisclosureInteractions(
    canvas,
    selectedSessionSection,
  );
  try {
    await verifyMixedTranscriptDetails(canvas);
  } finally {
    if (selectedSessionToggle.getAttribute("aria-expanded") === "true") {
      await userEvent.click(selectedSessionToggle);
    }
  }
  expect(selectedSessionToggle).toHaveAttribute("aria-expanded", "false");
  canvasElement.dataset.providerSessionVerificationReady = "true";
}

async function verifyCollapsedSummary(
  canvas: ReturnType<typeof within>,
): Promise<HTMLElement> {
  const selectedSessionHeading = await canvas.findByRole("heading", {
    name: "Selected Session Details",
  });
  const selectedSessionSection = selectedSessionHeading.closest("section");
  if (!selectedSessionSection) {
    throw new Error("Selected-session disclosure section was not rendered.");
  }
  expect(
    [...selectedSessionSection.querySelectorAll("dt")].map(
      (term) => term.textContent,
    ),
  ).toEqual([
    "Session ID",
    "Input Tokens",
    "Output Tokens",
    "Cached Tokens",
    "Source File",
  ]);
  expect(
    canvas.queryByRole("heading", { name: "Source File" }),
  ).not.toBeInTheDocument();
  expect(
    canvas.queryByRole("heading", { name: "Session Analysis" }),
  ).not.toBeInTheDocument();
  return selectedSessionSection;
}

async function verifyDisclosureInteractions(
  canvas: ReturnType<typeof within>,
  selectedSessionSection: HTMLElement,
) {
  const selectedSessionToggle = within(selectedSessionSection).getByRole(
    "button",
    { name: "Expand Selected Session Details" },
  );
  expect(selectedSessionToggle).toHaveAttribute("aria-expanded", "false");
  await userEvent.click(selectedSessionToggle);
  expect(selectedSessionToggle).toHaveAttribute("aria-expanded", "true");

  const sessionAnalysisHeading = canvas.getByRole("heading", {
    name: "Session Analysis",
  });
  const transcriptHeading = canvas.getByRole("heading", {
    name: "Transcript",
  });
  expect(
    sessionAnalysisHeading.compareDocumentPosition(transcriptHeading) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();

  const transcriptToggle = canvas.getByRole("button", {
    name: "Collapse Transcript",
  });
  const userMessageToggle = canvas.getByRole("button", {
    name: "Collapse User",
  });
  expect(transcriptToggle).toHaveAttribute("aria-expanded", "true");
  expect(userMessageToggle).toHaveAttribute("aria-expanded", "true");
  expect(
    canvas.getByText("Summarize the rollout state for this work item."),
  ).toBeVisible();

  await toggleAndVerify(userMessageToggle);
  await toggleAndVerify(transcriptToggle);
  return selectedSessionToggle;
}

async function toggleAndVerify(toggle: HTMLElement) {
  await userEvent.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  await userEvent.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "true");
}

async function verifyMixedTranscriptDetails(canvas: ReturnType<typeof within>) {
  expect(
    canvas.getAllByText('{"path":"pkg/api/provider_session_details.go"}'),
  ).toHaveLength(1);
  expect(
    canvas.getAllByText("Inspect the parser branch before retrying."),
  ).toHaveLength(1);
  expect(canvas.getAllByText("Encrypted Reasoning").length).toBeGreaterThan(0);
  expect(canvas.getByLabelText("Selected Session Details").className).toContain(
    "af-provider-session-sans",
  );
  for (const text of [
    "Command Result",
    "Exit Code",
    "0.6289 seconds",
    "provider-session parsing verified successfully",
    "Reasoning occurred for this step, but plaintext content is intentionally unavailable.",
  ]) {
    expect(canvas.getByText(text)).toBeTruthy();
  }
  const diagnosticsToggle = canvas.getByRole("button", {
    name: "Expand Maintainer Diagnostics",
  });
  await userEvent.click(diagnosticsToggle);
  expect(canvas.getByText("unexpected end of JSON input")).toBeTruthy();
  expect(canvas.getByText("Unknown event on line 8")).toBeTruthy();
  await userEvent.click(diagnosticsToggle);
  const expectedTimestamp = formatDateTime("2026-05-20T17:35:27Z");
  expect(
    canvas
      .getAllByTitle("2026-05-20T17:35:27Z")
      .some(
        (element: HTMLElement) => element.textContent === expectedTimestamp,
      ),
  ).toBe(true);
}
