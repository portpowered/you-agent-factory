import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkflowActivityGraphImportMessages {
  dialogCloseLabel: string;
  dialogFlowLabel: string;
  dismissAction: string;
  graphDropHint: string;
  graphDropReadingMessage: (fileName: string) => string;
  graphDropTitle: string;
  graphImportErrorTitle: string;
  graphImportLoadingTitle: string;
  importErrorEmbeddedMetadataInvalid: string;
  importErrorFileReadFailed: string;
  importErrorMetadataMissing: string;
  importErrorNotPngFile: string;
  importErrorPngInvalid: string;
  importErrorPreviewUnavailable: string;
  importErrorUnsupportedSchemaVersion: (schemaVersion?: string) => string;
}

const workflowActivityGraphImportMessagesByLocale = {
  en: {
    dialogCloseLabel: "Close dialog",
    dialogFlowLabel: "Mutation flow",
    dismissAction: "Dismiss",
    graphDropHint: "Drop an Infinite You PNG onto this graph to start import.",
    graphDropReadingMessage: (fileName) =>
      `${fileName} is being parsed and validated locally before import continues.`,
    graphDropTitle: "Import factory PNG",
    graphImportErrorTitle: "Factory import failed",
    graphImportLoadingTitle: "Validating factory PNG",
    importErrorEmbeddedMetadataInvalid:
      "The embedded Infinite You factory metadata is invalid, so the current factory was left unchanged.",
    importErrorFileReadFailed:
      "The browser could not read the dropped file. Try dropping the PNG again.",
    importErrorMetadataMissing:
      "This PNG does not include the Infinite You factory metadata needed for import.",
    importErrorNotPngFile: "Drop a PNG image exported by Infinite You.",
    importErrorPngInvalid:
      "This PNG appears truncated or malformed, so import stopped before any activation request.",
    importErrorPreviewUnavailable:
      "The browser could not validate this PNG for import preview, so the current factory was left unchanged.",
    importErrorUnsupportedSchemaVersion: (schemaVersion) =>
      schemaVersion
        ? `This PNG uses unsupported Infinite You factory metadata version ${schemaVersion}.`
        : "This PNG uses an unsupported Infinite You factory metadata version.",
  },
  ja: {
    dialogCloseLabel: "ダイアログを閉じる",
    dialogFlowLabel: "変更フロー",
    dismissAction: "閉じる",
    graphDropHint:
      "Infinite You の PNG をこのグラフにドロップしてインポートを開始してください。",
    graphDropReadingMessage: (fileName) =>
      `${fileName} をローカルで解析して検証しています。完了するとインポートが続行されます。`,
    graphDropTitle: "ファクトリー PNG をインポート",
    graphImportErrorTitle: "ファクトリーのインポートに失敗しました",
    graphImportLoadingTitle: "ファクトリー PNG を検証しています",
    importErrorEmbeddedMetadataInvalid:
      "埋め込まれた Infinite You ファクトリーメタデータが無効なため、現在のファクトリーは変更されませんでした。",
    importErrorFileReadFailed:
      "ブラウザがドロップしたファイルを読み取れませんでした。もう一度 PNG をドロップしてください。",
    importErrorMetadataMissing:
      "この PNG にはインポートに必要な Infinite You ファクトリーメタデータが含まれていません。",
    importErrorNotPngFile:
      "Infinite You から書き出した PNG 画像をドロップしてください。",
    importErrorPngInvalid:
      "この PNG は途中で切れているか不正な形式のため、アクティベーション要求前にインポートを停止しました。",
    importErrorPreviewUnavailable:
      "ブラウザがこの PNG をインポートプレビュー用に検証できなかったため、現在のファクトリーは変更されませんでした。",
    importErrorUnsupportedSchemaVersion: (schemaVersion) =>
      schemaVersion
        ? `この PNG は未対応の Infinite You ファクトリーメタデータバージョン ${schemaVersion} を使用しています。`
        : "この PNG は未対応の Infinite You ファクトリーメタデータバージョンを使用しています。",
  },
  ko: {
    dialogCloseLabel: "대화상자 닫기",
    dialogFlowLabel: "변경 흐름",
    dismissAction: "닫기",
    graphDropHint:
      "이 그래프에 Infinite You PNG를 놓아 가져오기를 시작하세요.",
    graphDropReadingMessage: (fileName) =>
      `${fileName} 파일을 로컬에서 파싱하고 검증하는 중이며, 완료되면 가져오기가 계속됩니다.`,
    graphDropTitle: "팩토리 PNG 가져오기",
    graphImportErrorTitle: "팩토리 가져오기에 실패했습니다",
    graphImportLoadingTitle: "팩토리 PNG를 검증하는 중",
    importErrorEmbeddedMetadataInvalid:
      "내장된 Infinite You 팩토리 메타데이터가 잘못되어 현재 팩토리는 변경되지 않았습니다.",
    importErrorFileReadFailed:
      "브라우저가 드롭한 파일을 읽지 못했습니다. PNG를 다시 드롭해 보세요.",
    importErrorMetadataMissing:
      "이 PNG에는 가져오기에 필요한 Infinite You 팩토리 메타데이터가 없습니다.",
    importErrorNotPngFile:
      "Infinite You에서 내보낸 PNG 이미지를 드롭하세요.",
    importErrorPngInvalid:
      "이 PNG는 잘렸거나 손상된 것 같아서 활성화 요청 전에 가져오기가 중단되었습니다.",
    importErrorPreviewUnavailable:
      "브라우저가 이 PNG를 가져오기 미리보기용으로 검증하지 못해 현재 팩토리는 변경되지 않았습니다.",
    importErrorUnsupportedSchemaVersion: (schemaVersion) =>
      schemaVersion
        ? `이 PNG는 지원되지 않는 Infinite You 팩토리 메타데이터 버전 ${schemaVersion}을 사용합니다.`
        : "이 PNG는 지원되지 않는 Infinite You 팩토리 메타데이터 버전을 사용합니다.",
  },
  zh: {
    dialogCloseLabel: "关闭对话框",
    dialogFlowLabel: "变更流程",
    dismissAction: "关闭",
    graphDropHint: "将 Infinite You PNG 拖放到此图上即可开始导入。",
    graphDropReadingMessage: (fileName) =>
      `${fileName} 正在本地解析并校验，完成后会继续导入。`,
    graphDropTitle: "导入工厂 PNG",
    graphImportErrorTitle: "工厂导入失败",
    graphImportLoadingTitle: "正在校验工厂 PNG",
    importErrorEmbeddedMetadataInvalid:
      "嵌入的 Infinite You 工厂元数据无效，因此当前工厂未被更改。",
    importErrorFileReadFailed:
      "浏览器无法读取拖入的文件。请再次拖入该 PNG。",
    importErrorMetadataMissing:
      "此 PNG 不包含导入所需的 Infinite You 工厂元数据。",
    importErrorNotPngFile: "请拖入从 Infinite You 导出的 PNG 图片。",
    importErrorPngInvalid:
      "此 PNG 似乎已截断或损坏，因此导入在任何启用请求之前就已停止。",
    importErrorPreviewUnavailable:
      "浏览器无法为导入预览校验此 PNG，因此当前工厂未被更改。",
    importErrorUnsupportedSchemaVersion: (schemaVersion) =>
      schemaVersion
        ? `此 PNG 使用了不受支持的 Infinite You 工厂元数据版本 ${schemaVersion}。`
        : "此 PNG 使用了不受支持的 Infinite You 工厂元数据版本。",
  },
} satisfies LocalizedMessages<WorkflowActivityGraphImportMessages>;

export function getWorkflowActivityGraphImportMessages(
  locale?: string | null,
): WorkflowActivityGraphImportMessages {
  return resolveLocalizedMessages(
    workflowActivityGraphImportMessagesByLocale,
    locale,
  );
}

export { workflowActivityGraphImportMessagesByLocale };
