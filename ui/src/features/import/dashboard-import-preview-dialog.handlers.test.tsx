import type { ButtonHTMLAttributes, ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";

const dialogEventState = {
  escapePrevented: false,
  outsidePrevented: false,
};

vi.mock("../../components/ui", () => ({
  Button: ({
    children,
    ...props
  }: ButtonHTMLAttributes<HTMLButtonElement>) => <button {...props}>{children}</button>,
  Dialog: ({
    children,
    onOpenChange,
  }: {
    children: ReactNode;
    onOpenChange?: (open: boolean) => void;
    open?: boolean;
  }) => (
    <div>
      {children}
      <button onClick={() => onOpenChange?.(false)} type="button">
        Request close
      </button>
    </div>
  ),
  DialogContent: ({
    children,
    onEscapeKeyDown,
    onInteractOutside,
  }: {
    children: ReactNode;
    onEscapeKeyDown?: (event: { preventDefault: () => void }) => void;
    onInteractOutside?: (event: { preventDefault: () => void }) => void;
  }) => (
    <div aria-label="Review factory import" role="dialog">
      {children}
      <button
        onClick={() =>
          onEscapeKeyDown?.({
            preventDefault: () => {
              dialogEventState.escapePrevented = true;
            },
          })
        }
        type="button"
      >
        Trigger escape
      </button>
      <button
        onClick={() =>
          onInteractOutside?.({
            preventDefault: () => {
              dialogEventState.outsidePrevented = true;
            },
          })
        }
        type="button"
      >
        Trigger outside
      </button>
    </div>
  ),
  DialogDescription: ({ children }: { children: ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));

import { FactoryImportPreviewDialog } from "./dashboard-import-preview-dialog";

function createReadyPreviewState() {
  return {
    file: new File(["png"], "factory-import.png", { type: "image/png" }),
    status: "ready" as const,
    value: {
      factory: {
        name: "Dropped Factory",
        workTypes: [],
        workers: [],
        workstations: [],
      },
      previewImageSrc: "blob:factory-preview",
      revokePreviewImageSrc: vi.fn(),
      schemaVersion: "portos.agent-factory.png.v1",
    },
  };
}

describe("FactoryImportPreviewDialog close guards", () => {
  beforeEach(() => {
    dialogEventState.escapePrevented = false;
    dialogEventState.outsidePrevented = false;
  });

  it("allows dialog close requests and passive escape or outside events while idle", () => {
    const onCancel = vi.fn();

    render(
      <FactoryImportPreviewDialog
        activationState={{ status: "idle" }}
        onCancel={onCancel}
        onConfirm={vi.fn()}
        previewState={createReadyPreviewState()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Trigger escape" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger outside" }));
    fireEvent.click(screen.getByRole("button", { name: "Request close" }));

    expect(dialogEventState.escapePrevented).toBe(false);
    expect(dialogEventState.outsidePrevented).toBe(false);
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("prevents escape, outside, and close requests while activation is submitting", () => {
    const onCancel = vi.fn();

    render(
      <FactoryImportPreviewDialog
        activationState={{ status: "submitting" }}
        onCancel={onCancel}
        onConfirm={vi.fn()}
        previewState={createReadyPreviewState()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Trigger escape" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger outside" }));
    fireEvent.click(screen.getByRole("button", { name: "Request close" }));

    expect(dialogEventState.escapePrevented).toBe(true);
    expect(dialogEventState.outsidePrevented).toBe(true);
    expect(onCancel).not.toHaveBeenCalled();
  });
});
