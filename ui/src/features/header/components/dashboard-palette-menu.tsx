import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";

import {
  type ColorPaletteId,
  type ColorPaletteOption,
  useAppColorPalette,
} from "../../../theme";
import { getColorPaletteOptions } from "../messages/color-palette-options";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";
import {
  DashboardHeaderOptionMenuItem,
  DashboardHeaderOptionMenuSurface,
} from "./dashboard-header-option-menu";

export function DashboardPaletteMenu({ locale }: { locale: string }) {
  const { palette, setPalette } = useAppColorPalette();
  const [isOpen, setIsOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuId = useId();
  const headerMessages = getHeaderControlsMessages(locale);
  const paletteOptions = getColorPaletteOptions(locale);

  useHeaderOptionMenuDismissal({
    buttonRef,
    isOpen,
    menuRef,
    onDismiss: () => {
      setIsOpen(false);
      buttonRef.current?.focus();
    },
  });
  useHeaderOptionMenuSelectionFocus({
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
        aria-label={headerMessages.paletteMenuButtonLabel}
        compact
        onClick={() => {
          setIsOpen((current) => !current);
        }}
        onKeyDown={(event) => {
          if (shouldOpenHeaderOptionMenuFromKey(event.key)) {
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
          <path d="M12 3c-1.5 2.2-4.5 6.8-4.5 10.5a4.5 4.5 0 1 0 9 0C16.5 9.8 13.5 5.2 12 3Z" />
        </svg>
      </DashboardHeaderActionButton>
      {isOpen ? (
        <DashboardPaletteMenuList
          buttonRef={buttonRef}
          currentValue={palette}
          id={menuId}
          label={headerMessages.paletteLabel}
          menuRef={menuRef}
          onChangePalette={setPalette}
          onClose={() => {
            setIsOpen(false);
            buttonRef.current?.focus();
          }}
          paletteOptions={paletteOptions}
        />
      ) : null}
    </div>
  );
}

function DashboardPaletteMenuList({
  buttonRef,
  currentValue,
  id,
  label,
  menuRef,
  onChangePalette,
  onClose,
  paletteOptions,
}: {
  buttonRef: RefObject<HTMLButtonElement | null>;
  currentValue: ColorPaletteId;
  id: string;
  label: string;
  menuRef: RefObject<HTMLDivElement | null>;
  onChangePalette: (palette: string) => void;
  onClose: () => void;
  paletteOptions: readonly ColorPaletteOption[];
}) {
  return (
    <DashboardHeaderOptionMenuSurface
      aria-label={label}
      id={id}
      minWidthClassName="min-w-52"
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          onClose();
          return;
        }

        moveHeaderOptionMenuFocus(event, menuRef);
      }}
      ref={menuRef}
      role="menu"
    >
      {paletteOptions.map((option) => {
        const isSelected = option.id === currentValue;

        return (
          <DashboardHeaderOptionMenuItem
            key={option.id}
            isSelected={isSelected}
            onClick={() => {
              onChangePalette(option.id);
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

function shouldOpenHeaderOptionMenuFromKey(key: string): boolean {
  return (
    key === "ArrowDown" || key === "ArrowUp" || key === "Enter" || key === " "
  );
}

function useHeaderOptionMenuDismissal({
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

function useHeaderOptionMenuSelectionFocus({
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

function moveHeaderOptionMenuFocus(
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
