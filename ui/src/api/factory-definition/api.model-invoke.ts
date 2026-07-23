import type { components } from "../generated/openapi";
import {
  expectObject,
  FactoryDefinitionAPIError,
  readOptionalArray,
  readOptionalBoolean,
  readOptionalEnum,
  readOptionalObject,
  readOptionalString,
  readRequiredEnum,
  readRequiredEnumArray,
  readRequiredString,
  rejectUnknownKeys,
} from "./api.decode-helpers";

type FactorySchemas = components["schemas"];
type FactoryModelOperation = FactorySchemas["ModelOperation"];
type FactoryModelOperationSlot = FactorySchemas["ModelOperationSlot"];
type FactoryModelOperationContentType =
  FactorySchemas["ModelOperationContentType"];
type FactoryWorkContentCommonFields = FactorySchemas["WorkContentCommonFields"];
type FactoryWorkContentPart = FactorySchemas["WorkContentPart"];
type FactoryWorkTextContentPart = FactorySchemas["WorkTextContentPart"];
type FactoryWorkImageContentPart = FactorySchemas["WorkImageContentPart"];
type FactoryWorkAudioContentPart = FactorySchemas["WorkAudioContentPart"];
type FactoryWorkJsonContentPart = FactorySchemas["WorkJsonContentPart"];
type FactoryWorkBinaryContentPart = FactorySchemas["WorkBinaryContentPart"];
type FactoryWorkTextContentPartType =
  FactorySchemas["WorkTextContentPart"]["type"];
type FactoryWorkImageContentPartType =
  FactorySchemas["WorkImageContentPart"]["type"];
type FactoryWorkAudioContentPartType =
  FactorySchemas["WorkAudioContentPart"]["type"];
type FactoryWorkJsonContentPartType =
  FactorySchemas["WorkJsonContentPart"]["type"];
type FactoryWorkBinaryContentPartType =
  FactorySchemas["WorkBinaryContentPart"]["type"];
type FactoryWorkstationOperationBinding =
  FactorySchemas["WorkstationOperationBinding"];
type FactoryWorkstationOperationBindingSelector =
  FactorySchemas["WorkstationOperationBindingSelector"];

const MODEL_OPERATION_KEYS = new Set(["inputs", "name", "outputs"]);
const MODEL_OPERATION_SLOT_KEYS = new Set(["contentTypes", "name", "required"]);
const MODEL_OPERATION_CONTENT_TYPE_VALUES =
  new Set<FactoryModelOperationContentType>([
    "TEXT",
    "IMAGE",
    "AUDIO",
    "JSON",
    "BINARY",
  ]);
const WORKSTATION_OPERATION_BINDING_KEYS = new Set([
  "config",
  "defaultContent",
  "selector",
  "slot",
]);
const WORKSTATION_OPERATION_BINDING_SELECTOR_KEYS = new Set([
  "label",
  "role",
  "slot",
  "type",
]);
const WORK_CONTENT_COMMON_FIELD_KEYS = new Set([
  "artifactId",
  "contentType",
  "label",
  "metadata",
  "role",
  "slot",
]);
const WORK_TEXT_CONTENT_PART_KEYS = new Set(["text", "type"]);
const WORK_IMAGE_CONTENT_PART_KEYS = new Set(["file", "type", "url"]);
const WORK_AUDIO_CONTENT_PART_KEYS = new Set(["file", "type", "url"]);
const WORK_JSON_CONTENT_PART_KEYS = new Set(["json", "type"]);
const WORK_BINARY_CONTENT_PART_KEYS = new Set(["file", "type", "url"]);
const WORK_TEXT_CONTENT_PART_TYPE_VALUES = new Set<string>(["text", "TEXT"]);
const WORK_IMAGE_CONTENT_PART_TYPE_VALUES = new Set<string>(["image", "IMAGE"]);
const WORK_AUDIO_CONTENT_PART_TYPE_VALUES = new Set<string>(["AUDIO"]);
const WORK_JSON_CONTENT_PART_TYPE_VALUES = new Set<string>(["JSON"]);
const WORK_BINARY_CONTENT_PART_TYPE_VALUES = new Set<string>(["BINARY"]);
const WORK_TEXT_CONTENT_PART_ENUM_VALUES =
  new Set<FactoryWorkTextContentPartType>(["text", "TEXT"]);
const WORK_IMAGE_CONTENT_PART_ENUM_VALUES =
  new Set<FactoryWorkImageContentPartType>(["image", "IMAGE"]);
const WORK_AUDIO_CONTENT_PART_ENUM_VALUES =
  new Set<FactoryWorkAudioContentPartType>(["AUDIO"]);
const WORK_JSON_CONTENT_PART_ENUM_VALUES =
  new Set<FactoryWorkJsonContentPartType>(["JSON"]);
const WORK_BINARY_CONTENT_PART_ENUM_VALUES =
  new Set<FactoryWorkBinaryContentPartType>(["BINARY"]);

export function decodeModelOperation(
  value: unknown,
  path: string,
): FactoryModelOperation {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, MODEL_OPERATION_KEYS, path);

  const operation: FactoryModelOperation = {
    name: readRequiredString(record, "name", path),
  };
  const inputs = readOptionalArray(
    record,
    "inputs",
    path,
    decodeModelOperationSlot,
  );
  const outputs = readOptionalArray(
    record,
    "outputs",
    path,
    decodeModelOperationSlot,
  );
  if (inputs !== undefined) {
    operation.inputs = inputs;
  }
  if (outputs !== undefined) {
    operation.outputs = outputs;
  }
  return operation;
}

function decodeModelOperationSlot(
  value: unknown,
  path: string,
): FactoryModelOperationSlot {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, MODEL_OPERATION_SLOT_KEYS, path);

  const slot: FactoryModelOperationSlot = {
    name: readRequiredString(record, "name", path),
    contentTypes: readRequiredEnumArray(
      record,
      "contentTypes",
      path,
      MODEL_OPERATION_CONTENT_TYPE_VALUES,
    ),
  };
  const required = readOptionalBoolean(record, "required", path);
  if (required !== undefined) {
    slot.required = required;
  }
  return slot;
}

export function decodeWorkstationOperationBinding(
  value: unknown,
  path: string,
): FactoryWorkstationOperationBinding {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_OPERATION_BINDING_KEYS, path);

  const binding: FactoryWorkstationOperationBinding = {
    slot: readRequiredString(record, "slot", path),
  };
  const selector = readOptionalObject(
    record,
    "selector",
    path,
    decodeWorkstationOperationBindingSelector,
  );
  const config = readOptionalArray(
    record,
    "config",
    path,
    decodeWorkContentPart,
  );
  const defaultContent = readOptionalArray(
    record,
    "defaultContent",
    path,
    decodeWorkContentPart,
  );
  if (selector !== undefined) {
    binding.selector = selector;
  }
  if (config !== undefined) {
    binding.config = config;
  }
  if (defaultContent !== undefined) {
    binding.defaultContent = defaultContent;
  }
  return binding;
}

function decodeWorkstationOperationBindingSelector(
  value: unknown,
  path: string,
): FactoryWorkstationOperationBindingSelector {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_OPERATION_BINDING_SELECTOR_KEYS, path);

  const selector: FactoryWorkstationOperationBindingSelector = {};
  const slot = readOptionalString(record, "slot", path);
  const label = readOptionalString(record, "label", path);
  const type = readOptionalEnum(
    record,
    "type",
    path,
    MODEL_OPERATION_CONTENT_TYPE_VALUES,
  );
  const role = readOptionalString(record, "role", path);
  if (slot !== undefined) {
    selector.slot = slot;
  }
  if (label !== undefined) {
    selector.label = label;
  }
  if (type !== undefined) {
    selector.type = type;
  }
  if (role !== undefined) {
    selector.role = role;
  }
  return selector;
}

function decodeWorkContentPart(
  value: unknown,
  path: string,
): FactoryWorkContentPart {
  const record = expectObject(value, path);
  const type = readRequiredString(record, "type", path);

  if (WORK_TEXT_CONTENT_PART_TYPE_VALUES.has(type)) {
    return decodeWorkTextContentPart(record, path);
  }
  if (WORK_IMAGE_CONTENT_PART_TYPE_VALUES.has(type)) {
    return decodeWorkImageContentPart(record, path);
  }
  if (WORK_AUDIO_CONTENT_PART_TYPE_VALUES.has(type)) {
    return decodeWorkAudioContentPart(record, path);
  }
  if (WORK_JSON_CONTENT_PART_TYPE_VALUES.has(type)) {
    return decodeWorkJsonContentPart(record, path);
  }
  if (WORK_BINARY_CONTENT_PART_TYPE_VALUES.has(type)) {
    return decodeWorkBinaryContentPart(record, path);
  }

  throw new FactoryDefinitionAPIError(
    `${path}.type must be one of text, TEXT, image, IMAGE, AUDIO, JSON, BINARY.`,
  );
}

function decodeWorkTextContentPart(
  record: Record<string, unknown>,
  path: string,
): FactoryWorkTextContentPart {
  rejectUnknownKeys(
    record,
    new Set([
      ...WORK_CONTENT_COMMON_FIELD_KEYS,
      ...WORK_TEXT_CONTENT_PART_KEYS,
    ]),
    path,
  );
  const part: FactoryWorkTextContentPart = {
    type: readRequiredEnum(
      record,
      "type",
      path,
      WORK_TEXT_CONTENT_PART_ENUM_VALUES,
    ),
    text: readRequiredString(record, "text", path),
  };
  applyWorkContentCommonFields(record, path, part);
  return part;
}

function decodeWorkImageContentPart(
  record: Record<string, unknown>,
  path: string,
): FactoryWorkImageContentPart {
  rejectUnknownKeys(
    record,
    new Set([
      ...WORK_CONTENT_COMMON_FIELD_KEYS,
      ...WORK_IMAGE_CONTENT_PART_KEYS,
    ]),
    path,
  );
  const part: FactoryWorkImageContentPart = {
    type: readRequiredEnum(
      record,
      "type",
      path,
      WORK_IMAGE_CONTENT_PART_ENUM_VALUES,
    ),
    url: readRequiredString(record, "url", path),
  };
  const file = readOptionalString(record, "file", path);
  if (file !== undefined) {
    part.file = file;
  }
  applyWorkContentCommonFields(record, path, part);
  return part;
}

function decodeWorkAudioContentPart(
  record: Record<string, unknown>,
  path: string,
): FactoryWorkAudioContentPart {
  rejectUnknownKeys(
    record,
    new Set([
      ...WORK_CONTENT_COMMON_FIELD_KEYS,
      ...WORK_AUDIO_CONTENT_PART_KEYS,
    ]),
    path,
  );
  const part: FactoryWorkAudioContentPart = {
    type: readRequiredEnum(
      record,
      "type",
      path,
      WORK_AUDIO_CONTENT_PART_ENUM_VALUES,
    ),
    url: readRequiredString(record, "url", path),
  };
  const file = readOptionalString(record, "file", path);
  if (file !== undefined) {
    part.file = file;
  }
  applyWorkContentCommonFields(record, path, part);
  return part;
}

function decodeWorkJsonContentPart(
  record: Record<string, unknown>,
  path: string,
): FactoryWorkJsonContentPart {
  rejectUnknownKeys(
    record,
    new Set([
      ...WORK_CONTENT_COMMON_FIELD_KEYS,
      ...WORK_JSON_CONTENT_PART_KEYS,
    ]),
    path,
  );
  const part: FactoryWorkJsonContentPart = {
    type: readRequiredEnum(
      record,
      "type",
      path,
      WORK_JSON_CONTENT_PART_ENUM_VALUES,
    ),
    json: record.json,
  };
  applyWorkContentCommonFields(record, path, part);
  return part;
}

function decodeWorkBinaryContentPart(
  record: Record<string, unknown>,
  path: string,
): FactoryWorkBinaryContentPart {
  rejectUnknownKeys(
    record,
    new Set([
      ...WORK_CONTENT_COMMON_FIELD_KEYS,
      ...WORK_BINARY_CONTENT_PART_KEYS,
    ]),
    path,
  );
  const part: FactoryWorkBinaryContentPart = {
    type: readRequiredEnum(
      record,
      "type",
      path,
      WORK_BINARY_CONTENT_PART_ENUM_VALUES,
    ),
    url: readRequiredString(record, "url", path),
  };
  const file = readOptionalString(record, "file", path);
  if (file !== undefined) {
    part.file = file;
  }
  applyWorkContentCommonFields(record, path, part);
  return part;
}

function applyWorkContentCommonFields(
  record: Record<string, unknown>,
  path: string,
  part: FactoryWorkContentCommonFields,
): void {
  const slot = readOptionalString(record, "slot", path);
  const label = readOptionalString(record, "label", path);
  const role = readOptionalString(record, "role", path);
  const contentType = readOptionalString(record, "contentType", path);
  const artifactId = readOptionalString(record, "artifactId", path);
  const metadata = readOptionalObject(
    record,
    "metadata",
    path,
    decodeWorkContentMetadata,
  );
  if (slot !== undefined) {
    part.slot = slot;
  }
  if (label !== undefined) {
    part.label = label;
  }
  if (role !== undefined) {
    part.role = role;
  }
  if (contentType !== undefined) {
    part.contentType = contentType;
  }
  if (artifactId !== undefined) {
    part.artifactId = artifactId;
  }
  if (metadata !== undefined) {
    part.metadata = metadata;
  }
}

function decodeWorkContentMetadata(
  value: unknown,
  path: string,
): Record<string, unknown> {
  return expectObject(value, path);
}
