import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactoryEmulatorMessages {
  demos: {
    activity: (workstation: string, detail: string, duration: string) => string;
    activityFallback: (workstation: string) => string;
    activityLabels: {
      draft: string;
      finalReview: string;
      firstReview: string;
      polish: string;
      prepare: string;
      revise: string;
    };
    duration: (seconds: string) => string;
    fixtures: Record<
      "repeat-review-failure" | "success",
      { description: string; title: string }
    >;
    empty: string;
    error: string;
    failureDescription: string;
    failureTitle: string;
    progress: {
      categories: {
        active: {
          plural: (count: string) => string;
          singular: (count: string) => string;
        };
        completed: {
          plural: (count: string) => string;
          singular: (count: string) => string;
        };
        failed: {
          plural: (count: string) => string;
          singular: (count: string) => string;
        };
        queued: {
          plural: (count: string) => string;
          singular: (count: string) => string;
        };
        unclassified: {
          plural: (count: string) => string;
          singular: (count: string) => string;
        };
      };
      empty: string;
      regionLabel: string;
      title: string;
      total: (count: string) => string;
    };
    ready: string;
    regionLabel: string;
    status: {
      completed: string;
      error: string;
      failed: string;
      history: string;
      playing: string;
      ready: string;
    };
    successDescription: string;
    successTitle: string;
    timeline: {
      alreadyFollowingLatest: string;
      currentMode: string;
      disabled: string;
      followLatest: string;
      historyMode: string;
      position: (selected: string, latest: string) => string;
      regionLabel: string;
      sliderLabel: string;
      title: string;
      unavailable: string;
    };
    topology: {
      activeDispatches: (count: number) => string;
      annotationsHidden: string;
      annotationsVisible: string;
      empty: string;
      failed: string;
      inactiveDispatches: string;
      imageFailed: string;
      imageLoading: string;
      loading: string;
      nodeLabel: (kind: string, label: string) => string;
      regionLabel: string;
      resourceOccupancy: (occupied: number, capacity: number) => string;
      resourceOccupancyUnavailable: string;
      retry: string;
      selectedNode: string;
      workStateCount: (count: number) => string;
      workStateCountUnavailable: string;
    };
  };
  submission: {
    blank: string;
    wrongWorkType: string;
    unavailable: {
      "ambiguous-default": string;
      closed: string;
      error: string;
      history: string;
      invalid: string;
      loading: string;
      "no-default": string;
    };
  };
}

const factoryEmulatorMessagesByLocale = {
  en: {
    demos: {
      activity: (workstation, detail, duration) =>
        `${workstation}: ${detail} (${duration})`,
      activityFallback: (workstation) => `Working at ${workstation}`,
      activityLabels: {
        draft: "Drafting the launch plan",
        finalReview: "Running final review",
        firstReview: "Reviewing the first draft",
        polish: "Polishing the revised launch plan",
        prepare: "Preparing the launch summary",
        revise: "Revising the launch plan",
      },
      duration: (seconds) => `${seconds} seconds virtual time`,
      fixtures: {
        "repeat-review-failure": {
          description:
            "Watch Review reject Work back to Execute before final Review fails.",
          title: "Review, rework, and failure",
        },
        success: {
          description:
            "Watch one task move through Execute to successful completion.",
          title: "Straightforward success",
        },
      },
      empty: "No initial Work is available for this demo.",
      error:
        "This demo could not be prepared. The other demo remains available.",
      failureDescription: "The final Review failed and the Work is terminal.",
      failureTitle: "Terminal failure",
      progress: {
        categories: {
          active: {
            plural: (count) => `${count} active`,
            singular: (count) => `${count} active`,
          },
          completed: {
            plural: (count) => `${count} completed`,
            singular: (count) => `${count} completed`,
          },
          failed: {
            plural: (count) => `${count} failed`,
            singular: (count) => `${count} failed`,
          },
          queued: {
            plural: (count) => `${count} queued`,
            singular: (count) => `${count} queued`,
          },
          unclassified: {
            plural: (count) => `${count} unclassified`,
            singular: (count) => `${count} unclassified`,
          },
        },
        empty: "No Work is available.",
        regionLabel: "Demo Work progress",
        title: "Work progress",
        total: (count) => `${count} Work total`,
      },
      ready: "Ready to run with deterministic local Work.",
      regionLabel: "Customer Factory emulator demos",
      status: {
        completed: "Completed successfully",
        error: "Setup error",
        failed: "Terminal failure",
        history: "Viewing history",
        playing: "Playing",
        ready: "Ready",
      },
      successDescription: "The Work reached its successful terminal state.",
      successTitle: "Successful completion",
      timeline: {
        alreadyFollowingLatest: "Already following the current tick.",
        currentMode: "Showing the current Factory.",
        disabled: "Timeline selection is unavailable.",
        followLatest: "Follow current",
        historyMode: "Viewing Factory history.",
        position: (selected, latest) => `Tick ${selected} of ${latest}`,
        regionLabel: "Factory replay timeline",
        sliderLabel: "Select replay tick",
        title: "Replay timeline",
        unavailable: "No replay ticks are available.",
      },
      topology: {
        activeDispatches: (count) => `${count} active Dispatches`,
        annotationsHidden: "Show annotations",
        annotationsVisible: "Hide annotations",
        empty: "Factory topology is unavailable.",
        failed: "Factory topology could not be shown.",
        inactiveDispatches: "No active Dispatches",
        imageFailed: "Annotation image unavailable.",
        imageLoading: "Loading annotation image.",
        loading: "Loading Factory topology.",
        nodeLabel: (kind, label) => `${kind}: ${label}`,
        regionLabel: "Factory topology replay",
        resourceOccupancy: (occupied, capacity) =>
          `${occupied} of ${capacity} resources occupied`,
        resourceOccupancyUnavailable: "Resource occupancy unavailable",
        retry: "Retry",
        selectedNode: "Selected",
        workStateCount: (count) => `${count} Work in this state`,
        workStateCountUnavailable: "Work count unavailable",
      },
    },
    submission: {
      blank: "Enter nonblank text to submit.",
      wrongWorkType: "Submit to the eligible default Work Type.",
      unavailable: {
        "ambiguous-default":
          "Configure exactly one eligible default Work Type.",
        closed: "Restart the completed emulator to submit more Work.",
        error:
          "Retry or restart the invalid emulator session before submitting.",
        history: "Return to the current tick before submitting.",
        invalid:
          "Retry or restart the invalid emulator session before submitting.",
        loading: "Start the emulator before submitting.",
        "no-default": "No eligible default Work Type is available.",
      },
    },
  },
  "zh-CN": {
    demos: {
      activity: (workstation, detail, duration) =>
        `${workstation}：${detail}（${duration}）`,
      activityFallback: (workstation) => `正在 ${workstation} 处理`,
      activityLabels: {
        draft: "正在起草发布计划",
        finalReview: "正在进行最终审核",
        firstReview: "正在审核第一版草稿",
        polish: "正在完善修订后的发布计划",
        prepare: "正在准备发布摘要",
        revise: "正在修订发布计划",
      },
      duration: (seconds) => `${seconds} 秒虚拟时间`,
      fixtures: {
        "repeat-review-failure": {
          description: "查看审核将工作退回执行，然后最终审核失败。",
          title: "审核、返工与失败",
        },
        success: {
          description: "查看一个任务通过执行并成功完成。",
          title: "直接成功",
        },
      },
      empty: "此演示没有可用的初始工作。",
      error: "无法准备此演示。另一个演示仍然可用。",
      failureDescription: "最终审核失败，工作已终止。",
      failureTitle: "终止失败",
      progress: {
        categories: {
          active: {
            plural: (count) => `${count} 个进行中`,
            singular: (count) => `${count} 个进行中`,
          },
          completed: {
            plural: (count) => `${count} 个已完成`,
            singular: (count) => `${count} 个已完成`,
          },
          failed: {
            plural: (count) => `${count} 个失败`,
            singular: (count) => `${count} 个失败`,
          },
          queued: {
            plural: (count) => `${count} 个排队中`,
            singular: (count) => `${count} 个排队中`,
          },
          unclassified: {
            plural: (count) => `${count} 个未分类`,
            singular: (count) => `${count} 个未分类`,
          },
        },
        empty: "没有可用工作。",
        regionLabel: "演示工作进度",
        title: "工作进度",
        total: (count) => `共 ${count} 个工作`,
      },
      ready: "已准备好运行确定性的本地工作。",
      regionLabel: "客户 Factory 模拟器演示",
      status: {
        completed: "成功完成",
        error: "设置错误",
        failed: "终止失败",
        history: "正在查看历史",
        playing: "正在播放",
        ready: "已就绪",
      },
      successDescription: "工作已到达成功终止状态。",
      successTitle: "成功完成",
      timeline: {
        alreadyFollowingLatest: "已在跟随当前逻辑时点。",
        currentMode: "正在显示当前 Factory。",
        disabled: "时间线选择不可用。",
        followLatest: "跟随当前",
        historyMode: "正在查看 Factory 历史。",
        position: (selected, latest) => `逻辑时点 ${selected} / ${latest}`,
        regionLabel: "Factory 回放时间线",
        sliderLabel: "选择回放逻辑时点",
        title: "回放时间线",
        unavailable: "没有可用的回放逻辑时点。",
      },
      topology: {
        activeDispatches: (count) => `${count} 个活动调度`,
        annotationsHidden: "显示注释",
        annotationsVisible: "隐藏注释",
        empty: "Factory 拓扑不可用。",
        failed: "无法显示 Factory 拓扑。",
        inactiveDispatches: "没有活动调度",
        imageFailed: "注释图片不可用。",
        imageLoading: "正在加载注释图片。",
        loading: "正在加载 Factory 拓扑。",
        nodeLabel: (kind, label) => `${kind}：${label}`,
        regionLabel: "Factory 拓扑回放",
        resourceOccupancy: (occupied, capacity) =>
          `${capacity} 个资源中有 ${occupied} 个被占用`,
        resourceOccupancyUnavailable: "资源占用情况不可用",
        retry: "重试",
        selectedNode: "已选择",
        workStateCount: (count) => `此状态下有 ${count} 个工作`,
        workStateCountUnavailable: "工作数量不可用",
      },
    },
    submission: {
      blank: "请输入非空文本后再提交。",
      wrongWorkType: "请提交到符合条件的默认工作类型。",
      unavailable: {
        "ambiguous-default": "请仅配置一个符合条件的默认工作类型。",
        closed: "请重新启动已完成的模拟器以提交更多工作。",
        error: "请先重试或重新启动无效的模拟器会话。",
        history: "请返回当前逻辑时点后再提交。",
        invalid: "请先重试或重新启动无效的模拟器会话。",
        loading: "请先启动模拟器。",
        "no-default": "没有符合条件的默认工作类型。",
      },
    },
  },
} satisfies LocalizedMessageCatalog<FactoryEmulatorMessages>;

export function getFactoryEmulatorMessages(locale?: string | null) {
  return resolveLocalizedMessages(factoryEmulatorMessagesByLocale, locale);
}
