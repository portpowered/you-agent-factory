import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { expectNoInlineSaveOutcomesIn } from "../../base/components/current-selection-save-toast-test-helpers";
import { EditableConfigurationSection } from "./workstation-editable-configuration-section";
import {
  buildEditableConfigurationSectionReadyState,
  editableConfigurationSectionMessages,
  expandEditableConfigurationSection,
} from "./workstation-editable-configuration-section.test-helpers";

const messages = editableConfigurationSectionMessages;

describe("EditableConfigurationSection async states", () => {
  it("shows loading, error, and empty copy when expanded", () => {
    const { rerender } = render(
      <EditableConfigurationSection
        messages={messages}
        state={{ status: "loading" }}
      />,
    );

    expandEditableConfigurationSection();
    expect(
      screen.getByText(
        "Loading the current factory definition for this workstation.",
      ),
    ).toBeInTheDocument();

    rerender(
      <EditableConfigurationSection
        messages={messages}
        state={{ status: "error", errorMessage: "Factory unavailable." }}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Editable configuration unavailable. Factory unavailable.",
    );

    rerender(
      <EditableConfigurationSection
        messages={messages}
        state={{ status: "empty", message: "No editable values." }}
      />,
    );
    expect(screen.getByText("No editable values.")).toBeInTheDocument();
  });
});

describe("EditableConfigurationSection footer save controls", () => {
  it("renders footer save and reset controls when onSaveConfiguration is provided", async () => {
    const user = userEvent.setup();
    const onSaveConfiguration = vi.fn();
    const onResetToLatest = vi.fn();

    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={onSaveConfiguration}
        state={{
          ...buildEditableConfigurationSectionReadyState({ isDirty: true }),
          onResetToLatest,
        }}
      />,
    );

    expandEditableConfigurationSection();

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton).toBeEnabled();

    await user.click(saveButton);
    expect(onSaveConfiguration).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Reset to latest" }));
    expect(onResetToLatest).toHaveBeenCalledTimes(1);
  });

  it("disables footer save when validation errors exist or the draft is clean", () => {
    const { rerender } = render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildEditableConfigurationSectionReadyState({
          hasValidationErrors: true,
          isDirty: true,
        })}
      />,
    );

    expandEditableConfigurationSection();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();

    rerender(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildEditableConfigurationSectionReadyState({ isDirty: false })}
      />,
    );
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();
  });
});

describe("EditableConfigurationSection worker options", () => {
  it("surfaces worker option empty state inside the ready form", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          workerOptionsState: {
            message: "No workers are configured in this factory.",
            status: "empty",
          },
        })}
      />,
    );

    expandEditableConfigurationSection();
    expect(
      screen.getByText("No workers are configured in this factory."),
    ).toBeInTheDocument();
  });

  it("surfaces worker option error state inside the ready form", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          workerOptionsState: {
            message: "Worker list failed to load.",
            status: "error",
          },
        })}
      />,
    );

    expandEditableConfigurationSection();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Worker selection unavailable. Worker list failed to load.",
    );
  });

  it("renders worker select when options are ready", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState()}
      />,
    );

    expandEditableConfigurationSection();
    expect(screen.getByLabelText("Worker")).toHaveValue("reviewer");
  });
});

describe("EditableConfigurationSection workstation name field", () => {
  it("renders the workstation name input prefilled from the draft", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          draft: { name: "Review" },
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByLabelText("Workstation name")).toHaveValue("Review");
  });

  it("calls onNameChange when the workstation name input changes", async () => {
    const user = userEvent.setup();
    const onNameChange = vi.fn();

    render(
      <EditableConfigurationSection
        messages={messages}
        state={{
          ...buildEditableConfigurationSectionReadyState(),
          onNameChange,
        }}
      />,
    );

    expandEditableConfigurationSection();

    await user.clear(screen.getByLabelText("Workstation name"));
    await user.type(screen.getByLabelText("Workstation name"), "Plan");
    expect(onNameChange).toHaveBeenCalled();
  });

  it("shows name validation errors on the name field", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildEditableConfigurationSectionReadyState({
          hasValidationErrors: true,
          isDirty: true,
          validationErrors: {
            name: 'A workstation named "Plan" already exists in the running factory definition.',
          },
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByLabelText("Workstation name")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(
      screen.getByText(
        'A workstation named "Plan" already exists in the running factory definition.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();
  });

  it("enables save when only the name is dirty and validation passes", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildEditableConfigurationSectionReadyState({
          draft: { name: "Renamed" },
          isDirty: true,
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByRole("button", { name: "Save changes" })).toBeEnabled();
  });

  it("shows server-changed hint for overwritten name field", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          overwriteFieldNames: ["name"],
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(
      screen.getByText(messages.editableConfigurationServerFieldChangedHint),
    ).toBeInTheDocument();
  });

  it("renders the workstation name field for logical move workstations", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "LOGICAL_MOVE",
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByLabelText("Workstation name")).toBeInTheDocument();
    expect(screen.queryByLabelText("Worker")).not.toBeInTheDocument();
  });
});

describe("EditableConfigurationSection logical move workstations", () => {
  it("shows guard fields without worker assignment controls", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "LOGICAL_MOVE",
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.queryByLabelText("Worker")).not.toBeInTheDocument();
    expect(screen.getByText("Workstation guards")).toBeInTheDocument();
    expect(screen.getByText("Input guards")).toBeInTheDocument();
  });
});

describe("EditableConfigurationSection model workstation fields", () => {
  it("renders kind, runner, and prompt fields and updates behavior", async () => {
    const user = userEvent.setup();
    const onBehaviorChange = vi.fn();

    render(
      <EditableConfigurationSection
        messages={messages}
        state={{
          ...buildEditableConfigurationSectionReadyState(),
          onBehaviorChange,
        }}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByLabelText("Kind")).toHaveValue("STANDARD");
    expect(screen.getByLabelText("Runner")).toBeInTheDocument();
    expect(screen.getByLabelText("Prompt")).toHaveValue("Review prompt");

    await user.selectOptions(screen.getByLabelText("Kind"), "REPEATER");
    expect(onBehaviorChange).toHaveBeenCalledWith("REPEATER");
  });

  it("shows shared worker scope hint when the draft worker is shared", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          initialValues: {
            sharedWorkerWorkstationNamesByWorkerName: {
              reviewer: ["Plan"],
            },
          },
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(
      screen.getByText(/Worker reviewer is also used by Plan/i),
    ).toBeInTheDocument();
  });

  it("shows server-changed hints for overwritten worker, kind, runner, and prompt fields", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          overwriteFieldNames: ["worker", "behavior", "runner", "prompt"],
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(
      screen.getAllByText(messages.editableConfigurationServerFieldChangedHint),
    ).toHaveLength(4);
  });
});

describe("EditableConfigurationSection model workstation save feedback", () => {
  it("shows validation alert for blocking field errors", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        state={buildEditableConfigurationSectionReadyState({
          hasValidationErrors: true,
          validationErrors: { workerName: "Select a worker." },
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByRole("alert")).toHaveTextContent(
      messages.editableConfigurationValidationStatus,
    );
  });

  it("does not render inline save outcome copy in the configuration section", () => {
    const { container, rerender } = render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        saveState={{
          status: "success",
          message: messages.editableConfigurationSaveSuccess,
        }}
        state={buildEditableConfigurationSectionReadyState({
          isDirty: false,
        })}
      />,
    );

    expandEditableConfigurationSection();

    expectNoInlineSaveOutcomesIn(
      container.querySelector("section") as HTMLElement,
    );

    rerender(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        saveState={{
          errorMessage: "The current factory rejected the workstation update.",
          status: "error",
        }}
        state={buildEditableConfigurationSectionReadyState({ isDirty: true })}
      />,
    );

    expectNoInlineSaveOutcomesIn(
      container.querySelector("section") as HTMLElement,
    );
  });

  it("omits global validation alert when only prompt diagnostics block save", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          hasValidationErrors: true,
          isDirty: true,
          promptDiagnostics: [
            { message: "Unknown variable.", severity: "error" },
          ],
          validationErrors: { prompt: "Resolve prompt diagnostics." },
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(
      screen.getByText(messages.editableConfigurationPromptDiagnosticsHeading),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        `${messages.editableConfigurationPromptVariableDiagnosticLabel}: Unknown variable.`,
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(messages.editableConfigurationValidationStatus),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Resolve prompt diagnostics.")).toBeInTheDocument();
  });

  it("disables footer save and reset while submitting", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        saveState={{ status: "submitting" }}
        state={buildEditableConfigurationSectionReadyState({ isDirty: true })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Reset to latest" }),
    ).toBeDisabled();
  });
});

describe("EditableConfigurationSection overwrite and field errors", () => {
  it("shows overwrite warning and merges save field errors into guard fields", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        onSaveConfiguration={() => undefined}
        saveState={{
          fieldErrors: {
            "guards[0].maxVisits":
              "Max visits must be a positive whole number.",
          },
          status: "error",
          message: "Saving failed.",
        }}
        state={buildEditableConfigurationSectionReadyState({
          draft: {
            guards: [
              {
                type: "VISIT_COUNT",
                workstation: "Plan",
                maxVisits: 0,
              },
            ],
          },
          isDirty: true,
          overwriteFieldNames: ["worker", "prompt"],
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(
      screen.getByText(/Saving now will overwrite newer server values/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Max visits must be a positive whole number."),
    ).toBeInTheDocument();
  });
});
