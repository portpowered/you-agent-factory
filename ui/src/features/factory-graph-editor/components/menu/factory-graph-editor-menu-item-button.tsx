import {
  DashboardActionButton,
  type DashboardActionButtonProps,
} from "../../../../components/ui/dashboard-action-button";
import { cn } from "../../../../lib/cn";

const FACTORY_GRAPH_EDITOR_MENU_ITEM_BUTTON_CLASS =
  "min-h-0 w-full justify-start rounded-2xl border-transparent px-3 py-2 text-left [&>span]:grid [&>span]:w-full [&>span]:justify-items-start";

export function FactoryGraphEditorMenuItemButton({
  children,
  className,
  tone = "ghost",
  ...props
}: DashboardActionButtonProps) {
  return (
    <DashboardActionButton
      className={cn(FACTORY_GRAPH_EDITOR_MENU_ITEM_BUTTON_CLASS, className)}
      tone={tone}
      {...props}
    >
      {children}
    </DashboardActionButton>
  );
}
