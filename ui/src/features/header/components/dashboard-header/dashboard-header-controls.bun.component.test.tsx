import { expect, it, mock } from "bun:test";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { NATIVE_LANGUAGE_LABELS } from "../../../../i18n";
import {
  AppColorPaletteProvider,
  applyDocumentColorPalette,
} from "../../../../theme";
import { getExportDialogMessages } from "../../../export/messages/export-dialog";
import type { DashboardSessionTabsState } from "../../hooks/use-dashboard-session-tabs-state";
import { getColorPaletteOptions } from "../../messages/color-palette-options";
import { getHeaderControlsMessages } from "../../messages/header-controls";
import { DashboardHeaderColorPaletteControls } from "../dashboard-header-color-palette-controls";
import { DashboardSessionControls } from "../dashboard-session-controls";

function inactiveSessionTabsState(): DashboardSessionTabsState {
  return {
    activeSession: undefined,
    isSessionStreamPaused: () => false,
    toggleSessionStreamPaused: () => {},
  } as DashboardSessionTabsState;
}

it("header controls expose localized icon-only actions and open export", () => {
  const locale = "ko";
  const headerMessages = getHeaderControlsMessages(locale);
  const exportMessages = getExportDialogMessages(locale);
  const onOpenExportDialog = mock(() => {});

  render(
    <>
      <DashboardHeaderColorPaletteControls
        locale={locale}
        onChangeLocale={() => {}}
      />
      <DashboardSessionControls
        isExportDialogOpen={false}
        locale={locale}
        onOpenExportDialog={onOpenExportDialog}
        sessionTabsState={inactiveSessionTabsState()}
      />
    </>,
  );

  const languageButton = screen.getByRole("button", {
    name: headerMessages.languageMenuButtonLabel,
  });
  const paletteButton = screen.getByRole("button", {
    name: headerMessages.paletteMenuButtonLabel,
  });
  const exportButton = screen.getByRole("button", {
    name: exportMessages.triggerLabel,
  });

  expect(languageButton.textContent).toBe("");
  expect(paletteButton.textContent).toBe("");
  expect(exportButton.textContent).toBe("");
  expect(exportButton.getAttribute("aria-haspopup")).toBe("dialog");
  expect(exportButton.getAttribute("aria-expanded")).toBe("false");

  fireEvent.click(exportButton);
  expect(onOpenExportDialog).toHaveBeenCalledTimes(1);
});

it("locale menu supports keyboard opening, focus, selection, and dismissal", async () => {
  const messages = getHeaderControlsMessages("en");
  const onChangeLocale = mock((_locale: string) => {});
  render(
    <DashboardHeaderColorPaletteControls
      locale="en"
      onChangeLocale={onChangeLocale}
    />,
  );

  const languageButton = screen.getByRole("button", {
    name: messages.languageMenuButtonLabel,
  });
  languageButton.focus();
  fireEvent.keyDown(languageButton, { key: "Enter" });

  const languageMenu = screen.getByRole("menu", {
    name: messages.languageLabel,
  });
  await waitFor(() => {
    expect(document.activeElement).toBe(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.en,
      }),
    );
  });
  expect(
    screen.getByRole("menuitemradio", {
      name: NATIVE_LANGUAGE_LABELS["zh-CN"],
    }),
  ).toBeTruthy();
  expect(
    screen.getByRole("menuitemradio", { name: NATIVE_LANGUAGE_LABELS.ja }),
  ).toBeTruthy();

  fireEvent.keyDown(languageMenu, { key: "ArrowDown" });
  expect(document.activeElement).toBe(
    screen.getByRole("menuitemradio", {
      name: NATIVE_LANGUAGE_LABELS["zh-CN"],
    }),
  );
  fireEvent.click(
    screen.getByRole("menuitemradio", { name: NATIVE_LANGUAGE_LABELS.ko }),
  );

  expect(onChangeLocale).toHaveBeenCalledWith("ko");
  expect(
    screen.queryByRole("menu", { name: messages.languageLabel }),
  ).toBeNull();
  expect(document.activeElement).toBe(languageButton);
});

it("palette menu applies and persists the selected palette", async () => {
  window.sessionStorage.clear();
  applyDocumentColorPalette("factory-dark");
  const messages = getHeaderControlsMessages("en");

  render(
    <AppColorPaletteProvider>
      <DashboardHeaderColorPaletteControls
        locale="en"
        onChangeLocale={() => {}}
      />
    </AppColorPaletteProvider>,
  );

  fireEvent.click(
    screen.getByRole("button", { name: messages.paletteMenuButtonLabel }),
  );
  expect(
    screen.getAllByRole("menuitemradio").map((item) => item.textContent),
  ).toEqual(getColorPaletteOptions("en").map((option) => option.label));

  fireEvent.click(screen.getByRole("menuitemradio", { name: "Slate" }));

  await waitFor(() => {
    expect(document.documentElement.dataset.colorPalette).toBe("slate");
  });
  expect(
    window.sessionStorage.getItem("infinite-you-dashboard-color-palette"),
  ).toBe("slate");
  expect(
    screen.queryByRole("menu", { name: messages.paletteLabel }),
  ).toBeNull();
});

it("palette menu supports keyboard navigation and outside dismissal", async () => {
  window.sessionStorage.clear();
  applyDocumentColorPalette("factory-dark");
  const messages = getHeaderControlsMessages("en");

  render(
    <AppColorPaletteProvider>
      <DashboardHeaderColorPaletteControls
        locale="en"
        onChangeLocale={() => {}}
      />
    </AppColorPaletteProvider>,
  );

  const paletteButton = screen.getByRole("button", {
    name: messages.paletteMenuButtonLabel,
  });
  fireEvent.keyDown(paletteButton, { key: "ArrowDown" });

  const paletteMenu = screen.getByRole("menu", {
    name: messages.paletteLabel,
  });
  const menuItems = screen.getAllByRole("menuitemradio");
  await waitFor(() => {
    expect(document.activeElement).toBe(menuItems[0]);
  });
  fireEvent.keyDown(paletteMenu, { key: "ArrowDown" });
  expect(document.activeElement).toBe(menuItems[1]);

  fireEvent.pointerDown(document.body);
  await waitFor(() => {
    expect(
      screen.queryByRole("menu", { name: messages.paletteLabel }),
    ).toBeNull();
  });
  expect(document.activeElement).toBe(paletteButton);
});
