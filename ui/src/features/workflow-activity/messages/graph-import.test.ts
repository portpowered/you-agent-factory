import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  getWorkflowActivityGraphImportMessages,
  workflowActivityGraphImportMessagesByLocale,
} from "./graph-import";

describe("getWorkflowActivityGraphImportMessages", () => {
  it("supports the expected workflow-activity graph-import locales", () => {
    expect(
      Object.keys(workflowActivityGraphImportMessagesByLocale).sort(),
    ).toEqual([...SUPPORTED_LOCALES].sort());
  });

  it.each([
    ["en", "Import factory PNG", "Dismiss", "Close dialog", "Mutation flow"],
    ["zh", "导入工厂 PNG", "关闭", "关闭对话框", "变更流程"],
    ["ko", "팩토리 PNG 가져오기", "닫기", "대화상자 닫기", "변경 흐름"],
    ["ja", "ファクトリー PNG をインポート", "閉じる", "ダイアログを閉じる", "変更フロー"],
  ] as const)(
    "resolves %s graph-import shell copy",
    (locale, expectedTitle, expectedDismiss, expectedCloseLabel, expectedFlowLabel) => {
      const messages = getWorkflowActivityGraphImportMessages(locale);

      expect(messages.graphDropTitle).toBe(expectedTitle);
      expect(messages.dismissAction).toBe(expectedDismiss);
      expect(messages.dialogCloseLabel).toBe(expectedCloseLabel);
      expect(messages.dialogFlowLabel).toBe(expectedFlowLabel);
    },
  );

  it.each([
    [
      "en",
      "factory.png is being parsed and validated locally before import continues.",
      "This PNG uses unsupported Infinite You factory metadata version portos.agent-factory.png.v9.",
    ],
    [
      "zh",
      "factory.png 正在本地解析并校验，完成后会继续导入。",
      "此 PNG 使用了不受支持的 Infinite You 工厂元数据版本 portos.agent-factory.png.v9。",
    ],
    [
      "ko",
      "factory.png 파일을 로컬에서 파싱하고 검증하는 중이며, 완료되면 가져오기가 계속됩니다.",
      "이 PNG는 지원되지 않는 Infinite You 팩토리 메타데이터 버전 portos.agent-factory.png.v9을 사용합니다.",
    ],
    [
      "ja",
      "factory.png をローカルで解析して検証しています。完了するとインポートが続行されます。",
      "この PNG は未対応の Infinite You ファクトリーメタデータバージョン portos.agent-factory.png.v9 を使用しています。",
    ],
  ] as const)(
    "resolves %s helper copy",
    (locale, expectedReadingMessage, expectedUnsupportedVersion) => {
      const messages = getWorkflowActivityGraphImportMessages(locale);

      expect(messages.graphDropReadingMessage("factory.png")).toBe(
        expectedReadingMessage,
      );
      expect(
        messages.importErrorUnsupportedSchemaVersion(
          "portos.agent-factory.png.v9",
        ),
      ).toBe(expectedUnsupportedVersion);
    },
  );

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getWorkflowActivityGraphImportMessages("en");

    expect(getWorkflowActivityGraphImportMessages(undefined).graphDropTitle).toBe(
      defaultMessages.graphDropTitle,
    );
    expect(getWorkflowActivityGraphImportMessages("fr").dialogCloseLabel).toBe(
      defaultMessages.dialogCloseLabel,
    );
  });
});
