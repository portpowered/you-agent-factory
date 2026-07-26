// biome-ignore-all lint/style/noExcessiveLinesPerFile: submit-work interaction cases share one form and invocation fixture harness.
import "../../../testing/vitest-dom-capabilities.setup";

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
    const error = screen.getByText(
      messages.validationMessages.workTypeRequired,
    );

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

describe("SubmitWorkCard form-level status", () => {
  afterEach(() => {
    cleanup();
  });

  it("keeps required-field validation on the controls without a detached status panel", () => {
    renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        requestName: "",
        workTypeName: "",
      },
      validationErrors: {
        requestName: messages.validationMessages.requestRequired,
        workTypeName: messages.validationMessages.workTypeRequired,
      },
    });

    expect(
      screen.getByText(messages.validationMessages.requestRequired),
    ).toHaveAttribute("role", "alert");
    expect(
      screen.getByText(messages.validationMessages.workTypeRequired),
    ).toHaveAttribute("role", "alert");
    expect(
      screen.queryByText(messages.validationMessages.bothMissing),
    ).toBeNull();
    expect(
      screen.queryByRole("alert", { name: /before submitting/i }),
    ).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("shows server submission failures through the card status panel", () => {
    renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        requestName: "Driver review",
      },
      status: {
        kind: "error",
        message: "work_type_name is required",
      },
    });

    const statusPanel = screen.getByRole("alert");
    expect(statusPanel).toHaveTextContent("work_type_name is required");
    expect(statusPanel.className).toContain("bg-error-container");
    expect(
      screen.queryByText(messages.validationMessages.requestRequired),
    ).toBeNull();
    expect(
      screen.queryByText(messages.validationMessages.workTypeRequired),
    ).toBeNull();
    expect(
      screen.getByRole("textbox", {
        name: `${messages.requestNameLabel} (${messages.requestNameRequiredAffordance})`,
      }),
    ).not.toHaveAttribute("aria-invalid");
  });
});

describe("SubmitWorkCard submission outcomes", () => {
  afterEach(() => {
    cleanup();
  });

  it("shows successful submissions through the card status panel without stale field invalid styling", () => {
    renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        requestName: "",
      },
      status: {
        kind: "success",
        message: messages.statusMessages.success("trace-submit-story"),
      },
    });

    const statusPanel = screen.getByRole("status");
    expect(statusPanel).toHaveTextContent("trace-submit-story");
    expect(statusPanel.className).toContain("bg-success-container");
    expect(
      screen.getByRole("textbox", {
        name: `${messages.requestNameLabel} (${messages.requestNameRequiredAffordance})`,
      }),
    ).not.toHaveAttribute("aria-invalid");
    expect(
      screen.getByRole("combobox", {
        name: `${messages.workTypeLabel} (${messages.workTypeRequiredAffordance})`,
      }),
    ).not.toHaveAttribute("aria-invalid");
    expect(
      screen.queryByText(messages.validationMessages.requestRequired),
    ).toBeNull();
    expect(
      screen.queryByText(messages.validationMessages.workTypeRequired),
    ).toBeNull();
  });

  it("keeps submission-item validation on the detached status channel", () => {
    renderSubmitWorkCard({
      draft: {
        items: [
          {
            fileName: "ui.png",
            id: "submission-item-1",
            mediaType: "image/png",
            stagedFileRef: "staged://submit-work/ui.png",
            stagingStatus: "idle",
            type: "image",
          },
        ],
        requestName: "Driver review",
        workTypeName: "story",
      },
      status: {
        kind: "validation-error",
        message: messages.validationMessages.fileItemNeedsStaging,
      },
      validationErrors: {
        submissionItems: messages.validationMessages.fileItemNeedsStaging,
      },
    });

    const statusPanel = screen.getByRole("alert");
    expect(statusPanel).toHaveTextContent(
      messages.validationMessages.fileItemNeedsStaging,
    );
    expect(
      screen.getAllByText(messages.validationMessages.fileItemNeedsStaging),
    ).toHaveLength(2);
    expect(
      screen.getByRole("textbox", {
        name: `${messages.requestNameLabel} (${messages.requestNameRequiredAffordance})`,
      }),
    ).not.toHaveAttribute("aria-invalid");
    expect(
      screen.getByRole("combobox", {
        name: `${messages.workTypeLabel} (${messages.workTypeRequiredAffordance})`,
      }),
    ).not.toHaveAttribute("aria-invalid");
  });
});

describe("SubmitWorkCard submission textarea", () => {
  afterEach(() => {
    cleanup();
  });

  it("uses the shared textarea primitive with scroll constraints for long text", () => {
    renderSubmitWorkCard({
      draft: {
        ...defaultDraft,
        items: [
          {
            id: "submission-item-1",
            text: "line\n".repeat(40),
            type: "text",
          },
        ],
      },
    });

    const submissionTextarea = screen.getByRole<HTMLTextAreaElement>(
      "textbox",
      {
        name: messages.requestItemLabel(1),
      },
    );

    expect(submissionTextarea.className).toContain("min-h-28");
    expect(submissionTextarea.className).toContain("max-h-52");
    expect(submissionTextarea.className).toContain("overflow-y-auto");
    expect(submissionTextarea.className).toContain("resize-none");
    expect(submissionTextarea.className).toContain("border-outline");

    Object.defineProperty(submissionTextarea, "scrollHeight", {
      configurable: true,
      value: 480,
    });
    Object.defineProperty(submissionTextarea, "clientHeight", {
      configurable: true,
      value: 208,
    });

    expect(submissionTextarea.scrollHeight).toBeGreaterThan(
      submissionTextarea.clientHeight,
    );
    submissionTextarea.scrollTop = 72;
    expect(submissionTextarea.scrollTop).toBe(72);
  });

  it("preserves authoring behavior and disabled treatment on the submission textarea", () => {
    const onItemTextChange = vi.fn();

    const { rerender } = renderSubmitWorkCard({
      onItemTextChange,
    });

    const submissionTextarea = screen.getByRole<HTMLTextAreaElement>(
      "textbox",
      {
        name: messages.requestItemLabel(1),
      },
    );

    fireEvent.change(submissionTextarea, {
      target: { value: "Driver review details" },
    });
    expect(onItemTextChange).toHaveBeenCalledWith(
      "submission-item-1",
      "Driver review details",
    );

    fireEvent.paste(submissionTextarea, {
      clipboardData: {
        getData: () => " pasted",
      },
    });

    submissionTextarea.focus();
    expect(submissionTextarea).toHaveFocus();

    rerender(
      <SubmitWorkCard
        draft={defaultDraft}
        isSubmitting
        onAddItem={() => {}}
        onItemTextChange={onItemTextChange}
        onRemoveItem={() => {}}
        onRequestNameChange={() => {}}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={() => {}}
        status={{
          kind: "submitting",
          message: messages.statusMessages.submitting,
        }}
        submitWorkTypeNames={["story", "task"]}
      />,
    );

    expect(
      screen.getByRole<HTMLTextAreaElement>("textbox", {
        name: messages.requestItemLabel(1),
      }),
    ).toBeDisabled();
  });
});
