import { useId } from "react";

import { DashboardActionButton } from "../../../../../components/ui";
import { cn } from "../../../../../lib/cn";
import {
  FACTORY_LAYOUT_GROUP_COLOR_TOKENS,
  type FactoryLayoutGroup,
  type FactoryLayoutGroupCanvasNodeOption,
  type FactoryLayoutGroupColorToken,
  factoryLayoutGroupColorCssVariable,
  isApprovedFactoryLayoutGroupColor,
} from "../../../lib/layout/visual-groups/factory-graph-layout-groups";

export function FactoryGraphVisualGroupControls({
  canvasNodeOptions,
  colorLabel,
  colorOptionLabel,
  deleteGroupLabel,
  boundsError,
  emptyLabelError,
  group,
  isNodeMember,
  labelFieldLabel,
  membershipEmptyLabel,
  membershipLabel,
  membershipNodeLabel,
  membershipStaleNodeLabel,
  onDeleteGroup,
  onRenameGroup,
  onSetGroupColor,
  onToggleNodeMembership,
  selectedGroupLabel,
  staleMemberNodeIds,
}: {
  canvasNodeOptions: readonly FactoryLayoutGroupCanvasNodeOption[];
  colorLabel: string;
  colorOptionLabel: (token: FactoryLayoutGroupColorToken) => string;
  deleteGroupLabel: string;
  boundsError: string | null;
  emptyLabelError: string;
  group: FactoryLayoutGroup;
  isNodeMember: (nodeId: string) => boolean;
  labelFieldLabel: string;
  membershipEmptyLabel: string;
  membershipLabel: string;
  membershipNodeLabel: (label: string) => string;
  membershipStaleNodeLabel: (nodeId: string) => string;
  onDeleteGroup: () => void;
  onRenameGroup: (label: string) => void;
  onSetGroupColor: (color: FactoryLayoutGroupColorToken) => void;
  onToggleNodeMembership: (nodeId: string, selected: boolean) => void;
  selectedGroupLabel: string;
  staleMemberNodeIds: readonly string[];
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
      </div>
      <fieldset className="mt-3 grid gap-2 border-0 p-0">
        <legend className="text-sm text-on-surface-subtle">{colorLabel}</legend>
        <div className="flex flex-wrap gap-2">
          {FACTORY_LAYOUT_GROUP_COLOR_TOKENS.map((token) => (
            <DashboardActionButton
              aria-label={colorOptionLabel(token)}
              aria-pressed={selectedColor === token}
              className={cn(
                "h-8 min-h-8 w-8 min-w-8 rounded-full border-2 p-0",
                selectedColor === token
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
              tone={selectedColor === token ? "secondary" : "outline"}
              type="button"
            >
              <span className="sr-only">{colorOptionLabel(token)}</span>
            </DashboardActionButton>
          ))}
        </div>
      </fieldset>
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
                    <input
                      checked={checked}
                      className="h-4 w-4 rounded border-outline text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
                      data-factory-visual-group-member={option.id}
                      id={checkboxId}
                      onChange={(event) =>
                        onToggleNodeMembership(option.id, event.target.checked)
                      }
                      type="checkbox"
                    />
                    <span>{membershipNodeLabel(option.label)}</span>
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
