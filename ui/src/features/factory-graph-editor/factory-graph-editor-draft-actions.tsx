import { Button } from "../../components/ui";

const DRAFT_ACTIONS_CLASS =
  "flex flex-wrap items-center justify-between gap-3 rounded-[1.25rem] border border-af-warning/24 bg-af-warning/8 px-4 py-3";

export function FactoryGraphEditorDraftActions({
  canDiscard = true,
  canSave,
  description,
  isSaving = false,
  onDiscard,
  onSave,
  saveDisabledReason,
  visible,
}: {
  canDiscard?: boolean;
  canSave: boolean;
  description: string;
  isSaving?: boolean;
  onDiscard: () => void;
  onSave: () => void;
  saveDisabledReason?: string;
  visible: boolean;
}) {
  if (!visible) {
    return null;
  }

  return (
    <section
      aria-label="Pending graph changes"
      className={DRAFT_ACTIONS_CLASS}
    >
      <div className="grid gap-1">
        <p className="m-0 text-sm font-semibold text-af-ink">
          Pending graph changes
        </p>
        <p className="m-0 text-sm leading-6 text-af-ink/76">{description}</p>
        {saveDisabledReason ? (
          <p className="m-0 text-xs leading-5 text-af-warning-ink">
            {saveDisabledReason}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          disabled={!canDiscard || isSaving}
          onClick={onDiscard}
          tone="outline"
          type="button"
        >
          Discard changes
        </Button>
        <Button
          disabled={!canSave || isSaving}
          onClick={onSave}
          type="button"
        >
          {isSaving ? "Saving..." : "Save changes"}
        </Button>
      </div>
    </section>
  );
}
