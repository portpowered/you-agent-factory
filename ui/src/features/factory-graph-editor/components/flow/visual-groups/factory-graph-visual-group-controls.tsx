import { useId } from "react";

import { Checkbox } from "../../../../../components/ui/checkbox";
import { DashboardActionButton } from "../../../../../components/ui/dashboard-action-button";
import { cn } from "../../../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../../lib/draft/factory-graph-draft-types";
import {
  FACTORY_LAYOUT_GROUP_COLOR_TOKENS,
  type FactoryLayoutGroup,
  type FactoryLayoutGroupCanvasNodeOption,
  type FactoryLayoutGroupColorToken,
  factoryLayoutGroupColorCssVariable,
  isApprovedFactoryLayoutGroupColor,
  normalizeFactoryLayoutGroupCustomColor,
} from "../../../lib/layout/visual-groups/factory-graph-layout-groups";

const DEFAULT_FACTORY_LAYOUT_GROUP_CUSTOM_COLOR = "#808080";

export function FactoryGraphVisualGroupControls({
  canvasNodeOptions,
  colorLabel,
  colorOptionLabel,
  customColorLabel = colorLabel,
  deleteGroupLabel,
  fitGroupLabel,
  boundsError,
  emptyLabelError,
  group,
  isNodeMember,
  labelFieldLabel,
  membershipNodeKindLabel,
  membershipEmptyLabel,
  membershipLabel,
  membershipNodeLabel,
  membershipStaleNodeLabel,
  onDeleteGroup,
  onFitGroup,
  onRenameGroup,
  onSetGroupColor,
  onToggleNodeMembership,
  groupAriaLabel,
  staleMemberNodeIds,
}: {
  canvasNodeOptions: readonly FactoryLayoutGroupCanvasNodeOption[];
  colorLabel: string;
  colorOptionLabel: (token: FactoryLayoutGroupColorToken) => string;
  customColorLabel?: string;
  deleteGroupLabel: string;
  fitGroupLabel?: string;
  boundsError: string | null;
  emptyLabelError: string;
  group: FactoryLayoutGroup;
  isNodeMember: (nodeId: string) => boolean;
  labelFieldLabel: string;
  membershipNodeKindLabel: (kind: FactoryGraphNodeKind) => string;
  membershipEmptyLabel: string;
  membershipLabel: string;
  membershipNodeLabel: (label: string) => string;
  membershipStaleNodeLabel: (nodeId: string) => string;
  onDeleteGroup: () => void;
  onFitGroup?: () => void;
  onRenameGroup: (label: string) => void;
  onSetGroupColor: (color: string) => void;
  onToggleNodeMembership: (nodeId: string, selected: boolean) => void;
  groupAriaLabel: string;
  staleMemberNodeIds: readonly string[];
}) {
  const labelFieldId = useId();
  const trimmedLabel = group.label?.trim() ?? "";
  const labelError = trimmedLabel.length === 0 ? emptyLabelError : null;

  return (
    <section
      aria-label={groupAriaLabel}
      // tailwind-exception: intrinsic-sizing
      className="absolute bottom-4 right-4 z-20 max-h-[calc(100%-2rem)] max-w-sm overflow-y-auto rounded-xl border border-outline bg-surface-container-high p-3 shadow-sm"
      data-factory-visual-group-controls=""
    >
      <div className="grid gap-2">
        <label
          className="grid gap-1 text-sm text-on-surface"
          htmlFor={labelFieldId}
        >
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
        {boundsError ? (
          <p className="m-0 text-xs text-on-error-container" role="alert">
            {boundsError}
          </p>
        ) : null}
        {fitGroupLabel && onFitGroup ? (
          <DashboardActionButton
            className="w-full"
            data-factory-visual-group-fit=""
            onClick={onFitGroup}
            tone="outline"
            type="button"
          >
            {fitGroupLabel}
          </DashboardActionButton>
        ) : null}
      </div>
      <FactoryGraphVisualGroupColorControls
        colorLabel={colorLabel}
        colorOptionLabel={colorOptionLabel}
        customColorLabel={customColorLabel}
        groupColor={group.color}
        onSetGroupColor={onSetGroupColor}
      />
      <div className="mt-3">
        <DashboardActionButton
          className="w-full"
          data-factory-visual-group-delete=""
          onClick={onDeleteGroup}
          tone="destructive"
          type="button"
        >
          {deleteGroupLabel}
        </DashboardActionButton>
      </div>
      <fieldset
        className="mt-3 grid max-h-48 gap-2 overflow-y-auto border-0 p-0"
        data-factory-visual-group-membership=""
      >
        <legend className="text-sm text-on-surface-subtle">
          {membershipLabel}
        </legend>
        {canvasNodeOptions.length === 0 && staleMemberNodeIds.length === 0 ? (
          <p className="m-0 text-sm text-on-surface-subtle">
            {membershipEmptyLabel}
          </p>
        ) : (
          <ul className="m-0 grid list-none gap-2 p-0">
            {canvasNodeOptions.map((option) => {
              const checked = isNodeMember(option.id);
              const checkboxId = `${group.id}-${option.id}`;

              return (
                <li key={option.id}>
                  <label
                    className="flex items-center gap-2 text-sm text-on-surface"
                    htmlFor={checkboxId}
                  >
                    <Checkbox
                      aria-label={membershipNodeLabel(option.label)}
                      checked={checked}
                      data-factory-visual-group-member={option.id}
                      id={checkboxId}
                      onChange={(event) =>
                        onToggleNodeMembership(option.id, event.target.checked)
                      }
                    />
                    <span className="flex min-w-0 items-baseline gap-1">
                      <span className="shrink-0 text-on-surface-subtle">
                        {membershipNodeKindLabel(option.kind)}
                      </span>
                      <span
                        aria-hidden="true"
                        className="text-on-surface-subtle"
                      >
                        ·
                      </span>
                      <span className="truncate">{option.label}</span>
                    </span>
                  </label>
                </li>
              );
            })}
            {staleMemberNodeIds.map((nodeId) => (
              <li key={nodeId}>
                <p className="m-0 text-sm text-on-error-container" role="alert">
                  {membershipStaleNodeLabel(nodeId)}
                </p>
              </li>
            ))}
          </ul>
        )}
      </fieldset>
    </section>
  );
}

function FactoryGraphVisualGroupColorControls({
  colorLabel,
  colorOptionLabel,
  customColorLabel,
  groupColor,
  onSetGroupColor,
}: {
  colorLabel: string;
  colorOptionLabel: (token: FactoryLayoutGroupColorToken) => string;
  customColorLabel: string;
  groupColor: string | undefined;
  onSetGroupColor: (color: string) => void;
}) {
  const customColorFieldId = useId();
  const selectedCustomColor =
    normalizeFactoryLayoutGroupCustomColor(groupColor);
  const selectedPresetColor = isApprovedFactoryLayoutGroupColor(groupColor)
    ? groupColor
    : selectedCustomColor === null
      ? "neutral"
      : null;
  const customColorValue =
    selectedCustomColor ?? DEFAULT_FACTORY_LAYOUT_GROUP_CUSTOM_COLOR;

  return (
    <fieldset className="mt-3 grid gap-2 border-0 p-0">
      <legend className="text-sm text-on-surface-subtle">{colorLabel}</legend>
      <div className="flex flex-wrap gap-2">
        {FACTORY_LAYOUT_GROUP_COLOR_TOKENS.map((token) => (
          <DashboardActionButton
            aria-label={colorOptionLabel(token)}
            aria-pressed={selectedPresetColor === token}
            className={cn(
              "h-8 min-h-8 w-8 min-w-8 rounded-full border-2 p-0",
              selectedPresetColor === token
                ? "border-on-surface ring-2 ring-af-overlay-focus"
                : "border-outline",
            )}
            data-factory-visual-group-color={token}
            iconOnly
            key={token}
            onClick={() => onSetGroupColor(token)}
            style={{
              backgroundColor: factoryLayoutGroupColorCssVariable(token),
            }}
            tone={selectedPresetColor === token ? "secondary" : "outline"}
            type="button"
          >
            <span className="sr-only">{colorOptionLabel(token)}</span>
          </DashboardActionButton>
        ))}
      </div>
      <label
        className="flex items-center justify-between gap-2 text-sm text-on-surface"
        htmlFor={customColorFieldId}
      >
        <span className="text-on-surface-subtle">{customColorLabel}</span>
        <input
          aria-label={customColorLabel}
          className="h-10 w-16 cursor-pointer rounded-lg border border-outline bg-surface p-1 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-focus-ring"
          data-factory-visual-group-custom-color=""
          id={customColorFieldId}
          onChange={(event) => {
            const normalizedColor = normalizeFactoryLayoutGroupCustomColor(
              event.target.value,
            );
            if (normalizedColor !== null) {
              onSetGroupColor(normalizedColor);
            }
          }}
          type="color"
          value={customColorValue}
        />
      </label>
    </fieldset>
  );
}
