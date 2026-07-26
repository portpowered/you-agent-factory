import type { ReactNode } from "react";
import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Label } from "@you-agent-factory/components/primitives";

export function WorkContentItemShell({
  children,
  headerActions,
  itemTypeLabel,
}: {
  children: ReactNode;
  headerActions?: ReactNode;
  itemTypeLabel: string;
}) {
  return (
    <SurfacePanel asChild className="grid gap-3" radius="lg">
      <li>
        <div className="flex items-start justify-between gap-3">
          <div className="grid gap-1">
            <Label>{itemTypeLabel}</Label>
          </div>
          {headerActions}
        </div>
        {children}
      </li>
    </SurfacePanel>
  );
}
