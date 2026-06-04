import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";

import { DashboardActionButton } from "../../../components/ui";
import {
  getNativeLanguageLabel,
  SUPPORTED_LOCALES,
  type SupportedLocale,
} from "../../../i18n";
import { cn } from "../../../lib/cn";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";

interface HeaderLocaleOption {
  label: string;
  value: SupportedLocale;
}

interface DashboardLocaleMenuProps {
  locale: string;
  onChangeLocale: (locale: string) => void;
}

export function DashboardLocaleMenu({
  locale,
  onChangeLocale,
}: DashboardLocaleMenuProps) {
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
      className="absolute right-0 top-full z-10 mt-2 min-w-44 overflow-hidden rounded-2xl border border-outline bg-surface-container-high p-1 text-on-surface shadow-af-panel backdrop-blur-lg"
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
          <DashboardActionButton
            key={option.value}
            aria-checked={isSelected}
            className={cn(
              "min-h-0 w-full justify-start rounded-xl border-transparent px-3 py-2 text-sm",
              "[&>span]:grid [&>span]:w-full [&>span]:grid-cols-[minmax(0,1fr)_auto] [&>span]:items-center [&>span]:gap-2 [&>span]:text-left",
              isSelected
                ? "border-primary bg-primary-container text-on-surface"
                : "text-on-surface-variant",
            )}
            onClick={() => {
              onChangeLocale(option.value);
              onClose();
              buttonRef.current?.focus();
            }}
            role="menuitemradio"
            tone={isSelected ? "secondary" : "ghost"}
            type="button"
          >
            <span>{option.label}</span>
            {isSelected ? <LocaleMenuCheckIcon /> : null}
          </DashboardActionButton>
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

function resolveLanguageSwitcherValue(locale: string): SupportedLocale {
  return SUPPORTED_LOCALES.includes(locale as SupportedLocale)
    ? (locale as SupportedLocale)
    : "en";
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
