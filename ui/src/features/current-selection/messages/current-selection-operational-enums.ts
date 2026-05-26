import {
  localizeEnumLabel,
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CurrentSelectionOperationalEnumMessages {
  localizeOutcome: (value: string) => string;
  localizeRelationState: (value: string) => string;
}

const currentSelectionOperationalEnumMessagesByLocale = {
  en: {
    localizeOutcome: (value: string) =>
      localizeEnumLabel({
        category: "outcome",
        labels: {
          ACCEPTED: "Accepted",
          CONTINUE: "Continue",
          FAILED: "Failed",
          FAILED_EXIT_CODE: "Failed exit code",
          PENDING: "Pending",
          RECORDED: "Recorded",
          REJECTED: "Rejected",
          SUCCEEDED: "Succeeded",
          TIMED_OUT: "Timed out",
        },
        locale: "en",
        value,
      }),
    localizeRelationState: (value: string) =>
      localizeEnumLabel({
        category: "status",
        labels: {
          approved: "Approved",
          ready: "Ready",
        },
        locale: "en",
        value,
      }),
  },
  ja: {
    localizeOutcome: (value: string) =>
      localizeEnumLabel({
        category: "outcome",
        labels: {
          ACCEPTED: "受理済み",
          CONTINUE: "継続",
          FAILED: "失敗",
          FAILED_EXIT_CODE: "終了コード失敗",
          PENDING: "保留中",
          RECORDED: "記録済み",
          REJECTED: "却下",
          SUCCEEDED: "成功",
          TIMED_OUT: "タイムアウト",
        },
        locale: "ja",
        value,
      }),
    localizeRelationState: (value: string) =>
      localizeEnumLabel({
        category: "status",
        labels: {
          approved: "承認済み",
          ready: "準備完了",
        },
        locale: "ja",
        value,
      }),
  },
  ko: {
    localizeOutcome: (value: string) =>
      localizeEnumLabel({
        category: "outcome",
        labels: {
          ACCEPTED: "수락됨",
          CONTINUE: "계속",
          FAILED: "실패",
          FAILED_EXIT_CODE: "종료 코드 실패",
          PENDING: "대기 중",
          RECORDED: "기록됨",
          REJECTED: "거부됨",
          SUCCEEDED: "성공",
          TIMED_OUT: "시간 초과",
        },
        locale: "ko",
        value,
      }),
    localizeRelationState: (value: string) =>
      localizeEnumLabel({
        category: "status",
        labels: {
          approved: "승인됨",
          ready: "준비됨",
        },
        locale: "ko",
        value,
      }),
  },
  "zh-CN": {
    localizeOutcome: (value: string) =>
      localizeEnumLabel({
        category: "outcome",
        labels: {
          ACCEPTED: "已接受",
          CONTINUE: "继续",
          FAILED: "失败",
          FAILED_EXIT_CODE: "退出码失败",
          PENDING: "等待中",
          RECORDED: "已记录",
          REJECTED: "已拒绝",
          SUCCEEDED: "成功",
          TIMED_OUT: "已超时",
        },
        locale: "zh-CN",
        value,
      }),
    localizeRelationState: (value: string) =>
      localizeEnumLabel({
        category: "status",
        labels: {
          approved: "已批准",
          ready: "就绪",
        },
        locale: "zh-CN",
        value,
      }),
  },
} satisfies LocalizedMessageCatalog<CurrentSelectionOperationalEnumMessages>;

export function getCurrentSelectionOperationalEnumMessages(
  locale?: string | null,
): CurrentSelectionOperationalEnumMessages {
  return resolveLocalizedMessages(
    currentSelectionOperationalEnumMessagesByLocale,
    locale,
  );
}
