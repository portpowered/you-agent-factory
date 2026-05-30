import type { NamedFactoryAPIErrorCode } from "../../../api/named-factory";
import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

type MappedImportPreviewErrorCode = Extract<
  NamedFactoryAPIErrorCode,
  | "FACTORY_ALREADY_EXISTS"
  | "FACTORY_NOT_IDLE"
  | "INVALID_FACTORY"
  | "INVALID_FACTORY_NAME"
  | "NETWORK_ERROR"
>;

export interface ImportPreviewDialogMessages {
  activateAction: string;
  activatingAction: string;
  activationErrorTitle: string;
  cancelAction: string;
  closeLabel: string;
  createNewNamedFactoryDescription: string;
  createNewNamedFactoryLabel: string;
  createNewNamedFactoryResolvedNameLabel: string;
  descriptionTemplate: string;
  droppedFileLabel: string;
  embeddedFactoryLabel: string;
  errorByCode: Record<MappedImportPreviewErrorCode, string>;
  flowLabel: string;
  hint: string;
  importSaveChoiceLegend: string;
  previewImageAlt: (factoryName: string) => string;
  replaceCurrentFactoryDescription: (currentFactoryName: string) => string;
  replaceCurrentFactoryLabel: string;
  title: string;
}

export const IMPORT_PREVIEW_FACTORY_NAME_TOKEN = "{{factoryName}}";

const importPreviewDialogMessagesByLocale = {
  en: {
    activateAction: "Activate factory",
    activatingAction: "Activating factory...",
    activationErrorTitle: "Activation failed",
    cancelAction: "Cancel import",
    closeLabel: "Close import preview",
    createNewNamedFactoryDescription:
      "Create a new named factory from the embedded PNG definition and activate it in this session.",
    createNewNamedFactoryLabel: "Create new named factory",
    createNewNamedFactoryResolvedNameLabel: "Resolved factory name",
    descriptionTemplate: `Review the dropped factory before activation. Confirming this import in the next step will switch the current factory to ${IMPORT_PREVIEW_FACTORY_NAME_TOKEN}.`,
    droppedFileLabel: "Dropped file",
    embeddedFactoryLabel: "Embedded factory",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "A factory with this name already exists. Rename or remove the existing factory before importing this PNG.",
      FACTORY_NOT_IDLE:
        "The current factory runtime is still active. Wait until it becomes idle before switching factories.",
      INVALID_FACTORY:
        "The dropped factory payload was rejected by the activation API.",
      INVALID_FACTORY_NAME:
        "The embedded factory name is not valid for activation.",
      NETWORK_ERROR:
        "The dashboard could not reach the activation API. Try again once the connection is available.",
    },
    flowLabel: "Mutation flow",
    hint: "Choose whether to replace the factory already current in this session or create a new named factory from the embedded PNG definition.",
    importSaveChoiceLegend: "Import activation mode",
    previewImageAlt: (factoryName) => `${factoryName} preview`,
    replaceCurrentFactoryDescription: (currentFactoryName) =>
      `Replace the factory already current in this session (${currentFactoryName}) with the embedded PNG definition without changing its name.`,
    replaceCurrentFactoryLabel: "Replace current factory",
    title: "Review factory import",
  },
  ja: {
    activateAction: "ファクトリーを有効化",
    activatingAction: "ファクトリーを有効化しています...",
    activationErrorTitle: "有効化に失敗しました",
    cancelAction: "インポートをキャンセル",
    closeLabel: "インポートのプレビューを閉じる",
    createNewNamedFactoryDescription:
      "埋め込まれた PNG 定義から新しい名前付きファクトリーを作成し、このセッションで有効化します。",
    createNewNamedFactoryLabel: "新しい名前付きファクトリーを作成",
    createNewNamedFactoryResolvedNameLabel: "解決されたファクトリー名",
    descriptionTemplate: `有効化する前に、ドロップしたファクトリーを確認してください。インポートを確定すると、現在のファクトリーは${IMPORT_PREVIEW_FACTORY_NAME_TOKEN}に切り替わります。`,
    droppedFileLabel: "ドロップしたファイル",
    embeddedFactoryLabel: "埋め込みファクトリー",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "同じ名前のファクトリーがすでに存在します。この PNG をインポートする前に、既存のファクトリーの名前を変更するか削除してください。",
      FACTORY_NOT_IDLE:
        "現在のファクトリーランタイムはまだ稼働中です。アイドル状態になるまで待ってから切り替えてください。",
      INVALID_FACTORY:
        "有効化 API がドロップしたファクトリーペイロードを拒否しました。",
      INVALID_FACTORY_NAME:
        "埋め込まれたファクトリー名は有効化に使用できません。",
      NETWORK_ERROR:
        "ダッシュボードが有効化 API に接続できませんでした。接続が復旧したら再試行してください。",
    },
    flowLabel: "変更フロー",
    hint: "このセッションで現在のファクトリーを置き換えるか、埋め込まれた PNG 定義から新しい名前付きファクトリーを作成するかを選択してください。",
    importSaveChoiceLegend: "インポート有効化モード",
    previewImageAlt: (factoryName) => `${factoryName} のプレビュー`,
    replaceCurrentFactoryDescription: (currentFactoryName) =>
      `このセッションで現在のファクトリー (${currentFactoryName}) を、名前を変えずに埋め込まれた PNG 定義で置き換えます。`,
    replaceCurrentFactoryLabel: "現在のファクトリーを置き換え",
    title: "ファクトリーのインポートを確認",
  },
  ko: {
    activateAction: "팩토리 활성화",
    activatingAction: "팩토리를 활성화하는 중...",
    activationErrorTitle: "활성화 실패",
    cancelAction: "가져오기 취소",
    closeLabel: "가져오기 미리보기 닫기",
    createNewNamedFactoryDescription:
      "내장된 PNG 정의로 새 이름의 팩토리를 만들고 이 세션에서 활성화합니다.",
    createNewNamedFactoryLabel: "새 이름의 팩토리 만들기",
    createNewNamedFactoryResolvedNameLabel: "확정된 팩토리 이름",
    descriptionTemplate: `드롭한 팩토리를 활성화 전에 검토하세요. 이 가져오기를 확인하면 현재 팩토리가 다음 단계에서 ${IMPORT_PREVIEW_FACTORY_NAME_TOKEN}로 전환됩니다.`,
    droppedFileLabel: "드롭한 파일",
    embeddedFactoryLabel: "내장된 팩토리",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "같은 이름의 팩토리가 이미 있습니다. 이 PNG를 가져오기 전에 기존 팩토리의 이름을 바꾸거나 제거하세요.",
      FACTORY_NOT_IDLE:
        "현재 팩토리 런타임이 아직 활성 상태입니다. 유휴 상태가 된 뒤에 팩토리를 전환하세요.",
      INVALID_FACTORY: "활성화 API가 드롭한 팩토리 페이로드를 거부했습니다.",
      INVALID_FACTORY_NAME: "내장된 팩토리 이름이 활성화에 유효하지 않습니다.",
      NETWORK_ERROR:
        "대시보드가 활성화 API에 연결할 수 없습니다. 연결이 복구된 뒤 다시 시도하세요.",
    },
    flowLabel: "변경 흐름",
    hint: "이 세션의 현재 팩토리를 교체할지, 아니면 내장된 PNG 정의로 새 이름의 팩토리를 만들지 선택하세요.",
    importSaveChoiceLegend: "가져오기 활성화 모드",
    previewImageAlt: (factoryName) => `${factoryName} 미리보기`,
    replaceCurrentFactoryDescription: (currentFactoryName) =>
      `이 세션의 현재 팩토리(${currentFactoryName})를 이름을 바꾸지 않고 내장된 PNG 정의로 교체합니다.`,
    replaceCurrentFactoryLabel: "현재 팩토리 교체",
    title: "팩토리 가져오기 검토",
  },
  "zh-CN": {
    activateAction: "启用工厂",
    activatingAction: "正在启用工厂...",
    activationErrorTitle: "启用失败",
    cancelAction: "取消导入",
    closeLabel: "关闭导入预览",
    createNewNamedFactoryDescription:
      "根据嵌入的 PNG 定义创建一个新的命名工厂，并在此会话中启用它。",
    createNewNamedFactoryLabel: "创建新的命名工厂",
    createNewNamedFactoryResolvedNameLabel: "解析后的工厂名称",
    descriptionTemplate: `请在启用前检查已拖入的工厂。确认导入后，当前工厂将切换为${IMPORT_PREVIEW_FACTORY_NAME_TOKEN}。`,
    droppedFileLabel: "拖入的文件",
    embeddedFactoryLabel: "嵌入的工厂",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "同名工厂已存在。请先重命名或移除现有工厂，再导入此 PNG。",
      FACTORY_NOT_IDLE:
        "当前工厂运行时仍处于活动状态。请等待其空闲后再切换工厂。",
      INVALID_FACTORY: "启用 API 拒绝了拖入的工厂负载。",
      INVALID_FACTORY_NAME: "嵌入的工厂名称不符合启用要求。",
      NETWORK_ERROR: "仪表板无法连接到启用 API。请在连接恢复后重试。",
    },
    flowLabel: "变更流程",
    hint: "选择是替换此会话中的当前工厂，还是根据嵌入的 PNG 定义创建新的命名工厂。",
    importSaveChoiceLegend: "导入启用模式",
    previewImageAlt: (factoryName) => `${factoryName} 预览图`,
    replaceCurrentFactoryDescription: (currentFactoryName) =>
      `替换此会话中的当前工厂（${currentFactoryName}），并使用嵌入的 PNG 定义，且不更改其名称。`,
    replaceCurrentFactoryLabel: "替换当前工厂",
    title: "检查工厂导入",
  },
} satisfies LocalizedMessages<ImportPreviewDialogMessages>;

export function getImportPreviewDialogMessages(
  locale?: string | null,
): ImportPreviewDialogMessages {
  return resolveLocalizedMessages(importPreviewDialogMessagesByLocale, locale);
}

export { importPreviewDialogMessagesByLocale };
