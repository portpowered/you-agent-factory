import {
  type KeyboardEvent,
  type RefCallback,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";

import {
  AlertPanel,
  AlertPanelText,
  AlertPanelTitle,
  Code,
  CodePanel,
  DashboardStatusPill,
  Heading,
  Skeleton,
  StandardListSelection,
  StandardListSelectionItem,
  SurfacePanel,
  Text,
} from "../../../components/ui";
import {
  type CliCommandNavigationItem,
  type CliCommandProjection,
  projectCliManifest,
} from "../lib/cli-command-projection";
import { projectCliCommandControls } from "../lib/cli-control-projection";
import type {
  CliArgument,
  CliFlag,
  CliManifestLoadState,
} from "../lib/cli-manifest-types";
import { getCliCommandExplorerMessages } from "../messages/cli-command-explorer";
import { StaticCliControls } from "./static-cli-controls";

export interface StaticCliCommandExplorerProps {
  readonly locale?: string | null;
  readonly state: CliManifestLoadState;
}

type FlatNavigationItem = CliCommandNavigationItem & {
  readonly level: number;
  readonly parentId?: string;
};

function flattenNavigation(
  item: CliCommandNavigationItem,
  level = 1,
  parentId?: string,
): FlatNavigationItem[] {
  const current = { ...item, level, ...(parentId ? { parentId } : {}) };
  return [
    current,
    ...item.children.flatMap((child) =>
      flattenNavigation(child, level + 1, item.id),
    ),
  ];
}

export function StaticCliCommandExplorer({
  locale,
  state,
}: StaticCliCommandExplorerProps) {
  const messages = getCliCommandExplorerMessages(locale);
  switch (state.status) {
    case "loading":
      return (
        <AlertPanel
          aria-busy="true"
          aria-label={messages.loadingTitle}
          role="status"
          tone="info"
        >
          <Skeleton className="h-8 w-48" />
          <AlertPanelTitle>{messages.loadingTitle}</AlertPanelTitle>
          <AlertPanelText>{messages.loadingDescription}</AlertPanelText>
        </AlertPanel>
      );
    case "empty":
      return (
        <AlertPanel
          aria-label={messages.emptyTitle}
          role="status"
          tone="neutral"
          variant="empty"
        >
          <AlertPanelTitle>{messages.emptyTitle}</AlertPanelTitle>
          <AlertPanelText>{messages.emptyDescription}</AlertPanelText>
        </AlertPanel>
      );
    case "invalid-contract":
      return (
        <AlertPanel
          aria-label={messages.invalidTitle}
          role="alert"
          tone="danger"
        >
          <AlertPanelTitle>{messages.invalidTitle}</AlertPanelTitle>
          <AlertPanelText>{messages.invalidDescription}</AlertPanelText>
          <ul className="grid gap-2">
            {state.diagnostics.map((diagnostic) => (
              <li key={`${diagnostic.path.join(".")}-${diagnostic.code}`}>
                <Code size="supporting">{diagnostic.message}</Code>
              </li>
            ))}
          </ul>
        </AlertPanel>
      );
    case "unsupported-version":
      return (
        <AlertPanel
          aria-label={messages.unsupportedTitle}
          role="alert"
          tone="warning"
        >
          <AlertPanelTitle>{messages.unsupportedTitle}</AlertPanelTitle>
          <AlertPanelText>
            {messages.unsupportedDescription(
              state.receivedVersion,
              state.supportedVersions.join(", "),
            )}
          </AlertPanelText>
        </AlertPanel>
      );
    case "ready":
      return (
        <ReadyCliCommandExplorer
          key={`${state.manifest.formatVersion}-${state.manifest.rootPath}`}
          locale={locale}
          state={state}
        />
      );
  }
}

function ReadyCliCommandExplorer({
  locale,
  state,
}: {
  readonly locale?: string | null;
  readonly state: Extract<CliManifestLoadState, { status: "ready" }>;
}) {
  const messages = getCliCommandExplorerMessages(locale);
  const projection = useMemo(() => projectCliManifest(state), [state]);
  const items = useMemo(
    () => flattenNavigation(projection.navigation),
    [projection.navigation],
  );
  const [selectedId, setSelectedId] = useState(projection.rootCommandId);
  const [focusedId, setFocusedId] = useState(projection.rootCommandId);
  const selectedCommand = projection.commands[selectedId];
  const itemRefs = useRef(new Map<string, HTMLButtonElement>());
  const titleId = useId();

  const setItemRef =
    (id: string): RefCallback<HTMLButtonElement> =>
    (node) => {
      if (node) itemRefs.current.set(id, node);
      else itemRefs.current.delete(id);
    };
  const focusItem = (id: string | undefined) => {
    if (id) itemRefs.current.get(id)?.focus();
  };
  const onNavigationKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    item: FlatNavigationItem,
  ) => {
    const index = items.findIndex(({ id }) => id === item.id);
    let targetId: string | undefined;
    switch (event.key) {
      case "ArrowDown":
        targetId = items[Math.min(index + 1, items.length - 1)]?.id;
        break;
      case "ArrowUp":
        targetId = items[Math.max(index - 1, 0)]?.id;
        break;
      case "ArrowRight":
        targetId = item.children[0]?.id;
        break;
      case "ArrowLeft":
        targetId = item.parentId;
        break;
      case "Home":
        targetId = items[0]?.id;
        break;
      case "End":
        targetId = items.at(-1)?.id;
        break;
      default:
        return;
    }
    if (targetId) {
      event.preventDefault();
      focusItem(targetId);
    }
  };

  return (
    <section
      aria-label={messages.selectedCommand(selectedCommand.path)}
      className="grid min-w-0 gap-4 lg:grid-cols-[minmax(14rem,20rem)_minmax(0,1fr)]"
    >
      <SurfacePanel
        asChild
        className="min-w-0 self-start lg:sticky lg:top-4"
        padding="default"
        radius="2xl"
        surface="low"
      >
        <nav aria-label={messages.commandNavigation}>
          <Heading as="h2" wrap>
            {messages.commandNavigation}
          </Heading>
          <StandardListSelection
            aria-label={messages.commandNavigation}
            className="mt-3 max-h-80 overflow-y-auto lg:max-h-screen"
            role="tree"
            selectionAnnouncement={messages.selectedCommand(
              selectedCommand.path,
            )}
          >
            {items.map((item) => {
              const selected = item.id === selectedId;
              return (
                <StandardListSelectionItem
                  aria-level={item.level}
                  aria-selected={selected}
                  className="min-w-0"
                  key={item.id}
                  onClick={() => {
                    setFocusedId(item.id);
                    setSelectedId(item.id);
                  }}
                  onFocus={() => setFocusedId(item.id)}
                  onKeyDown={(event) => onNavigationKeyDown(event, item)}
                  ref={setItemRef(item.id)}
                  role="treeitem"
                  selected={selected}
                  tabIndex={item.id === focusedId ? 0 : -1}
                >
                  <span
                    className="grid min-w-0 gap-1"
                    style={{
                      paddingInlineStart: `${(item.level - 1) * 0.75}rem`,
                    }}
                  >
                    <Code className="break-words" size="supporting">
                      {item.path}
                    </Code>
                    <span className="flex flex-wrap gap-1">
                      <DashboardStatusPill size="compact">
                        {messages.lifecycle(item.lifecycleState)}
                      </DashboardStatusPill>
                      {item.visibility !== "visible" ? (
                        <DashboardStatusPill size="compact" tone="warning">
                          {messages.visibility(item.visibility)}
                        </DashboardStatusPill>
                      ) : null}
                    </span>
                  </span>
                </StandardListSelectionItem>
              );
            })}
          </StandardListSelection>
        </nav>
      </SurfacePanel>

      <CommandDetail
        command={selectedCommand}
        key={selectedCommand.id}
        locale={locale}
        titleId={titleId}
      />
    </section>
  );
}

function displayExample(example: unknown): string {
  return typeof example === "string" ? example : JSON.stringify(example);
}

function inputLabel(command: CliCommandProjection, inputId: string): string {
  const input = command.effectiveInputs.find(({ id }) => id === inputId);
  if (!input) return inputId;
  return input.kind === "argument"
    ? `<${(input.manifestInput as CliArgument).name}>`
    : `--${(input.manifestInput as CliFlag).long}`;
}

function CommandDetail({
  command,
  locale,
  titleId,
}: {
  readonly command: CliCommandProjection;
  readonly locale?: string | null;
  readonly titleId: string;
}) {
  const messages = getCliCommandExplorerMessages(locale);
  const controls = projectCliCommandControls(command);
  return (
    <article className="grid min-w-0 gap-6" aria-labelledby={titleId}>
      <SurfacePanel
        className="grid min-w-0 gap-4"
        padding="default"
        radius="2xl"
      >
        <div className="grid min-w-0 gap-2">
          <Code className="break-words text-on-surface-variant">
            {command.path}
          </Code>
          <Heading as="h1" id={titleId} level="page" wrap>
            {command.help.title.canonicalEnglish}
          </Heading>
          <div className="flex flex-wrap gap-2">
            <DashboardStatusPill size="compact" tone="success">
              {messages.lifecycle(command.lifecycle.state)}
            </DashboardStatusPill>
            <DashboardStatusPill size="compact">
              {messages.inputs(command.effectiveInputs.length)}
            </DashboardStatusPill>
          </div>
        </div>
        <Text className="whitespace-pre-wrap" wrap>
          {command.help.description.canonicalEnglish}
        </Text>
      </SurfacePanel>

      <SurfacePanel
        className="grid min-w-0 gap-3"
        padding="default"
        radius="2xl"
      >
        <Heading as="h2" wrap>
          {messages.usage}
        </Heading>
        <CodePanel className="overflow-x-auto whitespace-pre-wrap break-words">
          {command.usage.line}
        </CodePanel>
      </SurfacePanel>

      <SurfacePanel
        className="grid min-w-0 gap-3"
        padding="default"
        radius="2xl"
      >
        <Heading as="h2" wrap>
          {messages.examples}
        </Heading>
        {command.examples.length > 0 ? (
          <ul className="grid gap-2">
            {command.examples.map((example, index) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: published examples have no stable identity.
              <li key={index}>
                <CodePanel className="overflow-x-auto whitespace-pre-wrap break-words">
                  {displayExample(example)}
                </CodePanel>
              </li>
            ))}
          </ul>
        ) : (
          <Text variant="supporting">{messages.noExamples}</Text>
        )}
      </SurfacePanel>

      <SurfacePanel
        className="grid min-w-0 gap-3"
        padding="default"
        radius="2xl"
      >
        <Heading as="h2" wrap>
          {messages.relationships}
        </Heading>
        {command.relationships.length > 0 ? (
          <ul className="grid gap-2">
            {command.relationships.map((relationship) => (
              <li key={relationship.id}>
                <Text>
                  {messages.relationship(
                    relationship.kind,
                    relationship.participants
                      .map(({ inputId }) => inputLabel(command, inputId))
                      .join(", "),
                  )}
                </Text>
              </li>
            ))}
          </ul>
        ) : (
          <Text variant="supporting">{messages.noRelationships}</Text>
        )}
      </SurfacePanel>

      <SurfacePanel
        className="grid min-w-0 gap-4"
        padding="default"
        radius="2xl"
      >
        <Heading as="h2" wrap>
          {messages.controls}
        </Heading>
        {controls.status === "ready" ? (
          controls.model.controls.length > 0 ? (
            <StaticCliControls locale={locale} model={controls.model} />
          ) : (
            <Text variant="supporting">{messages.noInputs}</Text>
          )
        ) : (
          <AlertPanel role="alert" tone="danger">
            <AlertPanelTitle>
              {messages.unsupportedControlTitle}
            </AlertPanelTitle>
            <AlertPanelText>
              {messages.unsupportedControlDescription(
                controls.inputId,
                controls.valueType,
              )}
            </AlertPanelText>
          </AlertPanel>
        )}
      </SurfacePanel>
    </article>
  );
}
