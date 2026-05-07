import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface ExportDialogMessages {
  cancelAction: string;
  closeAction: string;
  closeLabel: string;
  description: string;
  exportAction: string;
  exportUnavailable: string;
  exportingAction: string;
  hint: string;
  imageDescription: string;
  imageLabel: string;
  imageRequiredValidation: string;
  imageTypeValidation: string;
  loadingStatus: string;
  nameDescription: string;
  nameLabel: string;
  namePlaceholder: string;
  nameRequiredValidation: string;
  selectedImageLabel: (imageName: string) => string;
  successMessage: (filename: string) => string;
  title: string;
  triggerLabel: string;
}

const exportDialogMessagesByLocale = {
  en: {
    cancelAction: "Cancel",
    closeAction: "Close",
    closeLabel: "Close export dialog",
    description:
      "Package the current factory into a PNG artifact without changing the live dashboard state.",
    exportAction: "Export PNG",
    exportUnavailable:
      "The current factory definition is not available for export yet.",
    exportingAction: "Exporting...",
    hint: "Confirming export keeps the current dashboard state unchanged and downloads a PNG artifact with embedded Infinite You factory metadata.",
    imageDescription:
      "Choose the image customers will see when they open the exported PNG.",
    imageLabel: "Cover image",
    imageRequiredValidation: "Choose a cover image before exporting.",
    imageTypeValidation: "Choose an image file before exporting.",
    loadingStatus: "Loading the current authored factory definition.",
    nameDescription:
      "This name is embedded in the exported Infinite You PNG metadata and used for the downloaded filename.",
    nameLabel: "Factory name",
    namePlaceholder: "Factory name",
    nameRequiredValidation: "Enter a factory name before exporting.",
    selectedImageLabel: (imageName) => `Selected image: ${imageName}`,
    successMessage: (filename) =>
      `Downloaded ${filename}. You can close this dialog or export another PNG with a different name or cover image.`,
    title: "Export factory",
    triggerLabel: "Export PNG",
  },
  ja: {
    cancelAction: "キャンセル",
    closeAction: "閉じる",
    closeLabel: "エクスポートダイアログを閉じる",
    description:
      "ライブのダッシュボード状態を変更せずに、現在のファクトリーを PNG アーティファクトとして書き出します。",
    exportAction: "PNG をエクスポート",
    exportUnavailable: "現在のファクトリー定義はまだエクスポートできません。",
    exportingAction: "エクスポートしています...",
    hint: "エクスポートを確定しても現在のダッシュボード状態は変わらず、Infinite You のファクトリーメタデータを埋め込んだ PNG アーティファクトがダウンロードされます。",
    imageDescription:
      "エクスポートした PNG を開いたときに顧客へ表示する画像を選択してください。",
    imageLabel: "カバー画像",
    imageRequiredValidation:
      "エクスポートする前にカバー画像を選択してください。",
    imageTypeValidation: "エクスポートする前に画像ファイルを選択してください。",
    loadingStatus: "現在作成中のファクトリー定義を読み込んでいます。",
    nameDescription:
      "この名前はエクスポートされた Infinite You PNG メタデータに埋め込まれ、ダウンロードするファイル名にも使われます。",
    nameLabel: "ファクトリー名",
    namePlaceholder: "ファクトリー名",
    nameRequiredValidation:
      "エクスポートする前にファクトリー名を入力してください。",
    selectedImageLabel: (imageName) => `選択した画像: ${imageName}`,
    successMessage: (filename) =>
      `${filename} をダウンロードしました。このダイアログを閉じるか、別の名前やカバー画像で別の PNG をエクスポートできます。`,
    title: "ファクトリーをエクスポート",
    triggerLabel: "PNG をエクスポート",
  },
  ko: {
    cancelAction: "취소",
    closeAction: "닫기",
    closeLabel: "내보내기 대화상자 닫기",
    description:
      "라이브 대시보드 상태를 변경하지 않고 현재 팩토리를 PNG 아티팩트로 내보냅니다.",
    exportAction: "PNG 내보내기",
    exportUnavailable: "현재 팩토리 정의는 아직 내보낼 수 없습니다.",
    exportingAction: "내보내는 중...",
    hint: "내보내기를 확인해도 현재 대시보드 상태는 바뀌지 않으며 Infinite You 팩토리 메타데이터가 포함된 PNG 아티팩트를 다운로드합니다.",
    imageDescription: "내보낸 PNG를 열 때 고객에게 보여줄 이미지를 선택하세요.",
    imageLabel: "커버 이미지",
    imageRequiredValidation: "내보내기 전에 커버 이미지를 선택하세요.",
    imageTypeValidation: "내보내기 전에 이미지 파일을 선택하세요.",
    loadingStatus: "현재 작성된 팩토리 정의를 불러오는 중입니다.",
    nameDescription:
      "이 이름은 내보낸 Infinite You PNG 메타데이터에 포함되며 다운로드 파일 이름에도 사용됩니다.",
    nameLabel: "팩토리 이름",
    namePlaceholder: "팩토리 이름",
    nameRequiredValidation: "내보내기 전에 팩토리 이름을 입력하세요.",
    selectedImageLabel: (imageName) => `선택한 이미지: ${imageName}`,
    successMessage: (filename) =>
      `${filename}을(를) 다운로드했습니다. 이 대화상자를 닫거나 다른 이름이나 커버 이미지로 다른 PNG를 내보낼 수 있습니다.`,
    title: "팩토리 내보내기",
    triggerLabel: "PNG 내보내기",
  },
  zh: {
    cancelAction: "取消",
    closeAction: "关闭",
    closeLabel: "关闭导出对话框",
    description: "在不更改当前仪表板状态的情况下，将当前工厂打包为 PNG 产物。",
    exportAction: "导出 PNG",
    exportUnavailable: "当前工厂定义暂时无法导出。",
    exportingAction: "正在导出...",
    hint: "确认导出不会更改当前仪表板状态，并会下载一个嵌入 Infinite You 工厂元数据的 PNG 产物。",
    imageDescription: "选择客户打开导出的 PNG 时会看到的图片。",
    imageLabel: "封面图片",
    imageRequiredValidation: "请在导出前选择封面图片。",
    imageTypeValidation: "请在导出前选择图片文件。",
    loadingStatus: "正在加载当前编写的工厂定义。",
    nameDescription:
      "该名称会嵌入导出的 Infinite You PNG 元数据中，并用于下载文件名。",
    nameLabel: "工厂名称",
    namePlaceholder: "工厂名称",
    nameRequiredValidation: "请在导出前输入工厂名称。",
    selectedImageLabel: (imageName) => `已选择图片：${imageName}`,
    successMessage: (filename) =>
      `已下载 ${filename}。您可以关闭此对话框，或使用其他名称或封面图片再导出一个 PNG。`,
    title: "导出工厂",
    triggerLabel: "导出 PNG",
  },
} satisfies LocalizedMessages<ExportDialogMessages>;

export function getExportDialogMessages(
  locale?: string | null,
): ExportDialogMessages {
  return resolveLocalizedMessages(exportDialogMessagesByLocale, locale);
}

export { exportDialogMessagesByLocale };
