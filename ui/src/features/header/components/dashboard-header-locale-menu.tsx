import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";

import {
  getNativeLanguageLabel,
  SUPPORTED_LOCALES,
  type SupportedLocale,
} from "../../../i18n";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";
import {
  DashboardHeaderOptionMenuItem,
  DashboardHeaderOptionMenuSurface,
} from "./dashboard-header-option-menu";

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
    <DashboardHeaderOptionMenuSurface
      aria-label={label}
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
          <DashboardHeaderOptionMenuItem
            key={option.value}
            isSelected={isSelected}
            onClick={() => {
              onChangeLocale(option.value);
              onClose();
              buttonRef.current?.focus();
            }}
          >
            <span>{option.label}</span>
          </DashboardHeaderOptionMenuItem>
        );
      })}
    </DashboardHeaderOptionMenuSurface>
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
