import { useId } from "react";

import { cn } from "../../../../lib/cn";
import {
  FACTORY_LAYOUT_GROUP_COLOR_TOKENS,
  type FactoryLayoutGroup,
  type FactoryLayoutGroupColorToken,
  factoryLayoutGroupColorCssVariable,
  isApprovedFactoryLayoutGroupColor,
} from "../../lib/layout/factory-graph-layout-groups";

export function FactoryGraphVisualGroupControls({
  colorLabel,
  colorOptionLabel,
  emptyLabelError,
  group,
  labelFieldLabel,
  onRenameGroup,
  onSetGroupColor,
  selectedGroupLabel,
}: {
  colorLabel: string;
  colorOptionLabel: (token: FactoryLayoutGroupColorToken) => string;
  emptyLabelError: string;
  group: FactoryLayoutGroup;
  labelFieldLabel: string;
  onRenameGroup: (label: string) => void;
  onSetGroupColor: (color: FactoryLayoutGroupColorToken) => void;
  selectedGroupLabel: string;
}) {
  const labelFieldId = useId();
  const trimmedLabel = group.label?.trim() ?? "";
  const labelError = trimmedLabel.length === 0 ? emptyLabelError : null;
  const selectedColor = isApprovedFactoryLayoutGroupColor(group.color)
    ? group.color
    : "primary";

  return (
    <section
      aria-label={selectedGroupLabel}
      className="absolute bottom-4 right-4 z-20 max-w-sm rounded-xl border border-outline bg-surface-container-high p-3 shadow-sm"
      data-factory-visual-group-controls=""
    >
      <p className="m-0 text-xs font-semibold text-on-surface-subtle">
        {selectedGroupLabel}
      </p>
      <div className="mt-3 grid gap-2">
        <label className="grid gap-1 text-sm text-on-surface" htmlFor={labelFieldId}>
          <span className="text-on-surface-subtle">{labelFieldLabel}</span>
          <input
            aria-invalid={labelError !== null}
            className={cn(
              "rounded-lg border bg-surface px-3 py-2 text-sm text-on-surface",
              labelError
                ? "border-af-danger-border"
                : "border-outline focus-visible:border-primary",
            )}
            id={labelFieldId}
            onChange={(event) => onRenameGroup(event.target.value)}
            value={group.label ?? ""}
          />
        </label>
        {labelError ? (
          <p className="m-0 text-xs text-on-error-container" role="alert">
            {labelError}
          </p>
        ) : null}
      </div>
      <fieldset className="mt-3 grid gap-2 border-0 p-0">
        <legend className="text-sm text-on-surface-subtle">{colorLabel}</legend>
        <div className="flex flex-wrap gap-2">
          {FACTORY_LAYOUT_GROUP_COLOR_TOKENS.map((token) => (
            <button
              aria-label={colorOptionLabel(token)}
              aria-pressed={selectedColor === token}
              className={cn(
                "h-8 w-8 rounded-full border-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary",
                selectedColor === token
                  ? "border-on-surface ring-2 ring-af-overlay-focus"
                  : "border-outline",
              )}
              data-factory-visual-group-color={token}
              key={token}
              onClick={() => onSetGroupColor(token)}
              style={{
                backgroundColor: factoryLayoutGroupColorCssVariable(token),
              }}
              type="button"
            />
          ))}
        </div>
      </fieldset>
    </section>
  );
}
