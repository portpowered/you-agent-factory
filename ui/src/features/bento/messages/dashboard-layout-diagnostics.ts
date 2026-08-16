import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { DashboardLayoutDiagnosticCode } from "../hooks/dashboardLayoutSchema";

export interface DashboardLayoutDiagnosticsMessages {
  diagnosticLabel: (
    code: DashboardLayoutDiagnosticCode,
    count: number,
  ) => string;
  emptyLabel: string;
  repairDetails: string;
  regionLabel: string;
  repairSummary: string;
  storageLabel: string;
  storageSummary: string;
  successLabel: string;
}

const diagnosticLabels: Record<
  DashboardLayoutDiagnosticCode,
  { en: string; zhCN: string }
> = {
  collision: {
    en: "Overlapping cards were repositioned.",
    zhCN: "重叠的小组件已重新定位。",
  },
  "duplicate-id": {
    en: "Duplicate card identities were repaired.",
    zhCN: "重复的小组件身份已修复。",
  },
  "invalid-id": {
    en: "Unsafe card identities were replaced.",
    zhCN: "不安全的小组件身份已替换。",
  },
  "invalid-item": {
    en: "Unsupported saved cards were removed.",
    zhCN: "不支持的已保存小组件已移除。",
  },
  "invalid-size": {
    en: "Invalid card sizes were repaired.",
    zhCN: "无效的小组件尺寸已修复。",
  },
  "malformed-json": {
    en: "The saved layout could not be read, so the safe default layout is in use.",
    zhCN: "无法读取已保存的布局，因此正在使用安全的默认布局。",
  },
  "out-of-bounds": {
    en: "Cards outside the dashboard bounds were repositioned.",
    zhCN: "超出看板边界的小组件已重新定位。",
  },
  "singleton-violation": {
    en: "Duplicate singleton cards were removed.",
    zhCN: "重复的单例小组件已移除。",
  },
  "storage-quota-exceeded": {
    en: "The browser storage limit was reached; this layout remains available for this session.",
    zhCN: "浏览器存储空间已满；当前布局仍可在本次会话中使用。",
  },
  "storage-read-failed": {
    en: "The browser could not read the saved layout; this layout remains available in memory.",
    zhCN: "浏览器无法读取已保存的布局；当前布局仍保留在内存中。",
  },
  "storage-unavailable": {
    en: "Browser storage is unavailable; this layout remains available in memory.",
    zhCN: "浏览器存储不可用；当前布局仍保留在内存中。",
  },
  "storage-write-failed": {
    en: "The browser could not save the layout; this layout remains available in memory.",
    zhCN: "浏览器无法保存布局；当前布局仍保留在内存中。",
  },
  "unsupported-envelope": {
    en: "The saved layout format is unsupported, so the safe default layout is in use.",
    zhCN: "已保存的布局格式不受支持，因此正在使用安全的默认布局。",
  },
};

const dashboardLayoutDiagnosticsMessagesByLocale = {
  en: {
    diagnosticLabel: (code, count) => {
      const label = diagnosticLabels[code].en;
      return isRepairDiagnosticCode(code) && count > 1
        ? `${label} (${count} items)`
        : label;
    },
    emptyLabel: "No layout issues detected.",
    repairDetails:
      "Saved layout data was repaired; dashboard content remains available.",
    regionLabel: "Dashboard layout status",
    repairSummary: "The dashboard repaired saved layout data.",
    storageLabel: "Layout storage issue",
    storageSummary: "The dashboard is using a safe in-memory layout.",
    successLabel: "Layout ready",
  },
  "zh-CN": {
    diagnosticLabel: (code, count) => {
      const label = diagnosticLabels[code].zhCN;
      return isRepairDiagnosticCode(code) && count > 1
        ? `${label}（${count} 项）`
        : label;
    },
    emptyLabel: "未检测到布局问题。",
    repairDetails: "已修复已保存的布局数据；看板内容仍可用。",
    regionLabel: "看板布局状态",
    repairSummary: "看板已修复已保存的布局数据。",
    storageLabel: "布局存储问题",
    storageSummary: "看板正在使用安全的内存布局。",
    successLabel: "布局已就绪",
  },
} satisfies LocalizedMessageCatalog<DashboardLayoutDiagnosticsMessages>;

export function getDashboardLayoutDiagnosticsMessages(
  locale?: string | null,
): DashboardLayoutDiagnosticsMessages {
  return resolveLocalizedMessages(
    dashboardLayoutDiagnosticsMessagesByLocale,
    locale,
  );
}

export { dashboardLayoutDiagnosticsMessagesByLocale };

function isRepairDiagnosticCode(code: DashboardLayoutDiagnosticCode): boolean {
  return (
    code !== "malformed-json" &&
    code !== "storage-quota-exceeded" &&
    code !== "storage-read-failed" &&
    code !== "storage-unavailable" &&
    code !== "storage-write-failed" &&
    code !== "unsupported-envelope"
  );
}
