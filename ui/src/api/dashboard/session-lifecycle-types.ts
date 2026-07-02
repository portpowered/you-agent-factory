export interface DashboardSessionBracket {
  artifact_ids?: string[];
  completed_at?: string;
  dispatch_counts?: {
    completed: number;
    queued: number;
    running: number;
  };
  duration_millis?: number;
  factory_id?: string;
  failure_message?: string;
  failure_reason?: string;
  final_status?: string;
  lifecycle_control_status?: string;
  orchestrator_dialect?: string;
  orchestrator_kind?: string;
  paused_at?: string;
  resumed_at?: string;
  result_status?: string;
  result_summary?: Array<{ text?: string; type?: string }>;
  session_id?: string;
  source_ref?: string;
  started_at?: string;
  terminal?: boolean;
}
