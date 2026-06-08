import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { EditableConfigurationSection } from "./workstation-editable-configuration-section";
import {
  buildEditableConfigurationSectionReadyState,
  editableConfigurationSectionMessages,
  expandEditableConfigurationSection,
} from "./workstation-editable-configuration-section.test-helpers";

const messages = editableConfigurationSectionMessages;

describe("EditableConfigurationSection cron workstations", () => {
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
