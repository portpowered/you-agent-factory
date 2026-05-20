import { render, screen } from "@testing-library/react";

import {
  CurrentSelectionLocaleProvider,
  useCurrentSelectionDetailMessages,
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionShellMessages,
} from "./current-selection-locale";

function LocaleProbe() {
  const detailMessages = useCurrentSelectionDetailMessages();
  const shellMessages = useCurrentSelectionShellMessages();
  const dispatchHistoryMessages = useCurrentSelectionDispatchHistoryMessages();

  return (
    <>
      <p>{detailMessages.requestDetailsTitle}</p>
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

    expect(screen.getByText("请求详情")).toBeTruthy();
    expect(screen.getByText("当前选择")).toBeTruthy();
    expect(screen.getByText("当前分派")).toBeTruthy();
  });

  it("falls back to default messages when the provider is absent", () => {
    render(<LocaleProbe />);

    expect(screen.getByText("Request details")).toBeTruthy();
    expect(screen.getByText("Current selection")).toBeTruthy();
    expect(screen.getByText("Current dispatch")).toBeTruthy();
  });
});
