import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import { cn } from "../../../lib/cn";
import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  getNativeLanguageLabel,
  SUPPORTED_LOCALES,
  type SupportedLocale,
  useAppLocale,
} from "../../../i18n";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import { getExportDialogMessages } from "../../export/messages/export-dialog";
import { useExportDialogStore } from "../../export/state/exportDialogStore";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { DashboardBrandLockup } from "./dashboard-brand-lockup";
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";
import { TickSliderControl } from "./tick-slider-control";
import { getHeaderControlsMessages } from "../messages/header-controls";

const DASHBOARD_TOOLBAR_CLASS = cn(
  DASHBOARD_PANEL_SHELL_CLASS,
  "mb-4 flex flex-wrap items-center gap-3 p-3 md:px-4 md:py-3",
);
const DASHBOARD_TITLE_CLASS = cn("m-0 shrink-0", DASHBOARD_PAGE_HEADING_CLASS);
const DASHBOARD_CONTROLS_CLASS = cn(
  "ml-auto flex min-w-0 flex-1 flex-wrap items-center justify-end gap-3",
  "max-md:ml-0 max-md:w-full max-md:justify-stretch",
);
const STREAM_STATUS_SHELL_CLASS = cn(
  "flex shrink-0 items-center justify-end",
  "max-md:justify-start",
);
const STREAM_STATUS_CLASS = cn(
  "inline-flex h-10 w-10 items-center justify-center rounded-full border border-af-overlay/12 bg-af-overlay/4",
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const LOCALE_MENU_PANEL_CLASS =
  "absolute right-0 top-full z-10 mt-2 min-w-44 overflow-hidden rounded-2xl border border-af-overlay/12 bg-af-surface/96 p-1 shadow-af-panel backdrop-blur-lg";
const LOCALE_MENU_ITEM_CLASS = cn(
  "flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-af-ink/82 outline-none transition-colors",
  "focus-visible:bg-af-overlay/8 focus-visible:ring-2 focus-visible:ring-af-accent/25",
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
  const snapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick],
  );
  const streamState = useDashboardStreamStore((state) => state.streamState);
  const isExportDialogOpen = useExportDialogStore(
    (state) => state.isExportDialogOpen,
  );
  const openExportDialog = useExportDialogStore(
    (state) => state.openExportDialog,
  );
  const exportMessages = getExportDialogMessages(resolvedLocale);
  const headerMessages = getHeaderControlsMessages(resolvedLocale);

  if (!snapshot) {
    return null;
  }

  return (
    <section
      className={DASHBOARD_TOOLBAR_CLASS}
      aria-label={headerMessages.dashboardSummaryLabel}
    >
      <h1 className={DASHBOARD_TITLE_CLASS}>
        <DashboardBrandLockup
          locale={resolvedLocale}
          wordmarkClassName="truncate"
        />
      </h1>
      <div className={DASHBOARD_CONTROLS_CLASS}>
        <TickSliderControl locale={resolvedLocale} />
        <DashboardLocaleMenu
          locale={resolvedLocale}
          onChangeLocale={setLocale}
        />
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
        <div className={STREAM_STATUS_SHELL_CLASS}>
          <div
            aria-label={streamStatusLabel(streamState.status, resolvedLocale)}
            className={streamStatusClassName(streamState.status)}
            role="status"
          >
            <StreamStatusIcon status={streamState.status} />
          </div>
        </div>
      </div>
    </section>
  );
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
        onKeyDown={(event) => {
          if (shouldOpenLocaleMenuFromKey(event.key)) {
            event.preventDefault();
            setIsOpen(true);
          }
        }}
        onClick={() => {
          setIsOpen((current) => !current);
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
            aria-checked={isSelected}
            className={cn(
              LOCALE_MENU_ITEM_CLASS,
              isSelected && "bg-af-accent/10 text-af-accent",
            )}
            key={option.value}
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
      viewBox="0 0 16 16"
      width="16"
    >
      <path d="m3.5 8.5 2.5 2.5 6-6" />
    </svg>
  );
}

function createHeaderLocaleOptions(): readonly HeaderLocaleOption[] {
  return SUPPORTED_LOCALES.map((locale) => ({
    label: getNativeLanguageLabel(locale),
    value: locale,
  }));
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

function shouldOpenLocaleMenuFromKey(key: string): boolean {
  return (
    key === "ArrowDown" || key === "ArrowUp" || key === "Enter" || key === " "
  );
}

function moveLocaleMenuFocus(
  event: ReactKeyboardEvent<HTMLDivElement>,
  menuRef: RefObject<HTMLDivElement | null>,
) {
  if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
    return;
  }

  const items = Array.from(
    menuRef.current?.querySelectorAll<HTMLButtonElement>(
      '[role="menuitemradio"]',
    ) ?? [],
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

function streamStatusClassName(status: DashboardStreamState["status"]): string {
  return cn(
    STREAM_STATUS_CLASS,
    status === "live" &&
      "border-af-success/30 bg-af-success/16 text-af-success-ink",
    status === "connecting" &&
      "border-af-accent/30 bg-af-accent/12 text-af-accent",
    status === "offline" &&
      "border-af-danger/30 bg-af-danger/12 text-af-danger-ink",
  );
}

function streamStatusLabel(
  status: DashboardStreamState["status"],
  locale?: string,
): string {
  const messages = getHeaderControlsMessages(locale);

  if (status === "live") {
    return messages.streamStatusLiveLabel;
  }
  if (status === "offline") {
    return messages.streamStatusOfflineLabel;
  }

  return messages.streamStatusConnectingLabel;
}

function StreamStatusIcon({
  status,
}: {
  status: DashboardStreamState["status"];
}) {
  if (status === "live") {
    return (
      <span
        aria-hidden="true"
        className="relative inline-flex size-3.5 items-center justify-center"
      >
        <span className="absolute inline-flex size-full animate-ping rounded-full bg-current opacity-35" />
        <span className="relative inline-flex size-2.5 rounded-full bg-current" />
      </span>
    );
  }

  if (status === "offline") {
    return (
      <svg
        aria-hidden="true"
        fill="none"
        height="16"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
        viewBox="0 0 16 16"
        width="16"
      >
        <circle cx="8" cy="8" r="4.25" />
        <path d="M4.75 11.25 11.25 4.75" />
      </svg>
    );
  }

  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 16 16"
      width="16"
    >
      <circle cx="8" cy="8" r="4.25" strokeDasharray="1.6 2.2" />
      <path d="M8 5v3" />
      <circle cx="8" cy="11" r="0.75" fill="currentColor" stroke="none" />
    </svg>
  );
}
