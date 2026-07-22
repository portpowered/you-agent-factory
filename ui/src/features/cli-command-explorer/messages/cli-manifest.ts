import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CliManifestMessages {
  contractFailure: (path: string, detail: string) => string;
  schemaTypeConstraint: (expectedType: string) => string;
  schemaConstraint: (keyword: string) => string;
  requiredField: (field: string) => string;
  duplicateId: (id: string, firstPath: string) => string;
  commandKeyMismatch: (key: string, id: string) => string;
  inputKeyMismatch: (key: string, id: string) => string;
  argumentPosition: (expected: number) => string;
  finalArgumentVariadic: () => string;
  argumentCardinality: (id: string) => string;
  relationshipParticipant: (id: string, type: string) => string;
  duplicateCommandPath: (path: string) => string;
  commandPathName: (path: string, name: string) => string;
  missingRootCommand: (path: string) => string;
  missingParentCommand: (path: string) => string;
  commandOutsideRoot: (path: string, root: string) => string;
  inheritedFlagContradiction: (name: string) => string;
  inheritedFlagMissing: (name: string) => string;
  invalidRootSpacing: () => string;
}

const cliManifestMessagesByLocale = {
  en: {
    contractFailure: (path, detail) =>
      `Expected ${path} to satisfy the CLI manifest contract: ${detail}.`,
    schemaTypeConstraint: (expectedType) => `must be ${expectedType}`,
    schemaConstraint: (keyword) => `must satisfy the ${keyword} constraint`,
    requiredField: (field) => `Expected required field ${field}.`,
    duplicateId: (id, firstPath) => `Stable id ${id} duplicates ${firstPath}.`,
    commandKeyMismatch: (key, id) =>
      `Command key ${key} must match stable id ${id}.`,
    inputKeyMismatch: (key, id) =>
      `Input key ${key} must match stable id ${id}.`,
    argumentPosition: (expected) =>
      `Argument positions must be unique and contiguous from zero; expected ${expected}.`,
    finalArgumentVariadic: () =>
      "Only the final positional argument may be variadic.",
    argumentCardinality: (id) =>
      `Argument ${id} has contradictory required, minimum, maximum, or variadic cardinality.`,
    relationshipParticipant: (id, type) =>
      `Relationship participant ${id} does not resolve to a ${type} on this command.`,
    duplicateCommandPath: (path) => `Command path ${path} is duplicated.`,
    commandPathName: (path, name) =>
      `Command path ${path} must contain non-empty segments and end with command name ${name}.`,
    missingRootCommand: (path) =>
      `Root path ${path} does not resolve to a command.`,
    missingParentCommand: (path) =>
      `Parent command path ${path || "<empty>"} does not resolve.`,
    commandOutsideRoot: (path, root) =>
      `Command path ${path} is outside root ${root}.`,
    inheritedFlagContradiction: (name) =>
      `Inherited flag --${name} contradicts its persistent ancestor definition.`,
    inheritedFlagMissing: (name) =>
      `Inherited flag --${name} has no persistent ancestor definition.`,
    invalidRootSpacing: () =>
      "Root path must contain non-empty command segments separated by single spaces.",
  },
  "zh-CN": {
    contractFailure: (path, detail) =>
      `${path} 应符合 CLI 清单契约：${detail}。`,
    schemaTypeConstraint: (expectedType) => {
      const labels: Readonly<Record<string, string>> = {
        array: "数组",
        boolean: "布尔值",
        integer: "整数",
        null: "空值",
        number: "数字",
        object: "对象",
        string: "字符串",
      };
      return `必须是${labels[expectedType] ?? expectedType}`;
    },
    schemaConstraint: (keyword) => `必须满足 ${keyword} 约束`,
    requiredField: (field) => `缺少必填字段 ${field}。`,
    duplicateId: (id, firstPath) => `稳定标识 ${id} 与 ${firstPath} 重复。`,
    commandKeyMismatch: (key, id) =>
      `命令键 ${key} 必须与稳定标识 ${id} 一致。`,
    inputKeyMismatch: (key, id) => `输入键 ${key} 必须与稳定标识 ${id} 一致。`,
    argumentPosition: (expected) =>
      `参数位置必须从零开始连续且不重复；此处应为 ${expected}。`,
    finalArgumentVariadic: () => `只有最后一个位置参数可以是可变参数。`,
    argumentCardinality: (id) =>
      `参数 ${id} 的必填、最小值、最大值或可变参数基数相互矛盾。`,
    relationshipParticipant: (id, type) =>
      `关系参与者 ${id} 无法解析为此命令的 ${type} 输入。`,
    duplicateCommandPath: (path) => `命令路径 ${path} 重复。`,
    commandPathName: (path, name) =>
      `命令路径 ${path} 必须由非空段组成，并以命令名 ${name} 结尾。`,
    missingRootCommand: (path) => `根路径 ${path} 无法解析为命令。`,
    missingParentCommand: (path) => `父命令路径 ${path || "<空>"} 无法解析。`,
    commandOutsideRoot: (path, root) =>
      `命令路径 ${path} 不在根路径 ${root} 下。`,
    inheritedFlagContradiction: (name) =>
      `继承标志 --${name} 与其持久祖先定义相矛盾。`,
    inheritedFlagMissing: (name) =>
      `继承标志 --${name} 没有对应的持久祖先定义。`,
    invalidRootSpacing: () => `根路径必须由单个空格分隔的非空命令段组成。`,
  },
} satisfies LocalizedMessageCatalog<CliManifestMessages>;

export function getCliManifestMessages(locale?: string | null) {
  return resolveLocalizedMessages(cliManifestMessagesByLocale, locale);
}
