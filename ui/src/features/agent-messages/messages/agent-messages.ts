import type {
  AgentMessage,
  MessageReadFailure,
} from "../../../api/agent-messages/types";
import { resolveLocalizedMessages } from "../../../i18n/messages";

const en = {
  title: "Messages",
  inbox: "Message inbox",
  inboxDescription:
    "Observe worker messages on the selected host. Viewing messages does not mark them read.",
  close: "Close inbox",
  apply: "Apply filters",
  recipientFilter: "Recipient Worker Session",
  senderFilter: "Sender Worker Session",
  threadFilter: "Thread ID",
  statusFilter: "Status",
  allStatuses: "All statuses",
  newData: "New messages are available. Refresh to start a new page walk.",
  live: "Observing message updates.",
  received: "Received",
  sent: "Sent",
  refresh: "Refresh",
  retry: "Retry",
  original: "Back to original Worker Session",
  previous: "Previous",
  next: "Next",
  back: "Back to messages",
  thread: "Open thread",
  loading: "Loading messages…",
  empty: "No messages in this page.",
  unavailable: "Select an available Worker Session to inspect messages.",
  sender: "From",
  recipient: "To",
  reply: "In reply to",
  expires: "Expires",
  age: "Sent",
  hold: "Hold if ended",
  reason: "Reason",
  successor: "Open revived Worker Session",
  successorError:
    "The revived Worker Session is unavailable. Your selection has been preserved.",
  stale: "Refresh to check for newer message status.",
  status: {
    QUEUED: "Queued",
    READ: "Read",
    DELIVERED_REVIVE: "Delivered by revival",
    REPLIED: "Replied",
    EXPIRED: "Expired",
    DELIVERED_INTERRUPT: "Delivered by interruption",
    REJECTED: "Rejected",
  } satisfies Record<AgentMessage["status"], string>,
  reasons: {
    REVIVE_PENDING: "Revival pending",
    REVIVE_UNSUPPORTED: "Revival unsupported",
    REVIVE_FAILED: "Revival failed",
    REVIVE_LIMIT: "Revival limit reached",
  } as Record<string, string>,
  failures: {
    gap: "Message updates were interrupted. Retry to reload retained messages.",
    disabled: "Messaging is disabled on this host.",
    denied: "Messages are not available to this observer.",
    unavailable: "The message service or exact Worker Session is unavailable.",
    invalid: "The host returned an invalid message response.",
    "expired-page": "This page has expired. Refresh to start again.",
    error: "Messages could not be loaded. Retry the request.",
  } satisfies Record<MessageReadFailure, string>,
};
const es: typeof en = {
  title: "Mensajes",
  inbox: "Bandeja de mensajes",
  inboxDescription:
    "Observe mensajes de Workers en el host seleccionado. Ver mensajes no los marca como leídos.",
  close: "Cerrar bandeja",
  apply: "Aplicar filtros",
  recipientFilter: "Sesión de Worker destinataria",
  senderFilter: "Sesión de Worker remitente",
  threadFilter: "ID del hilo",
  statusFilter: "Estado",
  allStatuses: "Todos los estados",
  newData: "Hay mensajes nuevos. Actualice para iniciar una nueva paginación.",
  live: "Observando actualizaciones de mensajes.",
  received: "Recibidos",
  sent: "Enviados",
  refresh: "Actualizar",
  retry: "Reintentar",
  original: "Volver a la sesión de Worker original",
  previous: "Anterior",
  next: "Siguiente",
  back: "Volver a mensajes",
  thread: "Abrir hilo",
  loading: "Cargando mensajes…",
  empty: "No hay mensajes en esta página.",
  unavailable: "Seleccione una sesión de Worker disponible para ver mensajes.",
  sender: "De",
  recipient: "Para",
  reply: "En respuesta a",
  expires: "Caduca",
  age: "Enviado",
  hold: "Retener si terminó",
  reason: "Motivo",
  successor: "Abrir sesión de Worker reactivada",
  successorError:
    "La sesión de Worker reactivada no está disponible. Se conserva su selección.",
  stale: "Actualice para comprobar el estado de los mensajes.",
  status: {
    QUEUED: "En cola",
    READ: "Leído",
    DELIVERED_REVIVE: "Entregado por reactivación",
    REPLIED: "Respondido",
    EXPIRED: "Caducado",
    DELIVERED_INTERRUPT: "Entregado por interrupción",
    REJECTED: "Rechazado",
  },
  reasons: {
    REVIVE_PENDING: "Reactivación pendiente",
    REVIVE_UNSUPPORTED: "Reactivación no compatible",
    REVIVE_FAILED: "Reactivación fallida",
    REVIVE_LIMIT: "Límite de reactivación alcanzado",
  },
  failures: {
    gap: "Se interrumpieron las actualizaciones. Reintente para cargar los mensajes retenidos.",
    disabled: "Los mensajes están desactivados en este host.",
    denied: "Los mensajes no están disponibles para este observador.",
    unavailable:
      "El servicio de mensajes o la sesión exacta de Worker no están disponibles.",
    invalid: "El host devolvió una respuesta de mensajes no válida.",
    "expired-page": "Esta página ha caducado. Actualice para empezar de nuevo.",
    error: "No se pudieron cargar los mensajes. Reintente la solicitud.",
  },
};

const zh: typeof en = {
  title: "消息",
  inbox: "消息收件箱",
  inboxDescription:
    "查看所选主机上的 Worker 消息。查看消息不会将其标记为已读。",
  close: "关闭收件箱",
  apply: "应用筛选",
  recipientFilter: "接收方 Worker 会话",
  senderFilter: "发送方 Worker 会话",
  threadFilter: "消息串 ID",
  statusFilter: "状态",
  allStatuses: "所有状态",
  newData: "有新消息。刷新以重新分页。",
  live: "正在观察消息更新。",
  received: "收到",
  sent: "已发送",
  refresh: "刷新",
  retry: "重试",
  original: "返回原 Worker 会话",
  previous: "上一页",
  next: "下一页",
  back: "返回消息",
  thread: "打开消息串",
  loading: "正在加载消息…",
  empty: "此页没有消息。",
  unavailable: "请选择可用的 Worker 会话以查看消息。",
  sender: "发送方",
  recipient: "接收方",
  reply: "回复消息",
  expires: "到期",
  age: "发送时间",
  hold: "结束后保留",
  reason: "原因",
  successor: "打开已恢复的 Worker 会话",
  successorError: "已恢复的 Worker 会话不可用。已保留原选择。",
  stale: "刷新以查看最新消息状态。",
  status: {
    QUEUED: "排队中",
    READ: "已读",
    DELIVERED_REVIVE: "恢复后已送达",
    REPLIED: "已回复",
    EXPIRED: "已过期",
    DELIVERED_INTERRUPT: "中断后已送达",
    REJECTED: "已拒绝",
  },
  reasons: {
    REVIVE_PENDING: "等待恢复",
    REVIVE_UNSUPPORTED: "不支持恢复",
    REVIVE_FAILED: "恢复失败",
    REVIVE_LIMIT: "已达到恢复限制",
  },
  failures: {
    gap: "消息更新已中断。请重试以重新加载保留的消息。",
    disabled: "此主机已禁用消息功能。",
    denied: "此观察者无法查看消息。",
    unavailable: "消息服务或指定 Worker 会话不可用。",
    invalid: "主机返回的消息响应无效。",
    "expired-page": "此页已过期。请刷新以重新开始。",
    error: "无法加载消息。请重试请求。",
  },
};

export function getAgentMessageMessages(locale?: string) {
  if (locale?.toLowerCase().startsWith("es")) return es;
  return resolveLocalizedMessages({ en, "zh-CN": zh }, locale);
}

export function messageRelativeTime(
  timestamp: string,
  now: number,
  locale?: string,
): string {
  const seconds = Math.round((Date.parse(timestamp) - now) / 1000);
  const formatter = new Intl.RelativeTimeFormat(locale ?? "en", {
    numeric: "auto",
  });
  if (Math.abs(seconds) < 60) return formatter.format(seconds, "second");
  if (Math.abs(seconds) < 3600)
    return formatter.format(Math.round(seconds / 60), "minute");
  if (Math.abs(seconds) < 86400)
    return formatter.format(Math.round(seconds / 3600), "hour");
  return formatter.format(Math.round(seconds / 86400), "day");
}
