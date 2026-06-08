import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { expectStyledCheckbox } from "../../../../../testing/checkbox-test-helpers";

import { EditableConfigurationSection } from "./workstation-editable-configuration-section";
import {
  buildEditableConfigurationSectionReadyState,
  editableConfigurationSectionMessages,
  expandEditableConfigurationSection,
} from "./workstation-editable-configuration-section.test-helpers";

const messages = editableConfigurationSectionMessages;

describe("EditableConfigurationSection cron workstations", () => {
  it("renders the shared styled checkbox for cron trigger-at-start", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          draft: { behavior: "CRON" },
        })}
      />,
    );

    expandEditableConfigurationSection();

    const triggerAtStartCheckbox = screen.getByRole("checkbox", {
      name: messages.cronTriggerAtStartFieldLabel,
    });

    expectStyledCheckbox(triggerAtStartCheckbox);
    expect(triggerAtStartCheckbox.checked).toBe(true);
  });

  it("toggles cron trigger-at-start with Space while focused", async () => {
    const user = userEvent.setup();
    const onCronTriggerAtStartChange = vi.fn();

    render(
      <EditableConfigurationSection
        messages={messages}
        state={{
          ...buildEditableConfigurationSectionReadyState({
            draft: { behavior: "CRON" },
          }),
          onCronTriggerAtStartChange,
        }}
      />,
    );

    expandEditableConfigurationSection();

    const triggerAtStartCheckbox = screen.getByRole("checkbox", {
      name: messages.cronTriggerAtStartFieldLabel,
    });

    triggerAtStartCheckbox.focus();
    await user.keyboard(" ");
    expect(onCronTriggerAtStartChange).toHaveBeenCalledWith(false);
  });

  it("renders cron fields for CRON workstations and wires cron mutators", async () => {
    const user = userEvent.setup();
    const onCronScheduleChange = vi.fn();
    const onCronTriggerAtStartChange = vi.fn();

    render(
      <EditableConfigurationSection
        messages={messages}
        state={{
          ...buildEditableConfigurationSectionReadyState({
            draft: { behavior: "CRON" },
          }),
          onCronScheduleChange,
          onCronTriggerAtStartChange,
        }}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByLabelText(messages.cronScheduleFieldLabel)).toHaveValue(
      "0 9 * * *",
    );
    expect(
      screen.getByLabelText(messages.cronTriggerAtStartFieldLabel),
    ).toBeChecked();

    await user.clear(screen.getByLabelText(messages.cronScheduleFieldLabel));
    await user.type(
      screen.getByLabelText(messages.cronScheduleFieldLabel),
      "0 8 * * *",
    );
    expect(onCronScheduleChange).toHaveBeenCalled();

    await user.click(
      screen.getByLabelText(messages.cronTriggerAtStartFieldLabel),
    );
    expect(onCronTriggerAtStartChange).toHaveBeenCalledWith(false);
  });

  it("exposes invalid state and validation feedback on cron trigger-at-start errors", () => {
    const validationMessage = "trigger_at_start must be a boolean";

    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          draft: { behavior: "CRON" },
          hasValidationErrors: true,
          validationErrors: { cronTriggerAtStart: validationMessage },
        })}
      />,
    );

    expandEditableConfigurationSection();

    const triggerAtStartCheckbox = screen.getByRole("checkbox", {
      name: messages.cronTriggerAtStartFieldLabel,
    });

    expectStyledCheckbox(triggerAtStartCheckbox);
    expect(screen.getByText(validationMessage)).toBeInTheDocument();
  });

  it("shows validation alert when cron field errors exist", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          draft: { behavior: "CRON" },
          hasValidationErrors: true,
          validationErrors: { cronSchedule: "Enter a cron schedule." },
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(
      screen.getByText(messages.editableConfigurationValidationStatus),
    ).toBeInTheDocument();
    expect(screen.getByText("Enter a cron schedule.")).toBeInTheDocument();
  });
});
