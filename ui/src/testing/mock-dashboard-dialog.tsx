import { Button } from "@you-agent-factory/components/primitives";
import { createContext, type ReactNode, useContext } from "react";

const MockDialogContext = createContext<{
  onOpenChange?: (open: boolean) => void;
}>({});

export function Dialog({
  children,
  onOpenChange,
  open,
}: {
  children: ReactNode;
  onOpenChange?: (open: boolean) => void;
  open?: boolean;
}) {
  if (!open) {
    return null;
  }

  return (
    <MockDialogContext.Provider value={{ onOpenChange }}>
      {children}
    </MockDialogContext.Provider>
  );
}

export function DialogContent({
  children,
  closeDisabled = false,
  closeLabel = "Close",
}: {
  children: ReactNode;
  closeDisabled?: boolean;
  closeLabel?: string;
}) {
  const { onOpenChange } = useContext(MockDialogContext);

  return (
    <div aria-labelledby="mock-dashboard-dialog-title" role="dialog">
      {children}
      <Button
        aria-label={closeLabel}
        disabled={closeDisabled}
        onClick={() => {
          if (!closeDisabled) {
            onOpenChange?.(false);
          }
        }}
        size="icon"
        tone="ghost"
        type="button"
      >
        {closeLabel}
      </Button>
    </div>
  );
}

export function DialogDescription({ children }: { children: ReactNode }) {
  return <p>{children}</p>;
}

export function DialogFooter({ children }: { children: ReactNode }) {
  return <div>{children}</div>;
}

export function DialogHeader({ children }: { children: ReactNode }) {
  return <div>{children}</div>;
}

export function DialogTitle({ children }: { children: ReactNode }) {
  return <h2 id="mock-dashboard-dialog-title">{children}</h2>;
}

export function DialogOverlay() {
  return null;
}

export function DialogPortal({ children }: { children: ReactNode }) {
  return children;
}
