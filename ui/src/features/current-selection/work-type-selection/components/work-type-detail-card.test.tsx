import { fireEvent, render, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useEditableWorkTypeConfigurationState } from "../hooks/use-editable-work-type-configuration-state";
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
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
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

describe("WorkTypeDetailCard", () => {
  beforeEach(() => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument(),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);
  });

  it("renders a loading message while the factory document is pending", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      isError: false,
      isPending: true,
      status: "pending",
    } as never);

    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    expect(
      screen.getByText(
        "Loading the current factory definition for this work type.",
      ),
    ).toBeTruthy();
  });

  it("renders an error alert when the factory document fails to load", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: new Error("factory offline"),
      isError: true,
      isPending: false,
      status: "error",
    } as never);

    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    expect(screen.getByRole("alert").textContent).toContain(
      "Work type definition unavailable.",
    );
    expect(screen.getByText(/factory offline/)).toBeTruthy();
  });

  it("renders empty guidance when the selected work type is missing", () => {
    render(<WorkTypeDetailCardHarness workTypeName="missing" />);

    expect(
      screen.getByText(
        "This running factory definition does not include the selected work type.",
      ),
    ).toBeTruthy();
  });

  it("renders editable name and handling behavior fields with read-only state rows when ready", () => {
    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    const nameInput = within(panel).getByLabelText("Work type");

    expect(nameInput.getAttribute("value")).toBe("story");
    expect(
      within(panel).getByRole("checkbox", { name: "Default CLI handling" })
        .checked,
    ).toBe(false);
    expect(within(panel).getByRole("heading", { name: "States" })).toBeTruthy();
    expect(within(panel).getByText("queued")).toBeTruthy();
    expect(within(panel).getByText("Initial")).toBeTruthy();
    expect(within(panel).getByText("done")).toBeTruthy();
    expect(within(panel).getByText("Completed")).toBeTruthy();
  });

  it("surfaces name validation errors with aria-invalid and role alert", () => {
    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    const nameInput = within(panel).getByLabelText("Work type");

    fireEvent.change(nameInput, { target: { value: "   " } });

    expect(nameInput.getAttribute("aria-invalid")).toBe("true");
    expect(
      within(panel).getByText(
        "Enter a work type name before saving this work type.",
      ),
    ).toBeTruthy();
  });
});
