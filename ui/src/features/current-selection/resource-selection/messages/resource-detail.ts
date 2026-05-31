import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import { getResourceDetailEnumMessages } from "./resource-detail-enums";
import type { ResourceDetailMessages } from "./resource-detail-types";

type ResourceDetailCatalogMessages = Omit<
  ResourceDetailMessages,
  "localizeResourceType"
>;

const resourceDetailMessagesByLocale = {
  en: {
    backendFieldLabel: "Backend",
    capacityFieldLabel: "Capacity",
    configurationEmpty:
      "This running factory definition does not include the selected resource.",
    configurationErrorPrefix: "Resource definition unavailable.",
    configurationLoading:
      "Loading the current factory definition for this resource.",
    loadPolicyFieldLabel: "Load policy",
    modelFieldLabel: "Model",
    nameFieldLabel: "Name",
    notConfiguredValue: "Not configured",
    providerFieldLabel: "Provider",
    referencingWorkersEmpty:
      "No workers in the running factory definition require this resource.",
    referencingWorkersHeading: "Referencing workers",
    referencingWorkstationsEmpty:
      "No workstations in the running factory definition consume this resource.",
    referencingWorkstationsHeading: "Referencing workstations",
    summaryHeading: "Summary",
    tokenCountFieldLabel: "Available tokens",
    typeFieldLabel: "Type",
    unknownTypeValue: "Unknown type",
  },
  ja: {
    backendFieldLabel: "バックエンド",
    capacityFieldLabel: "容量",
    configurationEmpty:
      "実行中のファクトリ定義に選択したリソースが含まれていません。",
    configurationErrorPrefix: "リソース定義を取得できません。",
    configurationLoading:
      "このリソースの現在のファクトリ定義を読み込んでいます。",
    loadPolicyFieldLabel: "ロードポリシー",
    modelFieldLabel: "モデル",
    nameFieldLabel: "名前",
    notConfiguredValue: "未設定",
    providerFieldLabel: "プロバイダー",
    referencingWorkersEmpty:
      "実行中のファクトリ定義でこのリソースを必要とするワーカーはありません。",
    referencingWorkersHeading: "参照ワーカー",
    referencingWorkstationsEmpty:
      "実行中のファクトリ定義でこのリソースを消費するワークステーションはありません。",
    referencingWorkstationsHeading: "参照ワークステーション",
    summaryHeading: "概要",
    tokenCountFieldLabel: "利用可能トークン数",
    typeFieldLabel: "種別",
    unknownTypeValue: "不明な種別",
  },
  ko: {
    backendFieldLabel: "백엔드",
    capacityFieldLabel: "용량",
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 리소스가 포함되어 있지 않습니다.",
    configurationErrorPrefix: "리소스 정의를 사용할 수 없습니다.",
    configurationLoading: "이 리소스의 현재 팩토리 정의를 불러오는 중입니다.",
    loadPolicyFieldLabel: "로드 정책",
    modelFieldLabel: "모델",
    nameFieldLabel: "이름",
    notConfiguredValue: "구성되지 않음",
    providerFieldLabel: "프로바이더",
    referencingWorkersEmpty:
      "실행 중인 팩토리 정의에서 이 리소스를 필요로 하는 워커가 없습니다.",
    referencingWorkersHeading: "참조 워커",
    referencingWorkstationsEmpty:
      "실행 중인 팩토리 정의에서 이 리소스를 소비하는 워크스테이션이 없습니다.",
    referencingWorkstationsHeading: "참조 워크스테이션",
    summaryHeading: "요약",
    tokenCountFieldLabel: "사용 가능 토큰",
    typeFieldLabel: "유형",
    unknownTypeValue: "알 수 없는 유형",
  },
  "zh-CN": {
    backendFieldLabel: "后端",
    capacityFieldLabel: "容量",
    configurationEmpty: "当前运行中的工厂定义不包含所选资源。",
    configurationErrorPrefix: "资源定义不可用。",
    configurationLoading: "正在加载此资源的当前工厂定义。",
    loadPolicyFieldLabel: "加载策略",
    modelFieldLabel: "模型",
    nameFieldLabel: "名称",
    notConfiguredValue: "未配置",
    providerFieldLabel: "提供商",
    referencingWorkersEmpty: "当前运行中的工厂定义没有工人需要此资源。",
    referencingWorkersHeading: "引用此资源的工人",
    referencingWorkstationsEmpty: "当前运行中的工厂定义没有工位消耗此资源。",
    referencingWorkstationsHeading: "引用此资源的工位",
    summaryHeading: "摘要",
    tokenCountFieldLabel: "可用令牌数",
    typeFieldLabel: "类型",
    unknownTypeValue: "未知类型",
  },
} satisfies LocalizedMessages<ResourceDetailCatalogMessages>;

export function getResourceDetailMessages(
  locale?: string | null,
): ResourceDetailMessages {
  const enumMessages = getResourceDetailEnumMessages(locale);

  return {
    ...resolveLocalizedMessages(resourceDetailMessagesByLocale, locale),
    localizeResourceType: enumMessages.localizeResourceType,
  };
}
