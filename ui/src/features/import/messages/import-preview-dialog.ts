import type { NamedFactoryAPIErrorCode } from "../../../api/named-factory";
import { sessionFactoryOperatorErrorMessages } from "../../../api/session-factory";
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
  | "STALE_FACTORY_VERSION"
>;

export interface ImportPreviewDialogMessages {
  activateAction: string;
  activatingAction: string;
  activationErrorTitle: string;
  cancelAction: string;
  closeLabel: string;
  createNewNamedOption: string;
  createNewNamedOptionDescription: string;
  createResolvedNameLabel: string;
  descriptionTemplate: string;
  droppedFileLabel: string;
  embeddedFactoryLabel: string;
  errorByCode: Record<MappedImportPreviewErrorCode, string>;
  flowLabel: string;
  hint: string;
  previewImageAlt: (factoryName: string) => string;
  replaceCurrentOption: string;
  replaceCurrentOptionDescription: string;
  saveChoiceLegend: string;
  title: string;
}

export const IMPORT_PREVIEW_FACTORY_NAME_TOKEN = "{{factoryName}}";
export const IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN = "{{currentFactoryName}}";

const importPreviewDialogMessagesByLocale = {
  en: {
    activateAction: "Confirm import",
    activatingAction: "Activating factory...",
    activationErrorTitle: "Activation failed",
    cancelAction: "Cancel import",
    closeLabel: "Close import preview",
    createNewNamedOption: "Create new named factory",
    createNewNamedOptionDescription:
      "Save the imported definition as a new factory name in this session and switch to it.",
    createResolvedNameLabel: "New factory name",
    descriptionTemplate: `Review the dropped factory before confirming import. Choose whether to replace the current session factory (${IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN}) or create a new named factory.`,
    droppedFileLabel: "Dropped file",
    embeddedFactoryLabel: "Embedded factory",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "A factory with this name already exists. Rename or remove the existing factory before importing this PNG.",
      FACTORY_NOT_IDLE: sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE,
      INVALID_FACTORY: sessionFactoryOperatorErrorMessages.INVALID_FACTORY,
      INVALID_FACTORY_NAME:
        sessionFactoryOperatorErrorMessages.INVALID_FACTORY_NAME,
      NETWORK_ERROR:
        "The dashboard could not reach the activation API. Try again once the connection is available.",
      STALE_FACTORY_VERSION:
        sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION,
    },
    flowLabel: "Mutation flow",
    hint: "Replace current factory keeps the session factory name and updates its definition. Create new named factory saves under a separate name and activates it in this session.",
    previewImageAlt: (factoryName) => `${factoryName} preview`,
    replaceCurrentOption: "Replace current factory",
    replaceCurrentOptionDescription:
      "Overwrite the factory already current in this session while keeping its name.",
    saveChoiceLegend: "Import save choice",
    title: "Review factory import",
  },
  ja: {
    activateAction: "インポートを確定",
    activatingAction: "ファクトリーを有効化しています...",
    activationErrorTitle: "有効化に失敗しました",
    cancelAction: "インポートをキャンセル",
    closeLabel: "インポートのプレビューを閉じる",
    createNewNamedOption: "新しい名前のファクトリーを作成",
    createNewNamedOptionDescription:
      "インポートした定義をこのセッションの新しいファクトリー名として保存し、切り替えます。",
    createResolvedNameLabel: "新しいファクトリー名",
    descriptionTemplate: `インポートを確定する前に、ドロップしたファクトリーを確認してください。現在のセッションファクトリー（${IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN}）を置き換えるか、新しい名前のファクトリーを作成するかを選択します。`,
    droppedFileLabel: "ドロップしたファイル",
    embeddedFactoryLabel: "埋め込みファクトリー",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "同じ名前のファクトリーがすでに存在します。この PNG をインポートする前に、既存のファクトリーの名前を変更するか削除してください。",
      FACTORY_NOT_IDLE:
        "現在のファクトリーランタイムはまだ稼働中です。保存または切り替える前にアイドル状態になるまで待ってください。",
      INVALID_FACTORY:
        "セッションファクトリー API がファクトリー定義を拒否しました。",
      INVALID_FACTORY_NAME:
        "このセッションでは無効なファクトリー名です。",
      NETWORK_ERROR:
        "ダッシュボードが有効化 API に接続できませんでした。接続が復旧したら再試行してください。",
      STALE_FACTORY_VERSION:
        "ファクトリー定義が古くなっています。ダッシュボードを更新してから、保存またはインポートを再試行してください。",
    },
    flowLabel: "変更フロー",
    hint: "現在のファクトリーを置き換えると、セッションのファクトリー名を保ったまま定義を更新します。新しい名前のファクトリーを作成すると、別名で保存してこのセッションで有効化します。",
    previewImageAlt: (factoryName) => `${factoryName} のプレビュー`,
    replaceCurrentOption: "現在のファクトリーを置き換える",
    replaceCurrentOptionDescription:
      "このセッションで現在のファクトリー名を保ったまま、定義を上書きします。",
    saveChoiceLegend: "インポートの保存方法",
    title: "ファクトリーのインポートを確認",
  },
  ko: {
    activateAction: "가져오기 확인",
    activatingAction: "팩토리를 활성화하는 중...",
    activationErrorTitle: "활성화 실패",
    cancelAction: "가져오기 취소",
    closeLabel: "가져오기 미리보기 닫기",
    createNewNamedOption: "새 이름의 팩토리 만들기",
    createNewNamedOptionDescription:
      "가져온 정의를 이 세션의 새 팩토리 이름으로 저장하고 전환합니다.",
    createResolvedNameLabel: "새 팩토리 이름",
    descriptionTemplate: `가져오기를 확인하기 전에 드롭한 팩토리를 검토하세요. 현재 세션 팩토리(${IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN})를 바꿀지, 새 이름의 팩토리를 만들지 선택하세요.`,
    droppedFileLabel: "드롭한 파일",
    embeddedFactoryLabel: "내장된 팩토리",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "같은 이름의 팩토리가 이미 있습니다. 이 PNG를 가져오기 전에 기존 팩토리의 이름을 바꾸거나 제거하세요.",
      FACTORY_NOT_IDLE:
        "현재 팩토리 런타임이 아직 활성 상태입니다. 저장하거나 전환하기 전에 유휴 상태가 될 때까지 기다리세요.",
      INVALID_FACTORY:
        "세션 팩토리 API가 팩토리 정의를 거부했습니다.",
      INVALID_FACTORY_NAME: "이 세션에 유효하지 않은 팩토리 이름입니다.",
      NETWORK_ERROR:
        "대시보드가 활성화 API에 연결할 수 없습니다. 연결이 복구된 뒤 다시 시도하세요.",
      STALE_FACTORY_VERSION:
        "팩토리 정의가 오래되었습니다. 대시보드를 새로 고친 뒤 저장하거나 가져오기를 다시 시도하세요.",
    },
    flowLabel: "변경 흐름",
    hint: "현재 팩토리 바꾸기는 세션 팩토리 이름을 유지한 채 정의를 업데이트합니다. 새 이름의 팩토리 만들기는 별도 이름으로 저장한 뒤 이 세션에서 활성화합니다.",
    previewImageAlt: (factoryName) => `${factoryName} 미리보기`,
    replaceCurrentOption: "현재 팩토리 바꾸기",
    replaceCurrentOptionDescription:
      "이 세션의 현재 팩토리 이름을 유지한 채 정의를 덮어씁니다.",
    saveChoiceLegend: "가져오기 저장 방식",
    title: "팩토리 가져오기 검토",
  },
  "zh-CN": {
    activateAction: "确认导入",
    activatingAction: "正在启用工厂...",
    activationErrorTitle: "启用失败",
    cancelAction: "取消导入",
    closeLabel: "关闭导入预览",
    createNewNamedOption: "创建新的命名工厂",
    createNewNamedOptionDescription:
      "将导入的定义保存为此会话中的新工厂名称并切换过去。",
    createResolvedNameLabel: "新工厂名称",
    descriptionTemplate: `确认导入前请检查已拖入的工厂。选择是替换当前会话工厂（${IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN}），还是创建新的命名工厂。`,
    droppedFileLabel: "拖入的文件",
    embeddedFactoryLabel: "嵌入的工厂",
    errorByCode: {
      FACTORY_ALREADY_EXISTS:
        "同名工厂已存在。请先重命名或移除现有工厂，再导入此 PNG。",
      FACTORY_NOT_IDLE:
        "当前工厂运行时仍处于活动状态。请在保存或切换之前等待其进入空闲状态。",
      INVALID_FACTORY: "会话工厂 API 拒绝了该工厂定义。",
      INVALID_FACTORY_NAME: "该工厂名称对当前会话无效。",
      NETWORK_ERROR: "仪表板无法连接到启用 API。请在连接恢复后重试。",
      STALE_FACTORY_VERSION:
        "工厂定义已过期。请刷新仪表板后再保存或导入。",
    },
    flowLabel: "变更流程",
    hint: "替换当前工厂会保留会话工厂名称并更新其定义。创建新的命名工厂会以单独名称保存并在本会话中启用。",
    previewImageAlt: (factoryName) => `${factoryName} 预览图`,
    replaceCurrentOption: "替换当前工厂",
    replaceCurrentOptionDescription:
      "覆盖此会话中当前的工厂，同时保留其名称。",
    saveChoiceLegend: "导入保存方式",
    title: "检查工厂导入",
  },
} satisfies LocalizedMessages<ImportPreviewDialogMessages>;

export function getImportPreviewDialogMessages(
  locale?: string | null,
): ImportPreviewDialogMessages {
  return resolveLocalizedMessages(importPreviewDialogMessagesByLocale, locale);
}

export { importPreviewDialogMessagesByLocale };
