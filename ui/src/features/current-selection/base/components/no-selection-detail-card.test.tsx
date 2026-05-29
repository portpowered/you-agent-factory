import { render, screen } from "@testing-library/react";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import { NoSelectionDetailCard } from "./no-selection-detail-card";

describe("NoSelectionDetailCard", () => {
  it("renders no-selection guidance in the same current selection card", () => {
    render(<NoSelectionDetailCard />);

    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeTruthy();
  });

  it("renders localized no-selection guidance for the required non-default locale", () => {
    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <NoSelectionDetailCard />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByRole("heading", { name: "当前选择" })).toBeTruthy();
    expect(
      screen.getByText("选择工作站、工作项或状态节点以查看实时详细信息。"),
    ).toBeTruthy();
  });
});
