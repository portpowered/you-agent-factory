import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";

import { cn } from "../../../lib/cn";
import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import {
  DASHBOARD_PAGE_HEADING_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  DashboardActionRow,
  DashboardStatusPill,
} from "../../../components/ui";
import {
  getNativeLanguageLabel,
  SUPPORTED_LOCALES,
  type SupportedLocale,
  useAppLocale,
} from "../../../i18n";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import { getExportDialogMessages } from "../../export/messages/export-dialog";
import { useExportDialogStore } from "../../export/state/exportDialogStore";
import { DashboardBrandLockup } from "./dashboard-brand-lockup";
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";
import { DashboardSessionTabs } from "./dashboard-session-tabs";
import { TickSliderControl } from "./tick-slider-control";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { useDashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";

const DASHBOARD_TOOLBAR_CLASS = cn(
  DASHBOARD_PANEL_SHELL_CLASS,
  "mb-3 grid gap-2 p-2 md:px-3 md:py-2",
);
const DASHBOARD_HEADER_ROWS_CLASS = "flex min-w-0 flex-col gap-2";
const DASHBOARD_PRIMARY_ROW_CLASS = cn(
  "flex min-w-0 items-stretch gap-2",
  "max-md:flex-col",
);
const DASHBOARD_SECONDARY_ROW_CLASS = "flex min-w-0";
const DASHBOARD_BRAND_SLOT_CLASS = "min-w-0 self-end pb-2";
const DASHBOARD_TAB_STRIP_CLASS = "flex min-w-0 flex-1 items-stretch px-2 pt-1";
const DASHBOARD_TIMELINE_ROW_CLASS = "flex min-w-0 justify-end";
const DASHBOARD_TITLE_CLASS = cn("m-0 shrink-0", DASHBOARD_PAGE_HEADING_CLASS);
const DASHBOARD_CONTROLS_CLASS = "shrink-0 self-end pb-2";
const DASHBOARD_HEADER_ACTION_ROW_CLASS = "justify-end max-md:w-full";
const DASHBOARD_HEADER_ACTION_ROW_ACTIONS_CLASS =
  "max-md:w-full max-md:justify-end";
const LOCALE_MENU_PANEL_CLASS =
  "absolute right-0 top-full z-10 mt-2 min-w-44 overflow-hidden rounded-2xl border border-af-border bg-af-surface-raised p-1 text-af-text shadow-af-panel backdrop-blur-lg";
const LOCALE_MENU_ITEM_CLASS = cn(
  "flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-af-text-muted outline-none transition-colors hover:bg-af-overlay hover:text-af-text",
  "focus-visible:bg-af-overlay focus-visible:text-af-text focus-visible:ring-2 focus-visible:ring-af-focus-ring",
);

interface HeaderLocaleOption {
  label: string;
  value: SupportedLocale;
}

export interface DashboardHeaderProps {
  locale?: string;
}

export function DashboardHeader({ locale }: DashboardHeaderProps) {
  const { locale: resolvedLocale, setLocale } = useAppLocale(locale);
  const sessionTabsState = useDashboardSessionTabsState();
  const streamStatus = useDashboardStreamStore((state) => state.streamState.status);
  const isExportDialogOpen = useExportDialogStore(
    (state) => state.isExportDialogOpen,
  );
  const openExportDialog = useExportDialogStore(
    (state) => state.openExportDialog,
  );
  const exportMessages = getExportDialogMessages(resolvedLocale);
  const headerMessages = getHeaderControlsMessages(resolvedLocale);

  return (
    <section
      aria-label={headerMessages.dashboardSummaryLabel}
      className={DASHBOARD_TOOLBAR_CLASS}
    >
      <div className={DASHBOARD_HEADER_ROWS_CLASS}>
        <div className={DASHBOARD_PRIMARY_ROW_CLASS}>
          <h1 className={cn(DASHBOARD_TITLE_CLASS, DASHBOARD_BRAND_SLOT_CLASS)}>
            <DashboardBrandLockup
              locale={resolvedLocale}
              wordmarkClassName="truncate"
            />
          </h1>
          <div className={DASHBOARD_TAB_STRIP_CLASS}>
            <DashboardSessionTabs
              locale={resolvedLocale}
              state={sessionTabsState}
            />
          </div>
          <DashboardActionRow
            actions={
              <fieldset
                aria-label={headerMessages.globalHeaderActionsLabel}
                className={DASHBOARD_CONTROLS_CLASS}
              >
                <div className="flex items-center gap-2">
                  <DashboardHeaderActionButton
                    aria-label={exportMessages.triggerLabel}
                    aria-expanded={isExportDialogOpen}
                    aria-haspopup="dialog"
                    compact
                    onClick={openExportDialog}
                  >
                    <svg
                      aria-hidden="true"
                      fill="none"
                      height="18"
                      stroke="currentColor"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth="1.8"
                      viewBox="0 0 24 24"
                      width="18"
                    >
                      <path d="M14 5h5v5" />
                      <path d="M10 14 19 5" />
                      <path d="M19 13v5a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" />
                    </svg>
                  </DashboardHeaderActionButton>
                  <DashboardLocaleMenu
                    locale={resolvedLocale}
                    onChangeLocale={setLocale}
                  />
                </div>
              </fieldset>
            }
            actionsClassName={DASHBOARD_HEADER_ACTION_ROW_ACTIONS_CLASS}
            className={DASHBOARD_HEADER_ACTION_ROW_CLASS}
            statuses={
              <DashboardStatusPill
                aria-label={streamStatusLabel(streamStatus, headerMessages)}
                role="status"
                tone={streamStatusTone(streamStatus)}
              >
                {streamStatusLabel(streamStatus, headerMessages)}
              </DashboardStatusPill>
            }
          />
        </div>
        <div className={DASHBOARD_SECONDARY_ROW_CLASS}>
          <div className={DASHBOARD_TIMELINE_ROW_CLASS}>
            <TickSliderControl locale={resolvedLocale} />
          </div>
        </div>
      </div>
    </section>
  );
}

function streamStatusLabel(
  status: ReturnType<typeof useDashboardStreamStore.getState>["streamState"]["status"],
  messages: ReturnType<typeof getHeaderControlsMessages>,
) {
  if (status === "live") {
    return messages.streamStatusLiveLabel;
  }
  if (status === "offline") {
    return messages.streamStatusOfflineLabel;
  }

  return messages.streamStatusConnectingLabel;
}

function streamStatusTone(
  status: ReturnType<typeof useDashboardStreamStore.getState>["streamState"]["status"],
) {
  if (status === "live") {
    return "active";
  }
  if (status === "offline") {
    return "danger";
  }

  return "neutral";
}

function resolveLanguageSwitcherValue(locale: string): SupportedLocale {
  return SUPPORTED_LOCALES.includes(locale as SupportedLocale)
    ? (locale as SupportedLocale)
    : "en";
}

function DashboardLocaleMenu({
  locale,
  onChangeLocale,
}: {
  locale: string;
  onChangeLocale: (locale: string) => void;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuId = useId();
  const headerMessages = getHeaderControlsMessages(locale);
  const localeOptions = createHeaderLocaleOptions();
  const resolvedValue = resolveLanguageSwitcherValue(locale);
  useLocaleMenuDismissal({
    buttonRef,
    isOpen,
    menuRef,
    onDismiss: () => {
      setIsOpen(false);
      buttonRef.current?.focus();
    },
  });
  useLocaleMenuSelectionFocus({
    isOpen,
    menuRef,
  });

  return (
    <div className="relative shrink-0">
      <DashboardHeaderActionButton
        ref={buttonRef}
        aria-controls={menuId}
        aria-expanded={isOpen}
        aria-haspopup="menu"
        aria-label={headerMessages.languageMenuButtonLabel}
        compact
        onClick={() => {
          setIsOpen((current) => !current);
        }}
        onKeyDown={(event) => {
          if (shouldOpenLocaleMenuFromKey(event.key)) {
            event.preventDefault();
            setIsOpen(true);
          }
        }}
      >
        <svg
          aria-hidden="true"
          fill="none"
          height="18"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="1.8"
          viewBox="0 0 24 24"
          width="18"
        >
          <path d="M4 6h16" />
          <path d="M8.5 6c.9 3.8 3.2 7.3 6.5 10" />
          <path d="M11 13c-1.7 1.8-3.9 3.3-6.5 4" />
          <path d="M15 4v2" />
          <path d="M17.5 14 21 22" />
          <path d="m14 22 3.5-8 3.5 8" />
        </svg>
      </DashboardHeaderActionButton>
      {isOpen ? (
        <DashboardLocaleMenuList
          buttonRef={buttonRef}
          currentValue={resolvedValue}
          id={menuId}
          label={headerMessages.languageLabel}
          menuRef={menuRef}
          onChangeLocale={onChangeLocale}
          onClose={() => {
            setIsOpen(false);
            buttonRef.current?.focus();
          }}
          options={localeOptions}
        />
      ) : null}
    </div>
  );
}

function DashboardLocaleMenuList({
  buttonRef,
  currentValue,
  id,
  label,
  menuRef,
  onChangeLocale,
  onClose,
  options,
}: {
  buttonRef: RefObject<HTMLButtonElement | null>;
  currentValue: SupportedLocale;
  id: string;
  label: string;
  menuRef: RefObject<HTMLDivElement | null>;
  onChangeLocale: (locale: string) => void;
  onClose: () => void;
  options: readonly HeaderLocaleOption[];
}) {
  return (
    <div
      aria-label={label}
      className={LOCALE_MENU_PANEL_CLASS}
      id={id}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          onClose();
          return;
        }

        moveLocaleMenuFocus(event, menuRef);
      }}
      ref={menuRef}
      role="menu"
    >
      {options.map((option) => {
        const isSelected = option.value === currentValue;

        return (
          <button
            key={option.value}
            aria-checked={isSelected}
            className={cn(
              LOCALE_MENU_ITEM_CLASS,
              isSelected &&
                "border border-af-accent-border bg-af-accent-surface text-af-text",
            )}
            onClick={() => {
              onChangeLocale(option.value);
              onClose();
              buttonRef.current?.focus();
            }}
            role="menuitemradio"
            type="button"
          >
            <span>{option.label}</span>
            {isSelected ? <LocaleMenuCheckIcon /> : null}
          </button>
        );
      })}
    </div>
  );
}

function LocaleMenuCheckIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      width="16"
    >
      <path d="m5 12 4 4L19 6" />
    </svg>
  );
}

function createHeaderLocaleOptions(): readonly HeaderLocaleOption[] {
  return SUPPORTED_LOCALES.map((value) => ({
    label: getNativeLanguageLabel(value),
    value,
  }));
}

function shouldOpenLocaleMenuFromKey(key: string): boolean {
  return (
    key === "ArrowDown" || key === "ArrowUp" || key === "Enter" || key === " "
  );
}

function useLocaleMenuDismissal({
  buttonRef,
  isOpen,
  menuRef,
  onDismiss,
}: {
  buttonRef: RefObject<HTMLButtonElement | null>;
  isOpen: boolean;
  menuRef: RefObject<HTMLDivElement | null>;
  onDismiss: () => void;
}) {
  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) {
        return;
      }
      if (
        buttonRef.current?.contains(target) ||
        menuRef.current?.contains(target)
      ) {
        return;
      }

      onDismiss();
    };

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onDismiss();
      }
    };

    window.addEventListener("pointerdown", handlePointerDown);
    window.addEventListener("keydown", handleEscape);

    return () => {
      window.removeEventListener("pointerdown", handlePointerDown);
      window.removeEventListener("keydown", handleEscape);
    };
  }, [buttonRef, isOpen, menuRef, onDismiss]);
}

function useLocaleMenuSelectionFocus({
  isOpen,
  menuRef,
}: {
  isOpen: boolean;
  menuRef: RefObject<HTMLDivElement | null>;
}) {
  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const selectedItem = menuRef.current?.querySelector<HTMLButtonElement>(
      '[role="menuitemradio"][aria-checked="true"]',
    );
    selectedItem?.focus();
  }, [isOpen, menuRef]);
}

function moveLocaleMenuFocus(
  event: ReactKeyboardEvent<HTMLDivElement>,
  menuRef: RefObject<HTMLDivElement | null>,
) {
  if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
    return;
  }

  const items = Array.from(
    menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]') ??
      [],
  );
  if (items.length === 0) {
    return;
  }

  const currentIndex = items.indexOf(
    document.activeElement as HTMLButtonElement,
  );
  const direction = event.key === "ArrowDown" ? 1 : -1;
  const nextIndex =
    currentIndex === -1
      ? 0
      : (currentIndex + direction + items.length) % items.length;

  event.preventDefault();
  items[nextIndex]?.focus();
}
