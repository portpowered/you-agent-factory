import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { ProviderSessionDetailMessages } from "./provider-session-detail-types";

const providerSessionDetailMessagesByLocale = {
  en: {
    emptyState:
      "The selected session file did not contain any Codex event records.",
    errorPrefix: "Session details could not be loaded.",
    eventCountLabel: "Parsed events",
    functionCallsHeading: "Function calls",
    functionCallsUnavailable: "No function or tool calls were recorded.",
    malformedLineCountLabel: "Malformed lines",
    missingState:
      "The selected provider-session file could not be found under the configured Codex sessions directory.",
    modifiedAtLabel: "Modified at",
    parseDiagnosticsHeading: "Parse diagnostics",
    parseErrorsHeading: "Malformed records",
    parseErrorState:
      "The selected session file could not be parsed into Codex events. Review the malformed-line diagnostics below.",
    reasoningHeading: "Reasoning",
    reasoningUnavailable: "No reasoning entries were recorded.",
    relativePathLabel: "Relative path",
    selectedSessionHeading: "Selected session details",
    sessionLabel: "Provider session",
    sizeBytesLabel: "File size",
    sourceHeading: "Source file",
    tokenUsageHeading: "Token usage",
    tokenUsageUnavailable: "Token usage was not reported for this session.",
    turnsHeading: "Execution turns",
    turnsUnavailable: "No execution turns were inferred from this session.",
    unknownEventCountLabel: "Unknown events",
  },
  ja: {
    emptyState:
      "選択したセッションファイルには Codex のイベント記録が含まれていませんでした。",
    errorPrefix: "セッション詳細を読み込めませんでした。",
    eventCountLabel: "解析済みイベント数",
    functionCallsHeading: "関数呼び出し",
    functionCallsUnavailable:
      "関数またはツールの呼び出しは記録されていません。",
    malformedLineCountLabel: "不正な行数",
    missingState:
      "選択した provider-session ファイルは、設定済み Codex sessions ディレクトリ配下に見つかりませんでした。",
    modifiedAtLabel: "更新日時",
    parseDiagnosticsHeading: "解析診断",
    parseErrorsHeading: "不正なレコード",
    parseErrorState:
      "選択したセッションファイルを Codex イベントとして解析できませんでした。以下の不正な行の診断を確認してください。",
    reasoningHeading: "推論",
    reasoningUnavailable: "推論エントリは記録されていません。",
    relativePathLabel: "相対パス",
    selectedSessionHeading: "選択中セッションの詳細",
    sessionLabel: "Provider session",
    sizeBytesLabel: "ファイルサイズ",
    sourceHeading: "ソースファイル",
    tokenUsageHeading: "トークン使用量",
    tokenUsageUnavailable:
      "このセッションではトークン使用量が報告されていません。",
    turnsHeading: "実行ターン",
    turnsUnavailable:
      "このセッションから推定できる実行ターンはありませんでした。",
    unknownEventCountLabel: "不明なイベント数",
  },
  ko: {
    emptyState:
      "선택한 세션 파일에 Codex 이벤트 레코드가 포함되어 있지 않습니다.",
    errorPrefix: "세션 세부 정보를 불러올 수 없습니다.",
    eventCountLabel: "파싱된 이벤트",
    functionCallsHeading: "함수 호출",
    functionCallsUnavailable:
      "함수 또는 도구 호출이 기록되지 않았습니다.",
    malformedLineCountLabel: "잘못된 줄 수",
    missingState:
      "선택한 provider-session 파일을 구성된 Codex sessions 디렉터리 아래에서 찾을 수 없습니다.",
    modifiedAtLabel: "수정 시각",
    parseDiagnosticsHeading: "파싱 진단",
    parseErrorsHeading: "잘못된 레코드",
    parseErrorState:
      "선택한 세션 파일을 Codex 이벤트로 파싱할 수 없습니다. 아래의 잘못된 줄 진단을 확인하세요.",
    reasoningHeading: "추론",
    reasoningUnavailable: "추론 항목이 기록되지 않았습니다.",
    relativePathLabel: "상대 경로",
    selectedSessionHeading: "선택한 세션 세부 정보",
    sessionLabel: "Provider session",
    sizeBytesLabel: "파일 크기",
    sourceHeading: "소스 파일",
    tokenUsageHeading: "토큰 사용량",
    tokenUsageUnavailable: "이 세션에는 토큰 사용량이 보고되지 않았습니다.",
    turnsHeading: "실행 턴",
    turnsUnavailable:
      "이 세션에서 추론할 수 있는 실행 턴이 없습니다.",
    unknownEventCountLabel: "알 수 없는 이벤트",
  },
  "zh-CN": {
    emptyState: "所选会话文件不包含任何 Codex 事件记录。",
    errorPrefix: "无法加载会话详情。",
    eventCountLabel: "已解析事件数",
    functionCallsHeading: "函数调用",
    functionCallsUnavailable: "没有记录任何函数或工具调用。",
    malformedLineCountLabel: "损坏行数",
    missingState:
      "无法在已配置的 Codex sessions 目录下找到所选 provider-session 文件。",
    modifiedAtLabel: "修改时间",
    parseDiagnosticsHeading: "解析诊断",
    parseErrorsHeading: "损坏记录",
    parseErrorState:
      "无法将所选会话文件解析为 Codex 事件。请检查下面的损坏行诊断。",
    reasoningHeading: "推理",
    reasoningUnavailable: "没有记录任何推理条目。",
    relativePathLabel: "相对路径",
    selectedSessionHeading: "已选会话详情",
    sessionLabel: "Provider session",
    sizeBytesLabel: "文件大小",
    sourceHeading: "源文件",
    tokenUsageHeading: "Token 用量",
    tokenUsageUnavailable: "该会话没有报告 token 用量。",
    turnsHeading: "执行轮次",
    turnsUnavailable: "无法从该会话推断出任何执行轮次。",
    unknownEventCountLabel: "未知事件数",
  },
} satisfies LocalizedMessages<ProviderSessionDetailMessages>;

export function getProviderSessionDetailMessages(locale?: string | null) {
  return resolveLocalizedMessages(providerSessionDetailMessagesByLocale, locale);
}
