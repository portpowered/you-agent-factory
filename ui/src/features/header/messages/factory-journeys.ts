import type { LocalizedMessages } from "../../../i18n";

export interface HeaderFactoryJourneyMessages {
  closeDialogLabel: string;
  openSessionTargetLabel: string;
  openSessionTargetPendingLabel: string;
  openFactorySuccessLabel: string;
  newFactoryPreviewTitle: string;
  newFactoryPreviewDescriptionTemplate: string;
  newFactoryCancelLabel: string;
  newFactoryConfirmLabel: string;
  newFactoryConfirmPendingLabel: string;
  newFactorySuccessLabel: string;
}

export const headerFactoryJourneyMessagesByLocale = {
  en: {
    closeDialogLabel: "Close dialog",
    openSessionTargetLabel: "Open selected target",
    openSessionTargetPendingLabel: "Opening target...",
    openFactorySuccessLabel: "Factory opened successfully.",
    newFactoryPreviewTitle: "New Factory preview",
    newFactoryPreviewDescriptionTemplate:
      "Validation found a folder where a new Factory can be created. Confirmation will create exactly this target:",
    newFactoryCancelLabel: "Cancel",
    newFactoryConfirmLabel: "Create Factory",
    newFactoryConfirmPendingLabel: "Creating Factory...",
    newFactorySuccessLabel: "New Factory created and opened successfully.",
  },
  ja: {
    closeDialogLabel: "ダイアログを閉じる",
    openSessionTargetLabel: "選択したターゲットを開く",
    openSessionTargetPendingLabel: "ターゲットを開いています...",
    openFactorySuccessLabel: "ファクトリーを開きました。",
    newFactoryPreviewTitle: "新しいファクトリーのプレビュー",
    newFactoryPreviewDescriptionTemplate:
      "新しいファクトリーを作成できるフォルダーを確認しました。確認すると、次のターゲットだけを作成します:",
    newFactoryCancelLabel: "キャンセル",
    newFactoryConfirmLabel: "ファクトリーを作成",
    newFactoryConfirmPendingLabel: "ファクトリーを作成中...",
    newFactorySuccessLabel: "新しいファクトリーを作成して開きました。",
  },
  ko: {
    closeDialogLabel: "대화 상자 닫기",
    openSessionTargetLabel: "선택한 대상 열기",
    openSessionTargetPendingLabel: "대상을 여는 중...",
    openFactorySuccessLabel: "팩토리를 열었습니다.",
    newFactoryPreviewTitle: "새 팩토리 미리 보기",
    newFactoryPreviewDescriptionTemplate:
      "새 팩토리를 만들 수 있는 폴더를 확인했습니다. 확인하면 다음 대상만 정확히 만듭니다:",
    newFactoryCancelLabel: "취소",
    newFactoryConfirmLabel: "팩토리 만들기",
    newFactoryConfirmPendingLabel: "팩토리를 만드는 중...",
    newFactorySuccessLabel: "새 팩토리를 만들고 열었습니다.",
  },
  "zh-CN": {
    closeDialogLabel: "关闭对话框",
    openSessionTargetLabel: "打开所选目标",
    openSessionTargetPendingLabel: "正在打开目标...",
    openFactorySuccessLabel: "工厂已成功打开。",
    newFactoryPreviewTitle: "新工厂预览",
    newFactoryPreviewDescriptionTemplate:
      "已验证此文件夹可以创建新工厂。确认后将只创建以下确切目标:",
    newFactoryCancelLabel: "取消",
    newFactoryConfirmLabel: "创建工厂",
    newFactoryConfirmPendingLabel: "正在创建工厂...",
    newFactorySuccessLabel: "新工厂已创建并打开。",
  },
} satisfies LocalizedMessages<HeaderFactoryJourneyMessages>;
