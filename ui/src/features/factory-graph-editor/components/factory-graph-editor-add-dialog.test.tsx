import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";

vi.mock("../../../components/ui/dialog", () => ({
  Dialog: ({ children, open }: { children: ReactNode; open?: boolean }) =>
    open ? children : null,
  DialogContent: ({ children }: { children: ReactNode }) => (
    <div aria-labelledby="factory-graph-mock-dialog-title" role="dialog">
      {children}
    </div>
  ),
  DialogDescription: ({ children }: { children: ReactNode }) => (
    <p>{children}</p>
  ),
  DialogFooter: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children: ReactNode }) => (
    <h2 id="factory-graph-mock-dialog-title">{children}</h2>
  ),
  DialogOverlay: () => null,
  DialogPortal: ({ children }: { children: ReactNode }) => children,
}));

import { selectLabeledComboboxOption } from "../../../testing/select-test-helpers";
import { createEmptyEditableWorkstationCronDraft } from "../../current-factory-definition/lib/workstation-editable-values";
import type { CanonicalFactoryDefinition } from "../lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import { editableWorkstationBehaviorOptions } from "../lib/factory-graph-editor-additions";
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
      workerType: "MODEL_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(2, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "gpt-5.5",
      modelProvider: "",
      name: "writer",
      workerType: "MODEL_WORKER",
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
        workerType: "MODEL_WORKER",
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
      workerType: "SCRIPT_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(3, {
      argsText: "--verbose\n--dry-run",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      workerType: "SCRIPT_WORKER",
    });

    await selectLabeledComboboxOption(user, "Worker type", "Model worker");

    expect(workerChange).toHaveBeenCalledWith({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
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

    await selectLabeledComboboxOption(
      user,
      "Workstation type",
      "Model workstation",
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
        workstationType: "MODEL_WORKSTATION",
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
        workstationType: "MODEL_WORKSTATION",
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
      workstationType: "MODEL_WORKSTATION",
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
        workstationType: "MODEL_WORKSTATION",
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
