import { render, screen } from "@testing-library/react";

import {
  CurrentSelectionLocaleProvider,
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionShellMessages,
} from "./current-selection-locale";

function LocaleProbe() {
  const shellMessages = useCurrentSelectionShellMessages();
  const dispatchHistoryMessages = useCurrentSelectionDispatchHistoryMessages();

  return (
    <>
      <p>{shellMessages.title}</p>
      <p>{dispatchHistoryMessages.currentDispatchBadge}</p>
    </>
  );
}

describe("CurrentSelectionLocaleProvider", () => {
  it("resolves shell and dispatch-history messages through the current-selection locale context", () => {
    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <LocaleProbe />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByText("当前选择")).toBeTruthy();
    expect(screen.getByText("当前分派")).toBeTruthy();
  });

  it("falls back to default messages when the provider is absent", () => {
    render(<LocaleProbe />);

    expect(screen.getByText("Current selection")).toBeTruthy();
    expect(screen.getByText("Current dispatch")).toBeTruthy();
  });
});
