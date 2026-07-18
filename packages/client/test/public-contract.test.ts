import {
  type components,
  type FactoryDefinition,
  type FactoryEvent,
  type FactoryEventType,
  type FactoryRecording,
  type FactoryRecordingValidationError,
  type operations,
  parseFactoryRecording,
  type paths,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";

type Equal<Left, Right> =
  (<Value>() => Value extends Left ? 1 : 2) extends <
    Value,
  >() => Value extends Right ? 1 : 2
    ? true
    : false;
type Assert<Value extends true> = Value;
type FactoryLayoutEmptyState =
  components["schemas"]["FactoryLayoutEmptyState"];

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
type _FactoryLayoutEmptyStateRequiresVariant = Assert<
  Equal<{} extends FactoryLayoutEmptyState ? true : false, false>
>;
type _FactoryLayoutEmptyStateHasTextVariant = Assert<
  Equal<
    Extract<FactoryLayoutEmptyState, { text: string }> extends never
      ? false
      : true,
    true
  >
>;
type _FactoryLayoutEmptyStateHasImageVariant = Assert<
  Equal<
    Extract<
      FactoryLayoutEmptyState,
      { image: components["schemas"]["FactoryLayoutImage"] }
    > extends never
      ? false
      : true,
    true
  >
>;

declare const apiPaths: paths;
declare const apiOperations: operations;
void apiPaths;
void apiOperations;

declare const unknownRecording: unknown;
const parsedRecording: FactoryRecording =
  parseFactoryRecording(unknownRecording);
const safeResult = safeParseFactoryRecording(unknownRecording);
if (!safeResult.success) {
  const validationError: FactoryRecordingValidationError = safeResult.error;
  const issueCode: string = safeResult.issues[0]?.code ?? "";
  void validationError;
  void issueCode;
}
void parsedRecording;
