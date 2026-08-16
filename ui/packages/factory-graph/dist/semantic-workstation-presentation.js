import { WorkstationKind } from "@you-agent-factory/client";
import { factoryGraphNodeTitleClassName, factoryGraphNodeWrappedTextClassName, } from "./semantic-node-style.js";
import { factoryGraphWorkstationRuntimeRole, UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS, } from "./workstation-semantics.js";
/** Render-independent workstation presentation metadata for a graph node. */
export function factoryGraphWorkstationPresentation(semantics = UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS, locale) {
    const runtime = runtimePresentation(semantics.runtimeType, locale);
    return {
        ...semantics,
        ...schedulingPresentation(semantics.schedulingBehavior, locale),
        className: runtime.className,
        iconKind: runtime.iconKind,
        label: runtime.label,
    };
}
function runtimePresentation(runtimeType, locale) {
    const chinese = locale === "zh-CN";
    const runtimeRole = factoryGraphWorkstationRuntimeRole(runtimeType);
    const labels = {
        AGENT: chinese ? "代理" : "Agent",
        CLASSIFIER: chinese ? "分类器" : "Classifier",
        HUMAN_APPROVAL: chinese ? "人工审批" : "Human approval",
        INFERENCE: chinese ? "推理" : "Inference",
        LOGICAL_MOVE: chinese ? "逻辑移动" : "Logical move",
        POLLER: chinese ? "轮询" : "Poller",
        SCRIPT: chinese ? "脚本" : "Script",
        UNKNOWN: chinese ? "未知" : "Unknown",
    };
    const iconKinds = {
        AGENT: "workstation",
        CLASSIFIER: "queue",
        HUMAN_APPROVAL: "constraint",
        INFERENCE: "processing",
        LOGICAL_MOVE: "workstation",
        POLLER: "poller",
        SCRIPT: "workstation",
        UNKNOWN: "workstation",
    };
    return {
        className: "text-on-surface-subtle",
        iconKind: iconKinds[runtimeRole],
        label: labels[runtimeRole],
    };
}
function schedulingPresentation(behavior, locale) {
    const chinese = locale === "zh-CN";
    switch (behavior) {
        case WorkstationKind.CRON:
            return {
                borderClassName: "border-dashed",
                schedulingLabel: "Cron",
            };
        case WorkstationKind.POLLER:
            return {
                borderClassName: "border-dotted",
                schedulingLabel: chinese ? "轮询" : "Poller",
            };
        case WorkstationKind.REPEATER:
            return {
                borderClassName: "border-double",
                schedulingLabel: chinese ? "重复器" : "Repeater",
            };
        case WorkstationKind.STANDARD:
            return { schedulingLabel: undefined };
        case "UNKNOWN":
            return {
                schedulingLabel: chinese
                    ? "默认调度器"
                    : "Default scheduler",
            };
    }
}
export function factoryGraphWorkstationControlRoleLabel(controlRole, locale) {
    if (locale === "zh-CN") {
        switch (controlRole) {
            case "CLASSIFIER":
                return "分类器路由";
            case "HUMAN_APPROVAL":
                return "人工审批";
            case "LOGICAL_ROUTER":
                return "逻辑路由";
            case "LOOP_BREAKER":
                return "循环断路器";
            case "NONE":
                return "无控制角色";
            case "UNKNOWN":
                return "控制角色未知";
        }
    }
    switch (controlRole) {
        case "CLASSIFIER":
            return "Classifier route";
        case "HUMAN_APPROVAL":
            return "Human approval";
        case "LOGICAL_ROUTER":
            return "Logical router";
        case "LOOP_BREAKER":
            return "Loop breaker";
        case "NONE":
            return "No control role";
        case "UNKNOWN":
            return "Unknown control role";
    }
}
export function factoryGraphWorkstationGuardLimitValue(control) {
    const fixed = control.limit.fixed === undefined ? undefined : String(control.limit.fixed);
    const argument = control.limit.argument;
    if (fixed && argument)
        return `${fixed} (${argument})`;
    return fixed ?? argument ?? "Unknown";
}
export function factoryGraphWorkstationGuardTargetLabel(locale) {
    return locale === "zh-CN" ? "目标工作站" : "Target workstation";
}
export function factoryGraphWorkstationGuardLimitLabel(locale) {
    return locale === "zh-CN" ? "访问上限" : "Visit limit";
}
export function factoryGraphWorkItemLabel(item) {
    return item.display_name?.trim() || item.work_id?.trim() || "Unknown work";
}
export function factoryGraphGraphDuration(startedAt, now, locale) {
    const millis = durationMillis(startedAt, now);
    if (millis === undefined)
        return "Unavailable";
    const [value, suffix] = graphDurationParts(millis, locale);
    return `${value}${suffix}`;
}
export function factoryGraphDurationText(startedAt, now, locale) {
    const millis = durationMillis(startedAt, now);
    if (millis === undefined)
        return "Unavailable";
    const seconds = Math.floor(millis / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    const [hour, minute, second] = locale === "zh-CN" ? ["小时", "分", "秒"] : ["h", "m", "s"];
    if (hours > 0)
        return `${hours}${hour} ${minutes % 60}${minute}`;
    if (minutes > 0)
        return `${minutes}${minute} ${seconds % 60}${second}`;
    return `${seconds}${second}`;
}
export function factoryGraphActiveItemsLabel(count, locale) {
    return locale === "zh-CN"
        ? `${count} 个活动项`
        : `${count} active ${count === 1 ? "item" : "items"}`;
}
export function factoryGraphSelectWorkstationLabel(title, locale) {
    return locale === "zh-CN"
        ? `选择 ${title} 工作站`
        : `Select ${title} workstation`;
}
export function factoryGraphWorkstationTitleClassName(label) {
    return factoryGraphNodeTitleClassName(factoryGraphClassNames("basis-0 flex-1", label.length > 34
        ? "text-[0.78rem]"
        : label.length > 20
            ? "text-[0.88rem]"
            : "text-[1rem]"));
}
export function factoryGraphWorkItemLabelClassName(label) {
    return factoryGraphClassNames(factoryGraphNodeWrappedTextClassName("block basis-0 flex-1 leading-tight"), label.length > 58
        ? "text-[0.64rem]"
        : label.length > 38
            ? "text-[0.68rem]"
            : "text-[0.74rem]");
}
export function factoryGraphClassNames(...values) {
    return values.filter(Boolean).join(" ");
}
function durationMillis(startedAt, now) {
    const started = Date.parse(startedAt);
    return Number.isNaN(started) ? undefined : Math.max(0, now - started);
}
function graphDurationParts(millis, locale) {
    const units = [
        [24 * 60 * 60 * 1000, locale === "zh-CN" ? "天" : "d"],
        [60 * 60 * 1000, locale === "zh-CN" ? "时" : "h"],
        [60 * 1000, locale === "zh-CN" ? "分" : "m"],
        [1000, locale === "zh-CN" ? "秒" : "s"],
    ];
    const unit = units.find(([size]) => millis >= size) ?? units[3];
    return [
        new Intl.NumberFormat(locale).format(Math.floor(millis / unit[0])),
        unit[1],
    ];
}
