import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { EditableConfigurationWorkstationGuardsField } from "./workstation-guards-field";

const messages = getWorkstationDetailMessages("en");

describe("EditableConfigurationWorkstationGuardsField", () => {
  it("renders guard rows with type and summary", () => {
    render(
      <EditableConfigurationWorkstationGuardsField
        guards={[
          {
            maxVisits: 2,
            type: "VISIT_COUNT",
            workstation: "Review",
          },
          {
            matchConfig: { inputKey: ".Name" },
            type: "MATCHES_FIELDS",
          },
        ]}
        messages={messages}
        onGuardsChange={vi.fn()}
        workstationOptionsState={{
          options: ["Plan", "Review"],
          status: "ready",
        }}
      />,
    );

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
    expect(within(guardArticles[1]).getByText(".Name")).toBeTruthy();
  });

  it("adds and removes guards through the draft callback", async () => {
    const user = userEvent.setup();
    const onGuardsChange = vi.fn();

    const { rerender } = render(
      <EditableConfigurationWorkstationGuardsField
        guards={[]}
        messages={messages}
        onGuardsChange={onGuardsChange}
        workstationOptionsState={{
          options: ["Plan", "Review"],
          status: "ready",
        }}
      />,
    );

    await user.selectOptions(screen.getByLabelText("Add guard"), "VISIT_COUNT");

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
        workstationOptionsState={{
          options: ["Plan", "Review"],
          status: "ready",
        }}
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

  it("renders guard field validation errors with role=alert", () => {
    render(
      <EditableConfigurationWorkstationGuardsField
        fieldErrors={{
          "guards[0].maxVisits": "Max visits must be a positive whole number.",
          "guards[0].workstation":
            "Select the workstation whose visits are counted.",
        }}
        guards={[
          {
            maxVisits: 0,
            type: "VISIT_COUNT",
            workstation: "",
          },
        ]}
        messages={messages}
        onGuardsChange={vi.fn()}
        workstationOptionsState={{
          options: ["Plan", "Review"],
          status: "ready",
        }}
      />,
    );

    expect(screen.getAllByRole("alert")).toHaveLength(2);
    expect(
      screen.getByText("Max visits must be a positive whole number."),
    ).toBeTruthy();
  });
});
