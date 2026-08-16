import {
  Button,
  type ButtonProps,
} from "@you-agent-factory/components/primitives";
import { Maximize2, Move } from "lucide-react";
import {
  createContext,
  type KeyboardEvent,
  useContext,
  useId,
  useState,
} from "react";
import type { Layout } from "react-grid-layout";
import { cloneLayout } from "react-grid-layout/core";
import {
  applyDashboardKeyboardLayoutOperation,
  type DashboardKeyboardLayoutMode,
} from "../../hooks/keyboard/dashboardLayoutKeyboard";
import type { AgentBentoMessages } from "../../messages/agent-bento";

export interface DashboardLayoutKeyboardContextValue {
  columns: number;
  enabled: boolean;
  itemID: string;
  layout: Layout;
  messages: AgentBentoMessages;
  onCommitLayout: (layout: Layout) => void;
  onPreviewLayout: (layout: Layout) => void;
}

export const DashboardLayoutKeyboardContext =
  createContext<DashboardLayoutKeyboardContextValue | null>(null);

interface DashboardLayoutKeyboardControlsProps {
  title: string;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: keyboard mode owns one focused draft, announcement, and commit/cancel lifecycle.
export function DashboardLayoutKeyboardControls({
  title,
}: DashboardLayoutKeyboardControlsProps) {
  const context = useContext(DashboardLayoutKeyboardContext);
  const moveInstructionsID = useId();
  const resizeInstructionsID = useId();
  const [draftLayout, setDraftLayout] = useState<Layout | null>(null);
  const [baseLayout, setBaseLayout] = useState<Layout | null>(null);
  const [mode, setMode] = useState<DashboardKeyboardLayoutMode | null>(null);
  const [announcement, setAnnouncement] = useState("");

  if (!context?.enabled) {
    return null;
  }

  const keyboardContext = context;

  const activeLayout = draftLayout ?? keyboardContext.layout;
  function beginEditing(nextMode: DashboardKeyboardLayoutMode) {
    const initialLayout = cloneLayout(keyboardContext.layout);
    setBaseLayout(initialLayout);
    setDraftLayout(initialLayout);
    setMode(nextMode);
    setAnnouncement(
      keyboardContext.messages.layoutEditStarted(title, nextMode),
    );
  }

  function commitEditing() {
    if (!mode || !draftLayout || !baseLayout) {
      return;
    }

    const nextItem = draftLayout.find(
      (item) => item.i === keyboardContext.itemID,
    );
    const baseItem = baseLayout.find(
      (item) => item.i === keyboardContext.itemID,
    );
    const changed =
      nextItem !== undefined &&
      baseItem !== undefined &&
      !hasSameGeometry(nextItem, baseItem);

    if (changed) {
      keyboardContext.onCommitLayout(draftLayout);
      setAnnouncement(keyboardContext.messages.layoutChanged(title, nextItem));
    } else {
      setAnnouncement(keyboardContext.messages.layoutEditCancelled(title));
    }

    resetEditing();
  }

  function cancelEditing() {
    if (baseLayout) {
      keyboardContext.onPreviewLayout(cloneLayout(baseLayout));
    }
    setAnnouncement(keyboardContext.messages.layoutEditCancelled(title));
    resetEditing();
  }

  function handleKeyDown(
    event: KeyboardEvent<HTMLButtonElement>,
    buttonMode: DashboardKeyboardLayoutMode,
  ) {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      event.stopPropagation();
      if (mode === buttonMode) {
        commitEditing();
      } else {
        beginEditing(buttonMode);
      }
      return;
    }

    if (!mode || mode !== buttonMode) {
      return;
    }

    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      cancelEditing();
      return;
    }

    if (!isArrowKey(event.key)) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    const result = applyDashboardKeyboardLayoutOperation(
      activeLayout,
      keyboardContext.itemID,
      mode,
      event.key,
      keyboardContext.columns,
    );
    if (!result.changed) {
      setAnnouncement(keyboardContext.messages.layoutBoundary(title, mode));
      return;
    }

    setDraftLayout(result.layout);
    keyboardContext.onPreviewLayout(result.layout);
    const nextItem = result.layout.find(
      (item) => item.i === keyboardContext.itemID,
    );
    if (nextItem) {
      setAnnouncement(keyboardContext.messages.layoutChanged(title, nextItem));
    }
  }

  function renderModeButton(
    nextMode: DashboardKeyboardLayoutMode,
    icon: ButtonProps["children"],
    label: string,
  ) {
    const isActive = mode === nextMode;
    return (
      <Button
        aria-describedby={
          nextMode === "resize" ? resizeInstructionsID : moveInstructionsID
        }
        aria-keyshortcuts="ArrowUp ArrowDown ArrowLeft ArrowRight Enter Escape"
        aria-label={label}
        aria-pressed={isActive}
        className="shrink-0"
        data-bento-layout-keyboard-mode={nextMode}
        onClick={() => {
          if (isActive) {
            commitEditing();
            return;
          }
          beginEditing(nextMode);
        }}
        onKeyDown={(event) => handleKeyDown(event, nextMode)}
        size="icon"
        tone={isActive ? "secondary" : "ghost"}
        type="button"
      >
        {icon}
      </Button>
    );
  }

  function resetEditing() {
    setMode(null);
    setDraftLayout(null);
    setBaseLayout(null);
  }

  return (
    <>
      <span className="sr-only" id={moveInstructionsID}>
        {keyboardContext.messages.moveInstructions}
      </span>
      <span className="sr-only" id={resizeInstructionsID}>
        {keyboardContext.messages.resizeInstructions}
      </span>
      <span
        aria-live="polite"
        aria-atomic="true"
        className="sr-only"
        role="status"
      >
        {announcement}
      </span>
      {renderModeButton(
        "move",
        <Move aria-hidden="true" focusable="false" size={16} />,
        keyboardContext.messages.moveWidgetLabel(title),
      )}
      {renderModeButton(
        "resize",
        <Maximize2 aria-hidden="true" focusable="false" size={16} />,
        keyboardContext.messages.resizeWidgetLabel(title),
      )}
    </>
  );
}

function hasSameGeometry(
  left: Pick<Layout[number], "h" | "w" | "x" | "y">,
  right: Pick<Layout[number], "h" | "w" | "x" | "y">,
): boolean {
  return (
    left.h === right.h &&
    left.w === right.w &&
    left.x === right.x &&
    left.y === right.y
  );
}

function isArrowKey(key: string): boolean {
  return (
    key === "ArrowDown" ||
    key === "ArrowLeft" ||
    key === "ArrowRight" ||
    key === "ArrowUp"
  );
}
