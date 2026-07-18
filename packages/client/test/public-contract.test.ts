import type {
  FactoryDefinition,
  FactoryEvent,
  FactoryEventType,
  FactoryRecording,
  components,
  operations,
  paths,
} from "@you-agent-factory/client";

type Equal<Left, Right> =
  (<Value>() => Value extends Left ? 1 : 2) extends <Value>() =>
    Value extends Right ? 1 : 2
    ? true
    : false;
type Assert<Value extends true> = Value;

type _FactoryEventAlias = Assert<
  Equal<FactoryEvent, components["schemas"]["FactoryEvent"]>
>;
type _FactoryEventTypeAlias = Assert<
  Equal<FactoryEventType, components["schemas"]["FactoryEventType"]>
>;
type _FactoryDefinitionAlias = Assert<
  Equal<FactoryDefinition, components["schemas"]["Factory"]>
>;
type _FactoryRecordingAlias = Assert<
  Equal<FactoryRecording, components["schemas"]["FactoryRecording"]>
>;

declare const apiPaths: paths;
declare const apiOperations: operations;
void apiPaths;
void apiOperations;
