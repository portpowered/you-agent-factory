import type { LocalizedMessages } from "../../../i18n";

export interface HeaderFactoryJourneyMessages {
  closeDialogLabel: string;
  openFactoryNoExistingTargetError: string;
  openSessionTargetLabel: string;
  openSessionTargetPendingLabel: string;
  openFactorySuccessLabel: string;
  newFactoryButtonLabel: string;
  newFactoryDialogTitle: string;
  newFactoryDialogDescription: string;
  newFactoryExistingTargetError: string;
  newFactoryFolderFieldLabel: string;
  newFactoryFolderFieldPlaceholder: string;
  newFactoryFolderHelperText: string;
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
    openFactoryNoExistingTargetError:
      "No existing Factory was found in this folder. Choose New Factory to create one.",
    openSessionTargetLabel: "Open selected target",
    openSessionTargetPendingLabel: "Opening target...",
    openFactorySuccessLabel: "Factory opened successfully.",
    newFactoryButtonLabel: "New Factory",
    newFactoryDialogTitle: "New Factory",
    newFactoryDialogDescription:
      "Enter a parent folder for a new Factory. Validation only previews the exact target; nothing is created until you confirm.",
    newFactoryExistingTargetError:
      "This folder already contains a runnable Factory. Choose Open Factory to use it, or choose a different parent folder.",
    newFactoryFolderFieldLabel: "New Factory parent folder",
    newFactoryFolderFieldPlaceholder: "/home/bob/new-factory-project",
    newFactoryFolderHelperText:
      "Use an existing writable folder. The default scaffold will be created in its factory subfolder after confirmation.",
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
    openFactoryNoExistingTargetError:
      "このフォルダーに既存のファクトリーが見つかりません。作成するには「新しいファクトリー」を選んでください。",
    openSessionTargetLabel: "選択したターゲットを開く",
    openSessionTargetPendingLabel: "ターゲットを開いています...",
    openFactorySuccessLabel: "ファクトリーを開きました。",
    newFactoryButtonLabel: "新しいファクトリー",
    newFactoryDialogTitle: "新しいファクトリー",
    newFactoryDialogDescription:
      "新しいファクトリーの親フォルダーを入力してください。確認前の検証では正確な作成先だけを表示し、作成は行いません。",
    newFactoryExistingTargetError:
      "このフォルダーには実行可能なファクトリーがすでにあります。使うには「ファクトリーを開く」を選ぶか、別の親フォルダーを選んでください。",
    newFactoryFolderFieldLabel: "新しいファクトリーの親フォルダー",
    newFactoryFolderFieldPlaceholder: "/home/bob/new-factory-project",
    newFactoryFolderHelperText:
      "書き込み可能な既存のフォルダーを使ってください。確認後、その factory サブフォルダーに既定のスキャフォールドを作成します。",
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
    openFactoryNoExistingTargetError:
      "이 폴더에서 기존 팩토리를 찾을 수 없습니다. 만들려면 새 팩토리를 선택하세요.",
    openSessionTargetLabel: "선택한 대상 열기",
    openSessionTargetPendingLabel: "대상을 여는 중...",
    openFactorySuccessLabel: "팩토리를 열었습니다.",
    newFactoryButtonLabel: "새 팩토리",
    newFactoryDialogTitle: "새 팩토리",
    newFactoryDialogDescription:
      "새 팩토리의 상위 폴더를 입력하세요. 검증은 정확한 대상만 미리 보여 주며 확인하기 전에는 아무것도 만들지 않습니다.",
    newFactoryExistingTargetError:
      "이 폴더에는 이미 실행 가능한 팩토리가 있습니다. 사용하려면 팩토리 열기를 선택하거나 다른 상위 폴더를 선택하세요.",
    newFactoryFolderFieldLabel: "새 팩토리 상위 폴더",
    newFactoryFolderFieldPlaceholder: "/home/bob/new-factory-project",
    newFactoryFolderHelperText:
      "쓰기 가능한 기존 폴더를 사용하세요. 확인 후 factory 하위 폴더에 기본 스캐폴드가 만들어집니다.",
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
    openFactoryNoExistingTargetError:
      "此文件夹中没有现有工厂。请选择“新建工厂”来创建一个。",
    openSessionTargetLabel: "打开所选目标",
    openSessionTargetPendingLabel: "正在打开目标...",
    openFactorySuccessLabel: "工厂已成功打开。",
    newFactoryButtonLabel: "新建工厂",
    newFactoryDialogTitle: "新建工厂",
    newFactoryDialogDescription:
      "请输入新工厂的父文件夹。验证只会预览确切目标；确认前不会创建任何内容。",
    newFactoryExistingTargetError:
      "此文件夹已包含可运行的工厂。请选择“打开工厂”来使用它，或选择其他父文件夹。",
    newFactoryFolderFieldLabel: "新工厂父文件夹",
    newFactoryFolderFieldPlaceholder: "/home/bob/new-factory-project",
    newFactoryFolderHelperText:
      "请选择现有且可写的文件夹。确认后，将在其中的 factory 子文件夹创建默认脚手架。",
    newFactoryPreviewTitle: "新工厂预览",
    newFactoryPreviewDescriptionTemplate:
      "已验证此文件夹可以创建新工厂。确认后将只创建以下确切目标:",
    newFactoryCancelLabel: "取消",
    newFactoryConfirmLabel: "创建工厂",
    newFactoryConfirmPendingLabel: "正在创建工厂...",
    newFactorySuccessLabel: "新工厂已创建并打开。",
  },
} satisfies LocalizedMessages<HeaderFactoryJourneyMessages>;
