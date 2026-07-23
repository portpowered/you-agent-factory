import { resolveInheritedFlagSource } from "./cli-input-semantics";
import type {
  CliArgument,
  CliCommand,
  CliDocumentationText,
  CliFlag,
  CliLifecycle,
  CliManifestLoadState,
  CliRelationship,
} from "./cli-manifest-types";

type ReadyCliManifest = Extract<CliManifestLoadState, { status: "ready" }>;

export type CliInputCardinality = {
  readonly minimum: number;
  /** Null represents an unbounded maximum. */
  readonly maximum: number | null;
};

export type CliInputSource = {
  readonly commandId: string;
  readonly inputId: string;
  readonly scope: "local" | "persistent";
};

export type CliCommandInputProjection = {
  readonly id: string;
  readonly kind: "argument" | "flag";
  readonly inherited: boolean;
  readonly source: CliInputSource;
  readonly cardinality: CliInputCardinality;
  readonly manifestInput: CliArgument | CliFlag;
};

export type CliRelationshipParticipantProjection = {
  readonly type: "argument" | "flag";
  readonly inputId: string;
  readonly input: CliCommandInputProjection;
};

export type CliRelationshipProjection = {
  readonly id: string;
  readonly kind: CliRelationship["kind"];
  readonly participants: readonly CliRelationshipParticipantProjection[];
  readonly when?: CliRelationshipParticipantProjection;
  readonly manifestRelationship: CliRelationship;
};

export type CliCommandProjection = {
  readonly id: string;
  readonly name: string;
  readonly path: string;
  readonly aliases: readonly string[];
  readonly lifecycle: CliLifecycle;
  readonly visibility: CliCommand["visibility"];
  readonly runnable: boolean;
  readonly usage: CliCommand["usage"];
  readonly help: {
    readonly title: CliDocumentationText;
    readonly description: CliDocumentationText;
  };
  readonly examples: readonly unknown[];
  readonly localInputs: readonly CliCommandInputProjection[];
  readonly inheritedInputs: readonly CliCommandInputProjection[];
  readonly effectiveInputs: readonly CliCommandInputProjection[];
  readonly relationships: readonly CliRelationshipProjection[];
};

export type CliCommandNavigationItem = {
  readonly id: string;
  readonly name: string;
  readonly path: string;
  readonly lifecycleState: CliLifecycle["state"];
  readonly visibility: CliCommand["visibility"];
  readonly children: readonly CliCommandNavigationItem[];
};

export type CliManifestProjection = {
  readonly rootCommandId: string;
  readonly navigation: CliCommandNavigationItem;
  readonly commandOrder: readonly string[];
  readonly commands: Readonly<Record<string, CliCommandProjection>>;
};

function flagSource(
  command: CliCommand,
  flag: CliFlag,
  commands: readonly CliCommand[],
): CliInputSource {
  if (flag.scope !== "inherited") {
    return { commandId: command.id, inputId: flag.id, scope: flag.scope };
  }

  const source = resolveInheritedFlagSource(command, flag, commands);
  if (source) {
    return {
      commandId: source.command.id,
      inputId: source.flag.id,
      scope: "persistent",
    };
  }

  throw new Error(`Validated inherited flag ${flag.id} has no source.`);
}

function projectInputs(
  command: CliCommand,
  commands: readonly CliCommand[],
): {
  localInputs: CliCommandInputProjection[];
  inheritedInputs: CliCommandInputProjection[];
  effectiveInputs: CliCommandInputProjection[];
} {
  const argumentsInPositionOrder = Object.values(command.arguments ?? {}).sort(
    (left, right) => left.position - right.position,
  );
  const argumentInputs = argumentsInPositionOrder.map(
    (argument): CliCommandInputProjection => ({
      id: argument.id,
      kind: "argument",
      inherited: false,
      source: {
        commandId: command.id,
        inputId: argument.id,
        scope: "local",
      },
      cardinality: {
        minimum: argument.minCardinality,
        maximum:
          argument.maxCardinality === -1 ? null : argument.maxCardinality,
      },
      manifestInput: argument,
    }),
  );
  const flagInputs = Object.values(command.flags ?? {}).map(
    (flag): CliCommandInputProjection => ({
      id: flag.id,
      kind: "flag",
      inherited: flag.scope === "inherited",
      source: flagSource(command, flag, commands),
      cardinality: {
        minimum:
          flag.kind === "named" ? flag.minCardinality : flag.required ? 1 : 0,
        maximum:
          flag.kind === "named"
            ? flag.maxCardinality === -1
              ? null
              : flag.maxCardinality
            : flag.repeatable
              ? null
              : 1,
      },
      manifestInput: flag,
    }),
  );
  const inheritedInputs = flagInputs.filter((input) => input.inherited);
  const localInputs = [
    ...argumentInputs,
    ...flagInputs.filter((input) => !input.inherited),
  ];
  return {
    localInputs,
    inheritedInputs,
    effectiveInputs: [...localInputs, ...inheritedInputs],
  };
}

function projectRelationships(
  command: CliCommand,
  inputsById: ReadonlyMap<string, CliCommandInputProjection>,
): CliRelationshipProjection[] {
  const participant = (
    reference: CliRelationship["participants"][number],
  ): CliRelationshipParticipantProjection => {
    const input = inputsById.get(reference.id);
    if (!input) {
      throw new Error(
        `Validated relationship input ${reference.id} does not resolve.`,
      );
    }
    return { type: reference.type, inputId: input.id, input };
  };

  return Object.values(command.relationships ?? {}).map((relationship) => ({
    id: relationship.id,
    kind: relationship.kind,
    participants: relationship.participants.map(participant),
    ...(relationship.when ? { when: participant(relationship.when) } : {}),
    manifestRelationship: relationship,
  }));
}

function projectCommand(
  command: CliCommand,
  commands: readonly CliCommand[],
): CliCommandProjection {
  const inputs = projectInputs(command, commands);
  const inputsById = new Map(
    inputs.effectiveInputs.map((input) => [input.id, input] as const),
  );
  return {
    id: command.id,
    name: command.name,
    path: command.path,
    aliases: command.aliases,
    lifecycle: command.lifecycle,
    visibility: command.visibility,
    runnable: command.runnable,
    usage: command.usage,
    help: command.documentation.documentation,
    examples: command.documentation.examples,
    ...inputs,
    relationships: projectRelationships(command, inputsById),
  };
}

function projectNavigation(
  commands: readonly CliCommand[],
  rootPath: string,
): CliCommandNavigationItem {
  type MutableItem = Omit<CliCommandNavigationItem, "children"> & {
    children: CliCommandNavigationItem[];
  };
  const byPath = new Map<string, MutableItem>();
  for (const command of commands) {
    byPath.set(command.path, {
      id: command.id,
      name: command.name,
      path: command.path,
      lifecycleState: command.lifecycle.state,
      visibility: command.visibility,
      children: [],
    });
  }
  for (const command of commands) {
    if (command.path === rootPath) continue;
    const parentPath = command.path.split(" ").slice(0, -1).join(" ");
    byPath
      .get(parentPath)
      ?.children.push(byPath.get(command.path) as MutableItem);
  }
  return byPath.get(rootPath) as MutableItem;
}

/** Projects a previously validated ready manifest without mutating canonical data. */
export function projectCliManifest(
  ready: ReadyCliManifest,
): CliManifestProjection {
  const canonicalCommands = Object.values(ready.manifest.commands);
  const commandsByPath = new Map(
    canonicalCommands.map((command) => [command.path, command] as const),
  );
  const projectedCommands = Object.fromEntries(
    canonicalCommands.map((command) => [
      command.id,
      projectCommand(command, canonicalCommands),
    ]),
  );
  const rootCommand = commandsByPath.get(ready.manifest.rootPath);
  if (!rootCommand) {
    throw new Error("Validated CLI manifest has no root command.");
  }
  return {
    rootCommandId: rootCommand.id,
    navigation: projectNavigation(canonicalCommands, ready.manifest.rootPath),
    commandOrder: canonicalCommands.map((command) => command.id),
    commands: projectedCommands,
  };
}
