import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, useState } from "react";
import { afterEach, beforeEach } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { buildWorkstationGuardSelectorCompletionItems } from "../../../../../components/prompt-editor";
import { selectLabeledComboboxOption } from "../../../../../testing/select-test-helpers";
import { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import { EditableConfigurationWorkstationGuardsField } from "./workstation-guards-field";

const messages = getWorkstationDetailMessages("en");
const readyWorkstationOptions = {
  options: ["Plan", "Review"],
  status: "ready" as const,
};

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

function renderWorkstationGuardsField(
  props: Partial<
    ComponentProps<typeof EditableConfigurationWorkstationGuardsField>
  > = {},
) {
  return render(
    <EditableConfigurationWorkstationGuardsField
      guards={[]}
      messages={messages}
      onGuardsChange={vi.fn()}
      workstationOptionsState={readyWorkstationOptions}
      {...props}
    />,
  );
}

function typeSelectorValue(selectorInput: HTMLElement, targetValue: string) {
  let draftValue = "";
  for (const character of targetValue) {
    draftValue += character;
    fireEvent.change(selectorInput, { target: { value: draftValue } });
  }
}

describe("EditableConfigurationWorkstationGuardsField rendering", () => {
  it("renders guard rows with type and summary", () => {
    renderWorkstationGuardsField({
      guards: [
        {
          maxVisits: 2,
          type: "VISIT_COUNT",
          workstation: "Review",
        },
        {
          matchConfig: { inputKey: ".Name" },
          type: "MATCHES_FIELDS",
        },
      ],
    });

    expect(
      screen.getByRole("heading", { name: "Workstation guards" }),
    ).toBeTruthy();
    const guardArticles = screen.getAllByRole("article");
    expect(
      within(guardArticles[0]).getByRole("heading", { level: 6 }).textContent,
    ).toBe("Visit count");
    expect(within(guardArticles[0]).getByText("Review · max 2")).toBeTruthy();
    expect(
      within(guardArticles[1]).getByRole("heading", { level: 6 }).textContent,
    ).toBe("Matches fields");
    const matchesFieldsHeader = guardArticles[1].querySelector(
      "#editable-workstation-guard-1-heading",
    )?.parentElement as HTMLElement;
    expect(within(matchesFieldsHeader).getByText(".Name")).toBeTruthy();
  });

  it("adds and removes guards through the draft callback", async () => {
    const user = userEvent.setup();
    const onGuardsChange = vi.fn();

    const { rerender } = renderWorkstationGuardsField({
      onGuardsChange,
    });

    await selectLabeledComboboxOption(user, "Add guard", "Visit count");

    expect(onGuardsChange).toHaveBeenCalledWith([
      {
        maxVisits: 1,
        type: "VISIT_COUNT",
        workstation: "Plan",
      },
    ]);

    onGuardsChange.mockClear();
    rerender(
      <EditableConfigurationWorkstationGuardsField
        guards={[
          {
            maxVisits: 1,
            type: "VISIT_COUNT",
            workstation: "Plan",
          },
        ]}
        messages={messages}
        onGuardsChange={onGuardsChange}
        workstationOptionsState={readyWorkstationOptions}
      />,
    );

    const removeButton = within(screen.getByRole("article")).getByRole(
      "button",
      {
        name: "Remove guard",
      },
    );
    await user.click(removeButton);

    expect(onGuardsChange).toHaveBeenCalledWith([]);
  });
});

describe("EditableConfigurationWorkstationGuardsField MATCHES_FIELDS selector", () => {
  it("updates guards draft matchConfig.inputKey with the exact selector string", () => {
    const onGuardsChange = vi.fn();
    const selector = '.Tags["_last_output"]';

    renderWorkstationGuardsField({
      guards: [
        {
          matchConfig: { inputKey: "" },
          type: "MATCHES_FIELDS",
        },
      ],
      onGuardsChange,
    });

    const selectorInput = screen.getByLabelText("Field selector");
    fireEvent.change(selectorInput, { target: { value: selector } });

    expect(onGuardsChange).toHaveBeenLastCalledWith([
      {
        matchConfig: { inputKey: selector },
        type: "MATCHES_FIELDS",
      },
    ]);
  });

  it("keeps MATCHES_FIELDS selector focused while typing multiple characters", async () => {
    const user = userEvent.setup();

    function Harness() {
      const [guards, setGuards] = useState([
        {
          matchConfig: { inputKey: "" },
          type: "MATCHES_FIELDS" as const,
        },
      ]);

      return (
        <EditableConfigurationWorkstationGuardsField
          guards={guards}
          messages={messages}
          onGuardsChange={setGuards}
          workstationOptionsState={readyWorkstationOptions}
        />
      );
    }

    render(<Harness />);

    const selectorInput = screen.getByLabelText("Field selector");
    const targetValue = '.Tags["_last_output"]';
    await user.click(selectorInput);
    typeSelectorValue(selectorInput, targetValue);

    expect(selectorInput).toHaveFocus();
    expect(selectorInput).toHaveValue(targetValue);
  });

  it("renders MATCHES_FIELDS selector with Monaco guard-selector editor", () => {
    renderWorkstationGuardsField({
      guards: [
        {
          matchConfig: { inputKey: ".Name" },
          type: "MATCHES_FIELDS",
        },
      ],
    });

    const selectorEditor = screen.getByLabelText("Field selector");
    expect(selectorEditor).toHaveAttribute(
      "data-monaco-editor",
      "workstation-guard-selector",
    );
    expect(selectorEditor).toHaveValue(".Name");
  });

  it("applies accepted guard selector suggestion values through onGuardsChange", () => {
    const onGuardsChange = vi.fn();
    const suggestion = buildWorkstationGuardSelectorCompletionItems()[0];

    renderWorkstationGuardsField({
      guards: [
        {
          matchConfig: { inputKey: "" },
          type: "MATCHES_FIELDS",
        },
      ],
      onGuardsChange,
    });

    fireEvent.change(screen.getByLabelText("Field selector"), {
      target: { value: suggestion.insertText },
    });

    expect(onGuardsChange).toHaveBeenLastCalledWith([
      {
        matchConfig: { inputKey: suggestion.insertText },
        type: "MATCHES_FIELDS",
      },
    ]);
  });
});

describe("EditableConfigurationWorkstationGuardsField validation", () => {
  it("renders guard field validation errors with role=alert", () => {
    renderWorkstationGuardsField({
      fieldErrors: {
        "guards[0].maxVisits": "Max visits must be a positive whole number.",
        "guards[0].workstation":
          "Select the workstation whose visits are counted.",
      },
      guards: [
        {
          maxVisits: 0,
          type: "VISIT_COUNT",
          workstation: "",
        },
      ],
    });

    expect(screen.getAllByRole("alert")).toHaveLength(2);
    expect(
      screen.getByText("Max visits must be a positive whole number."),
    ).toBeTruthy();
  });

  it("shows required-field error for empty MATCHES_FIELDS inputKey", () => {
    renderWorkstationGuardsField({
      fieldErrors: {
        "guards[0].matchConfig.inputKey":
          messages.editableConfigurationMatchesFieldsInputKeyRequired,
      },
      guards: [
        {
          matchConfig: { inputKey: "" },
          type: "MATCHES_FIELDS",
        },
      ],
    });

    expect(screen.getByRole("alert")).toHaveTextContent(
      messages.editableConfigurationMatchesFieldsInputKeyRequired,
    );

    const selectorEditor = screen.getByLabelText("Field selector");
    expect(selectorEditor.parentElement?.getAttribute("aria-invalid")).toBe(
      "true",
    );
    expect(selectorEditor.parentElement?.getAttribute("aria-describedby")).toBe(
      "editable-workstation-guard-0-input-key-error",
    );
  });
});
