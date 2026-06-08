import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { selectComboboxOption } from "../../../testing/select-test-helpers";
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

describe("SubmitWorkCard work type selector", () => {
  let restoreBrowserShims: (() => void) | undefined;
  let user: ReturnType<typeof userEvent.setup>;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
    user = userEvent.setup();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("communicates that the work type is required before submission", () => {
    renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        workTypeName: "",
      },
    });

    expect(
      screen.getByRole("combobox", {
        name: `${messages.workTypeLabel} (${messages.workTypeRequiredAffordance})`,
      }),
    ).toHaveAttribute("aria-required", "true");
  });

  it("marks the work type selector invalid with accessible field feedback on submit", () => {
    renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        requestName: "Driver review",
        workTypeName: "",
      },
      validationErrors: {
        workTypeName: messages.validationMessages.workTypeRequired,
      },
    });

    const workType = screen.getByRole("combobox", {
      name: `${messages.workTypeLabel} (${messages.workTypeRequiredAffordance})`,
    });
    const error = screen.getByText(messages.validationMessages.workTypeRequired);

    expect(workType).toHaveAttribute("aria-invalid", "true");
    expect(workType).toHaveAttribute("aria-describedby", error.id);
    expect(error).toHaveAttribute("role", "alert");
    expect(
      screen.queryByText(messages.validationMessages.bothMissing),
    ).toBeNull();
  });

  it("opens the work type selector with keyboard interaction", async () => {
    const onWorkTypeNameChange = vi.fn();

    renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        workTypeName: "",
      },
      onWorkTypeNameChange,
    });

    const workType = screen.getByRole("combobox", {
      name: `${messages.workTypeLabel} (${messages.workTypeRequiredAffordance})`,
    });

    workType.focus();
    await user.keyboard("{Enter}");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowDown}{Enter}");

    expect(onWorkTypeNameChange).toHaveBeenCalledWith("story");
  });

  it("clears work type invalid styling when a valid option is selected", async () => {
    const onWorkTypeNameChange = vi.fn();
    const { rerender } = renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        requestName: "Driver review",
        workTypeName: "",
      },
      onWorkTypeNameChange,
      validationErrors: {
        workTypeName: messages.validationMessages.workTypeRequired,
      },
    });

    const workType = screen.getByRole("combobox", {
      name: `${messages.workTypeLabel} (${messages.workTypeRequiredAffordance})`,
    });

    await selectComboboxOption(user, workType, "story");
    expect(onWorkTypeNameChange).toHaveBeenCalledWith("story");

    rerender(
      <SubmitWorkCard
        draft={{
          ...defaultDraft,
          requestName: "Driver review",
          workTypeName: "story",
        }}
        onAddItem={() => {}}
        onItemTextChange={() => {}}
        onRemoveItem={() => {}}
        onRequestNameChange={() => {}}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={onWorkTypeNameChange}
        status={{
          kind: "guidance",
          message: messages.statusMessages.ready,
        }}
        submitWorkTypeNames={["story", "task"]}
      />,
    );

    const correctedWorkType = screen.getByRole("combobox", {
      name: `${messages.workTypeLabel} (${messages.workTypeRequiredAffordance})`,
    });

    expect(correctedWorkType).not.toHaveAttribute("aria-invalid");
    expect(correctedWorkType).not.toHaveAttribute("aria-describedby");
    expect(
      screen.queryByText(messages.validationMessages.workTypeRequired),
    ).toBeNull();
  });
});
