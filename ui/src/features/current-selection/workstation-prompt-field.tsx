import { useEffect, useMemo, useRef } from "react";

import { Textarea } from "../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { cx } from "../../lib/cx";
import type { WorkstationDetailCardProps } from "./detail-card-types";
import type { getWorkstationDetailMessages } from "./messages";

export function EditableConfigurationPromptInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const overlayRef = useRef<HTMLDivElement | null>(null);
  const diagnosticsId = "editable-workstation-prompt-diagnostics";
  const errorId = "editable-workstation-prompt-error";
  const describedBy = [
    state.validationErrors.prompt ? errorId : null,
    state.promptDiagnostics.length > 0 ? diagnosticsId : null,
  ]
    .filter(Boolean)
    .join(" ");

  useEffect(() => {
    const textarea = textareaRef.current;
    const overlay = overlayRef.current;
    if (!textarea || !overlay) {
      return;
    }

    const syncScroll = () => {
      overlay.scrollTop = textarea.scrollTop;
      overlay.scrollLeft = textarea.scrollLeft;
    };

    syncScroll();
    textarea.addEventListener("scroll", syncScroll);
    return () => textarea.removeEventListener("scroll", syncScroll);
  }, []);

  return (
    <div className="grid gap-2">
      <div className="relative">
        <div
          aria-hidden="true"
          className={cx(
            "pointer-events-none absolute inset-0 overflow-hidden rounded-xl border border-af-overlay/14 px-3 py-3 text-sm text-transparent",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          ref={overlayRef}
        >
          <PromptDiagnosticOverlay
            diagnostics={state.promptDiagnostics}
            prompt={state.draft.prompt}
          />
        </div>
        <Textarea
          aria-describedby={describedBy || undefined}
          aria-invalid={state.validationErrors.prompt ? "true" : undefined}
          className={cx(
            "relative z-10 bg-transparent",
            state.promptDiagnostics.length > 0
              ? "border-af-danger/45 focus-visible:border-af-danger focus-visible:ring-af-danger/20"
              : undefined,
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          id="editable-workstation-prompt"
          onChange={(event) => state.onPromptChange(event.target.value)}
          ref={textareaRef}
          value={state.draft.prompt}
        />
      </div>
      <EditableConfigurationPromptValidationFeedback
        diagnosticsId={diagnosticsId}
        messages={messages}
        state={state}
      />
    </div>
  );
}

function EditableConfigurationPromptValidationFeedback({
  diagnosticsId,
  messages,
  state,
}: {
  diagnosticsId: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  if (state.promptValidationState.status === "loading") {
    return (
      <p className={cx("m-0 text-af-ink/70", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationPromptValidationLoading}
      </p>
    );
  }

  if (state.promptValidationState.status === "error") {
    return (
      <p
        className={cx(
          "m-0 text-af-danger-ink",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
        role="alert"
      >
        {messages.editableConfigurationPromptValidationErrorPrefix}{" "}
        {state.promptValidationState.errorMessage}
      </p>
    );
  }

  if (state.promptDiagnostics.length === 0) {
    return null;
  }

  return (
    <div
      className="grid gap-2 rounded-xl border border-af-danger/20 bg-af-danger/6 p-3"
      id={diagnosticsId}
      role="alert"
    >
      <p className={cx("m-0 text-af-danger-ink", DASHBOARD_BODY_TEXT_CLASS)}>
        {messages.editableConfigurationPromptDiagnosticsSummary}
      </p>
      <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationPromptValidationDetail}
      </p>
      <div className="grid gap-2">
        <h5 className={cx("m-0", DASHBOARD_SUPPORTING_LABEL_CLASS)}>
          {messages.editableConfigurationPromptDiagnosticsHeading}
        </h5>
        <ul className="m-0 grid list-none gap-2 p-0">
          {state.promptDiagnostics.map((diagnostic, index) => (
            <li
              className="grid gap-1 rounded-lg border border-af-danger/18 bg-af-overlay/4 p-2"
              key={`${diagnostic.kind}:${diagnostic.path ?? diagnostic.sourceText ?? index}`}
            >
              <p
                className={cx("m-0 text-af-danger-ink", DASHBOARD_BODY_TEXT_CLASS)}
              >
                {diagnosticLabel(diagnostic.kind, messages)}: {diagnostic.message}
              </p>
              {diagnostic.path ? (
                <code className="text-xs text-af-ink/72">{diagnostic.path}</code>
              ) : null}
              {diagnostic.sourceText ? (
                <pre className="m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-2 text-xs text-af-ink/78 [overflow-wrap:anywhere]">
                  {diagnostic.sourceText}
                </pre>
              ) : null}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function PromptDiagnosticOverlay({
  diagnostics,
  prompt,
}: {
  diagnostics: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >["promptDiagnostics"];
  prompt: string;
}) {
  const segments = useMemo(
    () => buildPromptDiagnosticSegments(prompt, diagnostics),
    [diagnostics, prompt],
  );

  return (
    <pre className="m-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
      {segments.map((segment) =>
        segment.hasDiagnostic ? (
          <mark
            className="bg-transparent text-transparent underline decoration-wavy decoration-2 decoration-af-danger"
            key={segment.key}
          >
            {segment.text}
          </mark>
        ) : (
          <span key={segment.key}>{segment.text}</span>
        ),
      )}
    </pre>
  );
}

function buildPromptDiagnosticSegments(
  prompt: string,
  diagnostics: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >["promptDiagnostics"],
) {
  if (prompt.length === 0 || diagnostics.length === 0) {
    return [{ hasDiagnostic: false, key: "segment:0:0", text: prompt }];
  }

  const ranges = normalizePromptDiagnosticRanges(prompt, diagnostics);
  if (ranges.length === 0) {
    return [{ hasDiagnostic: false, key: "segment:0:0", text: prompt }];
  }

  const segments: Array<{ hasDiagnostic: boolean; key: string; text: string }> =
    [];
  let cursor = 0;
  for (const range of ranges) {
    if (cursor < range.start) {
      segments.push({
        hasDiagnostic: false,
        key: `segment:${cursor}:${range.start}:plain`,
        text: prompt.slice(cursor, range.start),
      });
    }

    segments.push({
      hasDiagnostic: true,
      key: `segment:${range.start}:${range.end}:diagnostic`,
      text: prompt.slice(range.start, range.end),
    });
    cursor = range.end;
  }

  if (cursor < prompt.length) {
    segments.push({
      hasDiagnostic: false,
      key: `segment:${cursor}:${prompt.length}:plain`,
      text: prompt.slice(cursor),
    });
  }

  return segments;
}

function normalizePromptDiagnosticRanges(
  prompt: string,
  diagnostics: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >["promptDiagnostics"],
) {
  const ranges = diagnostics
    .map((diagnostic, index) =>
      resolvePromptDiagnosticRange(prompt, diagnostic, index),
    )
    .filter((range): range is { end: number; start: number } => range !== null)
    .sort((left, right) => left.start - right.start || left.end - right.end);

  if (ranges.length === 0) {
    return [];
  }

  const merged: Array<{ end: number; start: number }> = [ranges[0]];
  for (const range of ranges.slice(1)) {
    const current = merged[merged.length - 1];
    if (range.start <= current.end) {
      current.end = Math.max(current.end, range.end);
      continue;
    }

    merged.push(range);
  }

  return merged;
}

function resolvePromptDiagnosticRange(
  prompt: string,
  diagnostic: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >["promptDiagnostics"][number],
  diagnosticIndex: number,
) {
  if (diagnostic.sourceText) {
    const sourceTextIndex = nthIndexOf(prompt, diagnostic.sourceText, diagnosticIndex);
    if (sourceTextIndex >= 0) {
      return {
        end: sourceTextIndex + diagnostic.sourceText.length,
        start: sourceTextIndex,
      };
    }
  }

  if (
    typeof diagnostic.startOffset === "number" &&
    typeof diagnostic.endOffset === "number"
  ) {
    const start = utf8ByteOffsetToCodeUnitIndex(prompt, diagnostic.startOffset);
    const end = utf8ByteOffsetToCodeUnitIndex(prompt, diagnostic.endOffset + 1);
    if (start < end) {
      return { end, start };
    }
  }

  return null;
}

function nthIndexOf(text: string, query: string, occurrence: number) {
  let fromIndex = 0;
  let matchIndex = -1;

  for (let index = 0; index <= occurrence; index += 1) {
    matchIndex = text.indexOf(query, fromIndex);
    if (matchIndex < 0) {
      return -1;
    }
    fromIndex = matchIndex + query.length;
  }

  return matchIndex;
}

function utf8ByteOffsetToCodeUnitIndex(text: string, oneBasedByteOffset: number) {
  if (oneBasedByteOffset <= 1) {
    return 0;
  }

  const targetByteCount = oneBasedByteOffset - 1;
  let bytesSeen = 0;

  for (let index = 0; index < text.length; index += 1) {
    const codePoint = text.codePointAt(index);
    if (codePoint == null) {
      return index;
    }

    const codeUnitWidth = codePoint > 0xffff ? 2 : 1;
    const byteWidth =
      codePoint <= 0x7f ? 1 : codePoint <= 0x7ff ? 2 : codePoint <= 0xffff ? 3 : 4;
    if (bytesSeen >= targetByteCount) {
      return index;
    }

    bytesSeen += byteWidth;
    if (bytesSeen >= targetByteCount) {
      return index + codeUnitWidth;
    }

    index += codeUnitWidth - 1;
  }

  return text.length;
}

function diagnosticLabel(
  kind: string,
  messages: ReturnType<typeof getWorkstationDetailMessages>,
) {
  return kind === "SYNTAX_ERROR"
    ? messages.editableConfigurationPromptSyntaxDiagnosticLabel
    : messages.editableConfigurationPromptVariableDiagnosticLabel;
}
