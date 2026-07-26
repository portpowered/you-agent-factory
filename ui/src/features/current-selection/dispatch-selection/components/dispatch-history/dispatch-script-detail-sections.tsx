import { CodePanel } from "@you-agent-factory/components/data-display";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { CurrentSelectionDetailCode } from "../../../base/components/detail/current-selection-detail-item";
import { CurrentSelectionLabel } from "../../../base/components/presentation/current-selection-label";

export function ScriptArgsSection({
  args,
  label,
}: {
  args: string[] | undefined;
  label: string;
}) {
  if (!args || args.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      <div className="grid gap-1">
        {args.map((arg) => (
          <CurrentSelectionDetailCode key={arg}>
            {arg}
          </CurrentSelectionDetailCode>
        ))}
      </div>
    </div>
  );
}

export function ScriptOutputSection({
  emptyMessage,
  label,
  value,
}: {
  emptyMessage: string;
  label: string;
  value: string | undefined;
}) {
  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      {value ? (
        <CodePanel>{value}</CodePanel>
      ) : (
        <WidgetDetailCopy>{emptyMessage}</WidgetDetailCopy>
      )}
    </div>
  );
}
