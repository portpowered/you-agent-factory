import "@xyflow/react/dist/style.css";

import { useId } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { ImportFactoryValue } from "../../../api/session-factory";
import { Heading } from "../../../components/ui";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import type { ReadFactoryImportFile } from "../../import/hooks/use-factory-png-drop";
import type { FactoryImportConfirmInput } from "../../import/lib/factory-import-save-choice";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "../hooks/current-activity-import-controller";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";
import { CurrentActivityGraphSaveNotifications } from "./react-flow-current-activity-card-save-notifications";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

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
      <Heading as="h2" className="m-0" id={headingID}>
        {messages.title}
      </Heading>
    </div>
  );
}

function useCurrentActivityAccessibilityIDs(widgetInstanceID?: string) {
  const fallbackID = useId();

  return {
    headingID: `workflow-graph-heading-${widgetInstanceID ?? fallbackID}`,
  };
}

export interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (
    input: FactoryImportConfirmInput,
  ) => Promise<ImportFactoryValue>;
  importController?: CurrentActivityImportController;
  locale?: string;
  now: number;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  onSelectDoc: (targetPath: string) => void;
  onSelectResource: (resourceName: string) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkType: (workTypeName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  selection: CurrentActivitySelection | null;
  sessionID?: string | null;
  snapshot: DashboardSnapshot;
  widgetInstanceID?: string;
}

export function ReactFlowCurrentActivityCardView(
  props: ReactFlowCurrentActivityCardProps & {
    viewModel: CurrentActivityGraphCardViewModel;
    showHeaderActions?: boolean;
  },
) {
  const { viewModel, showHeaderActions = false } = props;
  const { headingID } = useCurrentActivityAccessibilityIDs(
    props.widgetInstanceID,
  );
  const fallbackImportController = useCurrentActivityImportController({
    activateFactory: props.activateFactory,
    currentFactoryDefinition: props.snapshot.factory,
    locale: props.locale,
    onFactoryActivated: props.onFactoryActivated,
    onFactoryImportReady: props.onFactoryImportReady,
    readFactoryImportFile: props.readFactoryImportFile,
    sessionID: props.sessionID,
  });
  const imports = props.importController ?? fallbackImportController;
  const shouldRenderImportPreviewDialog = props.importController === undefined;
  const readyImportPreviewState =
    imports.importPreviewState.status === "ready"
      ? imports.importPreviewState
      : null;
  const handleDiscardEditorChanges = () => {
    viewModel.leaveControls.discardChanges();
  };

  return (
    <DashboardPanelShell
      aria-labelledby={headingID}
      className="relative flex h-full max-h-full min-h-0 min-w-0 flex-col overflow-hidden"
      style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
    >
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
        viewModel={viewModel}
        headingID={headingID}
        imports={imports}
        locale={props.locale}
        onSelectResource={props.onSelectResource}
        onSelectStateNode={props.onSelectStateNode}
        onSelectWorker={props.onSelectWorker}
        onSelectWorkType={props.onSelectWorkType}
        onSelectWorkstation={props.onSelectWorkstation}
        selection={props.selection}
        snapshot={props.snapshot}
      />
      <CurrentActivityGraphSaveNotifications
        viewModel={viewModel}
        locale={props.locale}
      />
      <CurrentActivityGraphEditorDialogs
        currentSessionFactoryName={props.snapshot.factory?.name ?? "factory"}
        discardEditorChanges={handleDiscardEditorChanges}
        viewModel={viewModel}
        imports={imports}
        locale={props.locale}
        readyImportPreviewState={readyImportPreviewState}
        shouldRenderImportPreviewDialog={shouldRenderImportPreviewDialog}
      />
    </DashboardPanelShell>
  );
}
