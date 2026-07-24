import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ModelOperationContentType } from "../../../../api/generated/openapi";
import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";

vi.mock("@you-agent-factory/components/overlays", async (importOriginal) => {
  const actual =
    await importOriginal<
      typeof import("@you-agent-factory/components/overlays")
    >();
  const mockDialog = await import("../../../../testing/mock-dashboard-dialog");

  return {
    ...actual,
    Dialog: mockDialog.Dialog,
    DialogContent: mockDialog.DialogContent,
    DialogDescription: mockDialog.DialogDescription,
    DialogFooter: mockDialog.DialogFooter,
    DialogHeader: mockDialog.DialogHeader,
    DialogOverlay: mockDialog.DialogOverlay,
    DialogPortal: mockDialog.DialogPortal,
    DialogTitle: mockDialog.DialogTitle,
  };
});

import { selectLabeledComboboxOption } from "../../../../testing/select-test-helpers";
import { createEmptyEditableWorkstationCronDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import type { CanonicalFactoryDefinition } from "../../lib/draft/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../../lib/editor/factory-graph-editor-additions";
import { editableWorkstationBehaviorOptions } from "../../lib/editor/factory-graph-editor-additions";
import { createEmptyFactoryGraphAddModelOperationDraft } from "../../lib/factory-graph-add-model-operation-draft";
import { FactoryGraphEditorAddEntityDialog } from "./factory-graph-editor-add-dialog";

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

const currentFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
      ],
    },
  ],
  workstations: [],
};

describe("FactoryGraphEditorAddEntityDialog", () => {
  it("does not render when closed or missing a draft", () => {
    const { rerender } = renderDialog({
      draft: null,
      isOpen: true,
    });

    expect(screen.queryByRole("dialog")).toBeNull();

    rerender(
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={currentFactoryDefinition}
        draft={{ capacity: "1", kind: "resource", name: "gpu" }}
        errors={{}}
        isOpen={false}
        onChange={vi.fn()}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("edits resource capacity fields and submits the form", () => {
    const onChange = vi.fn();
    const onSubmit = vi.fn();
    renderDialog({
      draft: { capacity: "1", kind: "resource", name: "gpu" },
      errors: {
        capacity: "Resource capacity must be a whole number greater than zero.",
      },
      onChange,
      onSubmit,
    });

    const dialog = screen.getByRole("dialog", { name: "Add resource" });
    expect(document.body.contains(dialog)).toBe(true);

    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "cpu" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Capacity" }), {
      target: { value: "3" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(onChange).toHaveBeenNthCalledWith(1, {
      capacity: "1",
      kind: "resource",
      name: "cpu",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      capacity: "3",
      kind: "resource",
      name: "gpu",
    });
    expect(
      screen.getByText(
        "Resource capacity must be a whole number greater than zero.",
      ),
    ).toBeTruthy();
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("renders model worker fields and emits onChange payloads", async () => {
    const user = userEvent.setup();
    const workerChange = vi.fn();
    renderDialog({
      draft: {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "writer",
        operations: [],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
      errors: {
        modelProvider: "Select a model provider for the new worker.",
      },
      onChange: workerChange,
    });

    expect(screen.getByRole("combobox", { name: "Worker type" })).toBeTruthy();
    expect(
      screen.getByRole("combobox", { name: "Model provider" }),
    ).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Model" })).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "Command" })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Args" })).toBeNull();

    await selectLabeledComboboxOption(user, "Model provider", "Cursor");
    fireEvent.change(screen.getByRole("textbox", { name: "Model" }), {
      target: { value: "gpt-5.5" },
    });

    expect(workerChange).toHaveBeenNthCalledWith(1, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "CURSOR",
      name: "writer",
      operations: [],
      provider: "",
      workerType: "INFERENCE_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(2, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "gpt-5.5",
      modelProvider: "",
      name: "writer",
      operations: [],
      provider: "",
      workerType: "INFERENCE_WORKER",
    });
    expect(
      screen.getByText("Select a model provider for the new worker."),
    ).toBeTruthy();
    expect(
      screen.getByText("Blank uses the provider default model."),
    ).toBeTruthy();
  });

  it("toggles script worker fields and clears deselected values", async () => {
    const user = userEvent.setup();
    const workerChange = vi.fn();
    const { rerender } = renderDialog({
      draft: {
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "runner",
        operations: [],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
      errors: {
        modelProvider: "Select a model provider for the new worker.",
      },
      onChange: workerChange,
    });

    await selectLabeledComboboxOption(user, "Worker type", "Script worker");

    expect(workerChange).toHaveBeenCalledWith({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      operations: [],
      provider: "",
      workerType: "SCRIPT_WORKER",
    });

    rerender(
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={currentFactoryDefinition}
        draft={{
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "",
          name: "runner",
          operations: [],
          provider: "",
          workerType: "SCRIPT_WORKER",
        }}
        errors={{}}
        isOpen={true}
        onChange={workerChange}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("combobox", { name: "Model provider" }),
    ).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Model" })).toBeNull();
    expect(screen.getByRole("textbox", { name: "Command" })).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Args" })).toBeTruthy();

    fireEvent.change(screen.getByRole("textbox", { name: "Command" }), {
      target: { value: "./run.sh" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Args" }), {
      target: { value: "--verbose\n--dry-run" },
    });

    expect(workerChange).toHaveBeenNthCalledWith(2, {
      argsText: "",
      command: "./run.sh",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      operations: [],
      provider: "",
      workerType: "SCRIPT_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(3, {
      argsText: "--verbose\n--dry-run",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      operations: [],
      provider: "",
      workerType: "SCRIPT_WORKER",
    });

    await selectLabeledComboboxOption(user, "Worker type", "Inference worker");

    expect(workerChange).toHaveBeenCalledWith({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      operations: [],
      provider: "",
      workerType: "INFERENCE_WORKER",
    });
  });

  it("renders model operation fields for model workers and emits operation drafts", () => {
    const workerChange = vi.fn();
    const operation = createEmptyFactoryGraphAddModelOperationDraft();
    renderDialog({
      draft: {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "CURSOR",
        name: "tts-worker",
        operations: [operation],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
      errors: {
        modelOperations: {
          byIndex: {
            0: {
              name: "Operation names must be uppercase letters, digits, or underscores.",
            },
          },
        },
      },
      onChange: workerChange,
    });

    expect(
      screen.getByRole("heading", { name: "Operation 1", level: 3 }),
    ).toBeTruthy();
    expect(
      screen.getByRole("textbox", { name: "Operation name" }),
    ).toBeTruthy();
    expect(screen.getByText("Input slots")).toBeTruthy();
    expect(screen.getByText("Output slots")).toBeTruthy();
    expect(
      screen.getByText(
        "Operation names must be uppercase letters, digits, or underscores.",
      ),
    ).toBeTruthy();

    const operationSection = screen
      .getByRole("heading", { name: "Operation 1", level: 3 })
      .closest("section");
    expect(operationSection).toBeTruthy();

    fireEvent.change(screen.getByRole("textbox", { name: "Operation name" }), {
      target: { value: "TTS" },
    });
    fireEvent.change(
      within(operationSection as HTMLElement).getAllByRole("textbox", {
        name: "Slot name",
      })[0],
      {
        target: { value: "text" },
      },
    );
    const inputTextCheckbox = operationSection?.querySelector(
      "#factory-graph-add-model-operation-input-slot-0-content-type-TEXT",
    );
    expect(inputTextCheckbox).toBeTruthy();
    fireEvent.click(inputTextCheckbox as HTMLInputElement);

    expect(workerChange).toHaveBeenNthCalledWith(1, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "CURSOR",
      name: "tts-worker",
      operations: [
        {
          ...operation,
          name: "TTS",
        },
      ],
      provider: "",
      workerType: "INFERENCE_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(2, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "CURSOR",
      name: "tts-worker",
      operations: [
        {
          ...operation,
          inputs: [
            {
              ...operation.inputs[0],
              name: "text",
            },
          ],
        },
      ],
      provider: "",
      workerType: "INFERENCE_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(3, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "CURSOR",
      name: "tts-worker",
      operations: [
        {
          ...operation,
          inputs: [
            {
              ...operation.inputs[0],
              contentTypes: [ModelOperationContentType.TEXT],
            },
          ],
        },
      ],
      provider: "",
      workerType: "INFERENCE_WORKER",
    });
  });

  it("exposes the full worker and workstation taxonomy in add creation controls", async () => {
    const user = userEvent.setup();

    renderDialog({
      draft: {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "writer",
        operations: [],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
    });

    await user.click(screen.getByRole("combobox", { name: "Worker type" }));
    const workerListbox = await screen.findByRole("listbox");
    for (const label of [
      "Inference worker",
      "Agent worker",
      "Script worker",
      "Poller worker",
    ]) {
      expect(
        within(workerListbox).getByRole("option", { name: label }),
      ).toBeTruthy();
    }

    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = installDashboardBrowserTestShims();

    renderDialog({
      draft: {
        behavior: "STANDARD",
        body: "",
        cron: null,
        kind: "workstation",
        name: "review",
        workerName: "writer",
        workstationType: "INFERENCE_RUN",
      },
    });

    await user.click(
      screen.getByRole("combobox", { name: "Workstation type" }),
    );
    const workstationListbox = await screen.findByRole("listbox");
    for (const label of [
      "Inference run",
      "Agent run",
      "Script run",
      "Poller run",
      "Logical move",
    ]) {
      expect(
        within(workstationListbox).getByRole("option", { name: label }),
      ).toBeTruthy();
    }
  });

  it("renders work-type specific fields", () => {
    const workTypeChange = vi.fn();
    renderDialog({
      draft: { initialStateName: "", kind: "work-type", name: "article" },
      errors: { initialStateName: "Enter the first work-state identifier." },
      onChange: workTypeChange,
    });

    fireEvent.change(screen.getByRole("textbox", { name: "First state" }), {
      target: { value: "drafting" },
    });

    expect(workTypeChange).toHaveBeenCalledWith({
      initialStateName: "drafting",
      kind: "work-type",
      name: "article",
    });
    expect(
      screen.getByText("Enter the first work-state identifier."),
    ).toBeTruthy();
  });

  it("edits work-state selects from the current factory definition", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderDialog({
      draft: {
        kind: "work-state",
        name: "review",
        stateType: "PROCESSING",
        workTypeName: "",
      },
      errors: {
        workTypeName: "Choose a work type before adding a work state.",
      },
      onChange,
    });

    await selectLabeledComboboxOption(user, "Work type", "story");
    await selectLabeledComboboxOption(user, "State type", "TERMINAL");

    expect(onChange).toHaveBeenNthCalledWith(1, {
      kind: "work-state",
      name: "review",
      stateType: "PROCESSING",
      workTypeName: "story",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      kind: "work-state",
      name: "review",
      stateType: "TERMINAL",
      workTypeName: "",
    });
    expect(
      screen.getByText("Choose a work type before adding a work state."),
    ).toBeTruthy();
  });

  it("includes CRON in add-workstation behavior options", () => {
    expect(editableWorkstationBehaviorOptions()).toContain("CRON");
  });

  it("shows cron fields when CRON behavior is selected", () => {
    renderDialog({
      draft: {
        behavior: "CRON",
        body: "",
        cron: createEmptyEditableWorkstationCronDraft(),
        kind: "workstation",
        name: "scheduler",
        workerName: "writer",
        workstationType: "AGENT_RUN",
      },
      errors: {
        cronSchedule: "Enter a cron schedule before adding this workstation.",
      },
    });

    expect(screen.getByRole("textbox", { name: "Cron schedule" })).toBeTruthy();
    expect(screen.getByLabelText("Cron trigger at start")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Cron jitter" })).toBeTruthy();
    expect(
      screen.getByRole("textbox", { name: "Cron expiry window" }),
    ).toBeTruthy();
    expect(
      screen.getByText("Enter a cron schedule before adding this workstation."),
    ).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Kind" })).toHaveTextContent(
      "Cron",
    );
  });

  it("hides worker and prompt fields for LOGICAL_MOVE workstations", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderDialog({
      draft: {
        behavior: "CRON",
        body: "",
        cron: createEmptyEditableWorkstationCronDraft(),
        kind: "workstation",
        name: "route",
        workerName: "",
        workstationType: "LOGICAL_MOVE",
      },
      onChange,
    });

    expect(
      screen.getByRole("combobox", { name: "Workstation type" }),
    ).toHaveTextContent("Logical move");
    expect(
      screen.queryByRole("combobox", { name: "Assigned worker" }),
    ).toBeNull();
    expect(screen.queryByLabelText("Prompt body")).toBeNull();
    expect(screen.getByRole("textbox", { name: "Cron schedule" })).toBeTruthy();

    await selectLabeledComboboxOption(user, "Workstation type", "Agent run");

    expect(onChange).toHaveBeenCalledWith({
      behavior: "CRON",
      body: "",
      cron: createEmptyEditableWorkstationCronDraft(),
      kind: "workstation",
      name: "route",
      workerName: "writer",
      workstationType: "AGENT_RUN",
    });
  });

  it("clears worker and prompt when switching to LOGICAL_MOVE", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderDialog({
      draft: {
        behavior: "STANDARD",
        body: "Route on schedule.",
        cron: null,
        kind: "workstation",
        name: "route",
        workerName: "writer",
        workstationType: "AGENT_RUN",
      },
      onChange,
    });

    await selectLabeledComboboxOption(user, "Workstation type", "Logical move");

    expect(onChange).toHaveBeenCalledWith({
      behavior: "STANDARD",
      body: "",
      cron: null,
      kind: "workstation",
      name: "route",
      workerName: "",
      workstationType: "LOGICAL_MOVE",
    });
  });

  it("updates cron draft fields from the add dialog", () => {
    const onChange = vi.fn();
    renderDialog({
      draft: {
        behavior: "CRON",
        body: "",
        cron: createEmptyEditableWorkstationCronDraft(),
        kind: "workstation",
        name: "scheduler",
        workerName: "writer",
        workstationType: "AGENT_RUN",
      },
      onChange,
    });

    fireEvent.change(screen.getByRole("textbox", { name: "Cron schedule" }), {
      target: { value: "0 * * * *" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Cron jitter" }), {
      target: { value: "5s" },
    });
    fireEvent.click(screen.getByLabelText("Cron trigger at start"));
    fireEvent.change(
      screen.getByRole("textbox", { name: "Cron expiry window" }),
      {
        target: { value: "30m" },
      },
    );

    expect(onChange).toHaveBeenNthCalledWith(1, {
      behavior: "CRON",
      body: "",
      cron: {
        expiryWindow: "",
        jitter: "",
        schedule: "0 * * * *",
        triggerAtStart: false,
      },
      kind: "workstation",
      name: "scheduler",
      workerName: "writer",
      workstationType: "AGENT_RUN",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      behavior: "CRON",
      body: "",
      cron: {
        expiryWindow: "",
        jitter: "5s",
        schedule: "",
        triggerAtStart: false,
      },
      kind: "workstation",
      name: "scheduler",
      workerName: "writer",
      workstationType: "AGENT_RUN",
    });
    expect(onChange).toHaveBeenNthCalledWith(3, {
      behavior: "CRON",
      body: "",
      cron: {
        expiryWindow: "",
        jitter: "",
        schedule: "",
        triggerAtStart: true,
      },
      kind: "workstation",
      name: "scheduler",
      workerName: "writer",
      workstationType: "AGENT_RUN",
    });
    expect(onChange).toHaveBeenNthCalledWith(4, {
      behavior: "CRON",
      body: "",
      cron: {
        expiryWindow: "30m",
        jitter: "",
        schedule: "",
        triggerAtStart: false,
      },
      kind: "workstation",
      name: "scheduler",
      workerName: "writer",
      workstationType: "AGENT_RUN",
    });
  });

  it("initializes cron draft when switching behavior to CRON", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderDialog({
      draft: {
        behavior: "STANDARD",
        body: "",
        cron: null,
        kind: "workstation",
        name: "review",
        workerName: "writer",
        workstationType: "AGENT_RUN",
      },
      onChange,
    });

    await selectLabeledComboboxOption(user, "Kind", "Cron");

    expect(onChange).toHaveBeenCalledWith({
      behavior: "CRON",
      body: "",
      cron: createEmptyEditableWorkstationCronDraft(),
      kind: "workstation",
      name: "review",
      workerName: "writer",
      workstationType: "AGENT_RUN",
    });
  });

  it("edits workstation worker assignment and prompt body", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const onClose = vi.fn();
    renderDialog({
      draft: {
        behavior: "STANDARD",
        body: "",
        cron: null,
        kind: "workstation",
        name: "review",
        workerName: "",
        workstationType: "AGENT_RUN",
      },
      errors: {
        behavior: "Poller workstations must use a script or hosted worker.",
      },
      onChange,
      onClose,
    });

    await selectLabeledComboboxOption(user, "Kind", "Poller");
    await selectLabeledComboboxOption(user, "Assigned worker", "writer");
    const promptBodyInput = getAddDialogPromptBodyInput();
    fireEvent.change(promptBodyInput, {
      target: { value: "Review the draft." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onChange).toHaveBeenNthCalledWith(1, {
      behavior: "POLLER",
      body: "",
      cron: null,
      kind: "workstation",
      name: "review",
      workerName: "",
      workstationType: "AGENT_RUN",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      behavior: "STANDARD",
      body: "",
      cron: null,
      kind: "workstation",
      name: "review",
      workerName: "writer",
      workstationType: "AGENT_RUN",
    });
    expect(onChange).toHaveBeenNthCalledWith(3, {
      behavior: "STANDARD",
      body: "Review the draft.",
      cron: null,
      kind: "workstation",
      name: "review",
      workerName: "",
      workstationType: "AGENT_RUN",
    });
    expect(
      screen.getByText(
        "Poller workstations must use a script or hosted worker.",
      ),
    ).toBeTruthy();
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

function getAddDialogPromptBodyInput() {
  const monacoSurface = document.querySelector(
    '[data-monaco-editor="workstation-prompt"]',
  );
  expect(monacoSurface).toBeTruthy();
  const promptBodyInput = monacoSurface?.querySelector(
    'textarea[aria-label="Prompt body"]',
  );
  expect(promptBodyInput).toBeTruthy();
  return promptBodyInput as HTMLTextAreaElement;
}

function renderDialog({
  draft,
  errors = {},
  isOpen = true,
  onChange = vi.fn(),
  onClose = vi.fn(),
  onSubmit = vi.fn(),
}: {
  draft: FactoryGraphAddEntityDraft | null;
  errors?: Parameters<typeof FactoryGraphEditorAddEntityDialog>[0]["errors"];
  isOpen?: boolean;
  onChange?: Parameters<
    typeof FactoryGraphEditorAddEntityDialog
  >[0]["onChange"];
  onClose?: () => void;
  onSubmit?: () => void;
}) {
  return render(
    <FactoryGraphEditorAddEntityDialog
      currentFactoryDefinition={currentFactoryDefinition}
      draft={draft}
      errors={errors}
      isOpen={isOpen}
      onChange={onChange}
      onClose={onClose}
      onSubmit={onSubmit}
    />,
  );
}
