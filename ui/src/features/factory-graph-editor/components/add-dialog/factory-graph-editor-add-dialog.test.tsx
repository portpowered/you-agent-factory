import { fireEvent, render, screen, within } from "@testing-library/react";
import { ModelOperationContentType } from "../../../../api/generated/openapi";
import { createEmptyEditableWorkstationCronDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import { createEmptyFactoryGraphAddModelOperationDraft } from "../../lib/factory-graph-add-model-operation-draft";
import type { CanonicalFactoryDefinition } from "../../lib/draft/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../../lib/editor/factory-graph-editor-additions";
import { editableWorkstationBehaviorOptions } from "../../lib/editor/factory-graph-editor-additions";
import { FactoryGraphEditorAddEntityDialog } from "./factory-graph-editor-add-dialog";

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

  it("renders model worker fields and emits onChange payloads", () => {
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
        workerType: "MODEL_WORKER",
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

    fireEvent.change(screen.getByRole("combobox", { name: "Model provider" }), {
      target: { value: "CURSOR" },
    });
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
      workerType: "MODEL_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(2, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "gpt-5.5",
      modelProvider: "",
      name: "writer",
      operations: [],
      workerType: "MODEL_WORKER",
    });
    expect(
      screen.getByText("Select a model provider for the new worker."),
    ).toBeTruthy();
    expect(
      screen.getByText("Blank uses the provider default model."),
    ).toBeTruthy();
  });

  it("toggles script worker fields and clears deselected values", () => {
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
        workerType: "MODEL_WORKER",
      },
      errors: {
        modelProvider: "Select a model provider for the new worker.",
      },
      onChange: workerChange,
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Worker type" }), {
      target: { value: "SCRIPT_WORKER" },
    });

    expect(workerChange).toHaveBeenCalledWith({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      operations: [],
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
      workerType: "SCRIPT_WORKER",
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Worker type" }), {
      target: { value: "MODEL_WORKER" },
    });

    expect(workerChange).toHaveBeenCalledWith({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      operations: [],
      workerType: "MODEL_WORKER",
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
        workerType: "MODEL_WORKER",
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
    expect(screen.getByRole("textbox", { name: "Operation name" })).toBeTruthy();
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
      workerType: "MODEL_WORKER",
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
      workerType: "MODEL_WORKER",
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
              contentTypes: [
                ModelOperationContentType.ModelOperationContentTypeText,
              ],
            },
          ],
        },
      ],
      workerType: "MODEL_WORKER",
    });
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

  it("edits work-state selects from the current factory definition", () => {
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

    fireEvent.change(screen.getByRole("combobox", { name: "Work type" }), {
      target: { value: "story" },
    });
    fireEvent.change(screen.getByRole("combobox", { name: "State type" }), {
      target: { value: "TERMINAL" },
    });

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
        workstationType: "MODEL_WORKSTATION",
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
    expect(screen.getByRole("option", { name: "Cron" })).toBeTruthy();
  });

  it("hides worker and prompt fields for LOGICAL_MOVE workstations", () => {
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
      (
        screen.getByRole("combobox", {
          name: "Workstation type",
        }) as HTMLSelectElement
      ).value,
    ).toBe("LOGICAL_MOVE");
    expect(
      screen.queryByRole("combobox", { name: "Assigned worker" }),
    ).toBeNull();
    expect(screen.queryByLabelText("Prompt body")).toBeNull();
    expect(screen.getByRole("textbox", { name: "Cron schedule" })).toBeTruthy();

    fireEvent.change(
      screen.getByRole("combobox", { name: "Workstation type" }),
      {
        target: { value: "MODEL_WORKSTATION" },
      },
    );

    expect(onChange).toHaveBeenCalledWith({
      behavior: "CRON",
      body: "",
      cron: createEmptyEditableWorkstationCronDraft(),
      kind: "workstation",
      name: "route",
      workerName: "writer",
      workstationType: "MODEL_WORKSTATION",
    });
  });

  it("clears worker and prompt when switching to LOGICAL_MOVE", () => {
    const onChange = vi.fn();
    renderDialog({
      draft: {
        behavior: "STANDARD",
        body: "Route on schedule.",
        cron: null,
        kind: "workstation",
        name: "route",
        workerName: "writer",
        workstationType: "MODEL_WORKSTATION",
      },
      onChange,
    });

    fireEvent.change(
      screen.getByRole("combobox", { name: "Workstation type" }),
      {
        target: { value: "LOGICAL_MOVE" },
      },
    );

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
        workstationType: "MODEL_WORKSTATION",
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
      workstationType: "MODEL_WORKSTATION",
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
      workstationType: "MODEL_WORKSTATION",
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
      workstationType: "MODEL_WORKSTATION",
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
      workstationType: "MODEL_WORKSTATION",
    });
  });

  it("initializes cron draft when switching behavior to CRON", () => {
    const onChange = vi.fn();
    renderDialog({
      draft: {
        behavior: "STANDARD",
        body: "",
        cron: null,
        kind: "workstation",
        name: "review",
        workerName: "writer",
        workstationType: "MODEL_WORKSTATION",
      },
      onChange,
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Kind" }), {
      target: { value: "CRON" },
    });

    expect(onChange).toHaveBeenCalledWith({
      behavior: "CRON",
      body: "",
      cron: createEmptyEditableWorkstationCronDraft(),
      kind: "workstation",
      name: "review",
      workerName: "writer",
      workstationType: "MODEL_WORKSTATION",
    });
  });

  it("edits workstation worker assignment and prompt body", () => {
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
        workstationType: "MODEL_WORKSTATION",
      },
      errors: {
        behavior: "Poller workstations must use a script or hosted worker.",
      },
      onChange,
      onClose,
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Kind" }), {
      target: { value: "POLLER" },
    });
    fireEvent.change(
      screen.getByRole("combobox", { name: "Assigned worker" }),
      {
        target: { value: "writer" },
      },
    );
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
      workstationType: "MODEL_WORKSTATION",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      behavior: "STANDARD",
      body: "",
      cron: null,
      kind: "workstation",
      name: "review",
      workerName: "writer",
      workstationType: "MODEL_WORKSTATION",
    });
    expect(onChange).toHaveBeenNthCalledWith(3, {
      behavior: "STANDARD",
      body: "Review the draft.",
      cron: null,
      kind: "workstation",
      name: "review",
      workerName: "",
      workstationType: "MODEL_WORKSTATION",
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
