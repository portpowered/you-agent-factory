import { Button } from "../../../components/ui";
import { getFactoryGraphEditorMessages } from "../messages/editor";

const DRAFT_ACTIONS_CLASS =
  "flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-af-warning-border bg-af-warning-surface px-4 py-3";

export function FactoryGraphEditorDraftActions({
  canDiscard = true,
  canSave,
  description,
  isSaving = false,
  locale,
  onDiscard,
  onSave,
  saveDisabledReason,
  visible,
}: {
  canDiscard?: boolean;
  canSave: boolean;
  description: string;
  isSaving?: boolean;
  locale?: string;
  onDiscard: () => void;
  onSave: () => void;
  saveDisabledReason?: string;
  visible: boolean;
}) {
  if (!visible) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <section aria-label={messages.draftActionsAriaLabel} className={DRAFT_ACTIONS_CLASS}>
      <div className="grid gap-1">
        <p className="m-0 text-sm font-semibold text-af-text">
          {messages.draftActionsTitle}
        </p>
        <p className="m-0 text-sm leading-6 text-af-text-muted">{description}</p>
        {saveDisabledReason ? (
          <p className="m-0 text-xs leading-5 text-af-warning-text">
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
          {messages.draftActionsDiscard}
        </Button>
        <Button
          disabled={!canSave || isSaving}
          onClick={onSave}
          type="button"
        >
          {isSaving ? messages.draftActionsSaving : messages.draftActionsSave}
        </Button>
      </div>
    </section>
  );
}
