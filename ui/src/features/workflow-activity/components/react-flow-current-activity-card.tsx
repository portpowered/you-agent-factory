import "@xyflow/react/dist/style.css";

import { useId } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { ImportFactoryValue } from "../../../api/session-factory";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import type { ReadFactoryImportFile } from "../../import/hooks/use-factory-png-drop";
import type { FactoryImportConfirmInput } from "../../import/lib/factory-import-save-choice";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { useCurrentActivityGraphViewModel } from "../hooks/react-flow-current-activity-card-graph-view-model";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";
import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";
import { CurrentActivityGraphSaveNotifications } from "./react-flow-current-activity-card-save-notifications";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "../lib/react-flow-current-activity-card-keys";

export { useCurrentActivityGraphViewModel } from "../hooks/react-flow-current-activity-card-graph-view-model";

const CURRENT_ACTIVITY_CARD_CLASS = cn(
  DASHBOARD_PANEL_SHELL_CLASS,
  "relative flex h-full min-h-0 min-w-0 flex-col",
);
const CURRENT_ACTIVITY_TITLE_CLASS = cn("m-0", DASHBOARD_SECTION_HEADING_CLASS);

export type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";

function CurrentActivityCardHeading({
  headingID,
  hidden = false,
  locale,
}: {
  headingID: string;
  hidden?: boolean;
  locale?: string;
}) {
  const messages = getWorkflowActivityShellMessages(locale);

  if (hidden) {
    return (
      <span className="sr-only" id={headingID}>
        {messages.title}
      </span>
    );
  }

  return (
    <div>
      <h2 className={CURRENT_ACTIVITY_TITLE_CLASS} id={headingID}>
        {messages.title}
      </h2>
    </div>
  );
}

function useCurrentActivityAccessibilityIDs(widgetInstanceID?: string) {
  const fallbackID = useId();

  return {
    headingID: `workflow-graph-heading-${widgetInstanceID ?? fallbackID}`,
  };
}

interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (input: FactoryImportConfirmInput) => Promise<ImportFactoryValue>;
  importController?: CurrentActivityImportController;
  locale?: string;
  now: number;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
  widgetInstanceID?: string;
}

export function ReactFlowCurrentActivityCard(
  props: ReactFlowCurrentActivityCardProps,
) {
  const { sessionID } = useDashboardSession();
  const editor = useCurrentActivityGraphEditor(
    props.snapshot,
    props.locale,
    sessionID,
  );
  return (
    <ReactFlowCurrentActivityCardView
      {...props}
      editor={editor}
      showHeaderActions
    />
  );
}

export function ReactFlowCurrentActivityCardView(
  props: ReactFlowCurrentActivityCardProps & {
    editor: ReturnType<typeof useCurrentActivityGraphEditor>;
    showHeaderActions?: boolean;
  },
) {
  const { editor, showHeaderActions = false } = props;
  const { headingID } = useCurrentActivityAccessibilityIDs(
    props.widgetInstanceID,
  );
  const graph = useCurrentActivityGraphViewModel({ ...props, editor });
  const fallbackImportController = useCurrentActivityImportController({
    activateFactory: props.activateFactory,
    locale: props.locale,
    onFactoryActivated: props.onFactoryActivated,
    onFactoryImportReady: props.onFactoryImportReady,
    readFactoryImportFile: props.readFactoryImportFile,
  });
  const imports = props.importController ?? fallbackImportController;
  const shouldRenderImportPreviewDialog = props.importController === undefined;
  const readyImportPreviewState =
    imports.importPreviewState.status === "ready"
      ? imports.importPreviewState
      : null;

  return (
    <section
      aria-labelledby={headingID}
      className={CURRENT_ACTIVITY_CARD_CLASS}
    >
      {showHeaderActions ? (
        <div className="mb-4">
          <CurrentActivityGraphHeaderActions
            editorMode={editor.editorMode}
            editorUnavailableClassifierWorkstationName={
              editor.editorUnavailableClassifierWorkstationName
            }
            hasChanges={editor.draftState.hasChanges}
            isDefinitionLoading={
              editor.editableDefinitionQuery.status === "pending"
            }
            loadErrorMessage={editor.editableDefinitionQuery.error?.message}
            locale={props.locale}
            onToggle={editor.handleEditorModeToggle}
          />
        </div>
      ) : null}
      {showHeaderActions ? (
        <CurrentActivityCardHeading
          headingID={headingID}
          locale={props.locale}
        />
      ) : (
        <CurrentActivityCardHeading
          headingID={headingID}
          hidden
          locale={props.locale}
        />
      )}
      <CurrentActivityGraphSurface
        editor={editor}
        graph={graph}
        headingID={headingID}
        imports={imports}
        locale={props.locale}
        selection={props.selection}
        snapshot={props.snapshot}
      />
      <CurrentActivityGraphSaveNotifications
        editor={editor}
        locale={props.locale}
      />
      <CurrentActivityGraphEditorDialogs
        currentSessionFactoryName={props.snapshot.factory?.name ?? "factory"}
        editor={editor}
        imports={imports}
        locale={props.locale}
        readyImportPreviewState={readyImportPreviewState}
        shouldRenderImportPreviewDialog={shouldRenderImportPreviewDialog}
      />
    </section>
  );
}
