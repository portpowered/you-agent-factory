import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { EditableConfigurationSection } from "./workstation-editable-configuration-section";
import {
  buildEditableConfigurationSectionReadyState,
  editableConfigurationSectionMessages,
  expandEditableConfigurationSection,
} from "./workstation-editable-configuration-section.test-helpers";

const messages = editableConfigurationSectionMessages;

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
