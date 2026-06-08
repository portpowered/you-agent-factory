import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { getSubmitWorkMessages } from "../messages/submit-work";
import { SubmitWorkCard } from "./submit-work-card";

const messages = getSubmitWorkMessages("en");
const defaultDraft = {
  items: [{ id: "submission-item-1", text: "", type: "text" as const }],
  requestName: "",
  workTypeName: "story",
};

function renderSubmitWorkCard(
  overrides: Partial<ComponentProps<typeof SubmitWorkCard>> = {},
) {
  const onRequestNameChange = vi.fn();

  const view = render(
    <SubmitWorkCard
      draft={defaultDraft}
      onAddItem={() => {}}
      onItemTextChange={() => {}}
      onRemoveItem={() => {}}
      onRequestNameChange={onRequestNameChange}
      onStageFileItems={() => {}}
      onSubmit={() => {}}
      onWorkTypeNameChange={() => {}}
      status={{
        kind: "guidance",
        message: messages.statusMessages.ready,
      }}
      submitWorkTypeNames={["story", "task"]}
      {...overrides}
    />,
  );

  return { onRequestNameChange, ...view };
}

describe("SubmitWorkCard request name field", () => {
  afterEach(() => {
    cleanup();
  });

  it("communicates that the request name is required before submission", () => {
    renderSubmitWorkCard();

    const card = screen.getByRole("article", { name: messages.cardTitle });
    expect(within(card).getByText("(required)")).toBeInTheDocument();
    expect(
      within(card).getByRole("textbox", {
        name: `${messages.requestNameLabel} (${messages.requestNameRequiredAffordance})`,
      }),
    ).toHaveAttribute("aria-required", "true");
  });

  it("marks the request name invalid with accessible field feedback on submit", () => {
    renderSubmitWorkCard({
      validationErrors: {
        requestName: messages.validationMessages.requestRequired,
      },
    });

    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: `${messages.requestNameLabel} (${messages.requestNameRequiredAffordance})`,
    });
    const error = screen.getByText(messages.validationMessages.requestRequired);

    expect(requestName).toHaveAttribute("aria-invalid", "true");
    expect(requestName).toHaveAttribute("aria-describedby", error.id);
    expect(error).toHaveAttribute("role", "alert");
    expect(
      screen.queryByText(messages.validationMessages.bothMissing),
    ).toBeNull();
  });

  it("clears request name invalid styling when a non-empty value is provided", () => {
    const { onRequestNameChange, rerender } = renderSubmitWorkCard({
      validationErrors: {
        requestName: messages.validationMessages.requestRequired,
      },
    });

    fireEvent.change(
      screen.getByRole("textbox", {
        name: `${messages.requestNameLabel} (${messages.requestNameRequiredAffordance})`,
      }),
      { target: { value: "Driver review" } },
    );
    expect(onRequestNameChange).toHaveBeenCalledWith("Driver review");

    rerender(
      <SubmitWorkCard
        draft={{ ...defaultDraft, requestName: "Driver review" }}
        onAddItem={() => {}}
        onItemTextChange={() => {}}
        onRemoveItem={() => {}}
        onRequestNameChange={onRequestNameChange}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={() => {}}
        status={{
          kind: "guidance",
          message: messages.statusMessages.ready,
        }}
        submitWorkTypeNames={["story", "task"]}
      />,
    );

    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: `${messages.requestNameLabel} (${messages.requestNameRequiredAffordance})`,
    });

    expect(requestName).not.toHaveAttribute("aria-invalid");
    expect(requestName).not.toHaveAttribute("aria-describedby");
    expect(
      screen.queryByText(messages.validationMessages.requestRequired),
    ).toBeNull();
  });
});
