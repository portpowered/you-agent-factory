import { render, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useEditableWorkTypeConfigurationState } from "../hooks/use-editable-work-type-configuration-state";
import type {
  EditableWorkTypeConfigurationState,
  EditableWorkTypeSaveState,
} from "../lib/detail-card-types";
import { WorkTypeDetailCard } from "./work-type-detail-card";

vi.mock(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

function buildFactoryDocument(
  overrides?: Partial<CurrentFactoryDocument>,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        id: "review",
        inputs: [],
        name: "Review",
        worker: "reviewer",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [{ name: "queued", type: "INITIAL" }],
      },
    ],
    ...overrides,
  };
}

function WorkTypeDetailCardHarness({ workTypeName }: { workTypeName: string }) {
  const editableConfigurationState = useEditableWorkTypeConfigurationState(
    { kind: "work-type", workTypeName },
    workTypeName,
  );

  return (
    <WorkTypeDetailCard
      editableConfigurationState={editableConfigurationState}
      workTypeName={workTypeName}
    />
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: default-handling coverage keeps factory reload and save-error cases together.
describe("WorkTypeDetailCard default handling", () => {
  beforeEach(() => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument(),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);
  });

  it("shows default status and a checked control when the factory marks the work type default", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            name: "story",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    const defaultCheckbox = within(panel).getByRole("checkbox", {
      name: "Mark as default work type",
    });

    expect(defaultCheckbox.checked).toBe(true);
    expect(within(panel).getByRole("status").textContent).toBe(
      "Default work type",
    );
    expect(
      within(panel).queryByText(
        "Default work type supplies prompt text for simplified factory runs.",
      ),
    ).toBeNull();
  });

  it("refreshes default status after the factory document reloads with updated handling behavior", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            name: "story",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const view = render(<WorkTypeDetailCardHarness workTypeName="story" />);
    const panel = () =>
      screen.getByRole("article", { name: "Current selection" });

    expect(
      within(panel()).getByRole("checkbox", {
        name: "Mark as default work type",
      }).checked,
    ).toBe(true);

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workTypes: [
          {
            name: "story",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    view.rerender(<WorkTypeDetailCardHarness workTypeName="story" />);

    expect(
      within(panel()).getByRole("checkbox", {
        name: "Mark as default work type",
      }).checked,
    ).toBe(false);
    expect(within(panel()).queryByRole("status")).toBeNull();
  });

  it("surfaces save handlingBehavior errors on the default control", () => {
    const editableConfigurationState: EditableWorkTypeConfigurationState = {
      baseVersion: buildFactoryDocument().version,
      canSave: true,
      draft: {
        handlingBehavior: ["DEFAULT"],
        name: "story",
      },
      hasValidationErrors: false,
      initialValues: {
        handlingBehavior: null,
        name: "story",
        states: [{ name: "queued", type: "INITIAL" }],
      },
      isDirty: true,
      markChangesSaved: vi.fn(),
      onHandlingBehaviorChange: vi.fn(),
      onNameChange: vi.fn(),
      onResetToLatest: vi.fn(),
      pendingFactoryDefinition: buildFactoryDocument({
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            name: "story",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
      }),
      status: "ready",
      validationErrors: {},
    };
    const saveState: EditableWorkTypeSaveState = {
      errorMessage: "Only one work type can be the factory default.",
      fieldErrors: {
        handlingBehavior: "Only one work type can be the factory default.",
      },
      status: "error",
    };

    render(
      <WorkTypeDetailCard
        editableConfigurationState={editableConfigurationState}
        saveState={saveState}
        workTypeName="story"
      />,
    );

    const panel = screen.getByRole("article", { name: "Current selection" });
    const defaultCheckbox = within(panel).getByRole("checkbox", {
      name: "Mark as default work type",
    });

    expect(defaultCheckbox.getAttribute("aria-invalid")).toBe("true");
    expect(
      within(panel).getByText("Only one work type can be the factory default."),
    ).toBeTruthy();
  });
});
