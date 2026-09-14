"use client";

import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  Clock3,
  GitBranch,
  Loader2,
  RotateCcw,
  ShieldCheck,
} from "lucide-react";
import type { ManifoldLoopDetail, ManifoldLoopNode } from "@multica/core/types/manifold-agent";
import { cn } from "@multica/ui/lib/utils";
import { currentLoopStage, loopProgress, projectLoopStages, type ManifoldStageStatus } from "../model/loop-stage-model";

export interface LoopDetailPanelLabels {
  currentStage: string;
  progress: string;
  retries: string;
  evaluation: string;
  recoveryTarget: string;
  approval: string;
  assignedTo: string;
  latestTask: string;
  optional: string;
  states: Record<ManifoldStageStatus, string>;
}

export interface LoopDetailPanelProps {
  loop: ManifoldLoopDetail;
  labels: LoopDetailPanelLabels;
  className?: string;
  onOpenIssue?: (issueId: string) => void;
  onOpenTask?: (taskId: string) => void;
}

const STATUS_VISUAL: Record<ManifoldStageStatus, { icon: typeof Circle; className: string }> = {
  backlog: { icon: Circle, className: "text-muted-foreground" },
  ready: { icon: Clock3, className: "text-blue-500" },
  running: { icon: Loader2, className: "text-blue-500" },
  waiting_approval: { icon: ShieldCheck, className: "text-amber-500" },
  failed: { icon: AlertTriangle, className: "text-destructive" },
  done: { icon: CheckCircle2, className: "text-emerald-500" },
};

function NodeCard({
  node,
  labels,
  onOpenIssue,
  onOpenTask,
}: {
  node: ManifoldLoopNode;
  labels: LoopDetailPanelLabels;
  onOpenIssue?: (issueId: string) => void;
  onOpenTask?: (taskId: string) => void;
}) {
  const clickableIssue = Boolean(onOpenIssue && node.issue_id);
  const issueBody = (
    <>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate text-sm font-medium">{node.title}</span>
            {!node.required ? (
              <span className="rounded-full border px-1.5 py-0.5 text-[10px] text-muted-foreground">
                {labels.optional}
              </span>
            ) : null}
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>{node.node_key}</span>
            {node.role ? <span>{node.role}</span> : null}
            <span>{node.status}</span>
          </div>
        </div>
        {node.retry_count > 0 ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
            <RotateCcw className="h-3.5 w-3.5" />
            {node.retry_count}/{node.max_retries}
          </span>
        ) : null}
      </div>

      {node.assignee_id ? (
        <div className="mt-3 text-xs text-muted-foreground">
          {labels.assignedTo}: {node.assignee_type ? `${node.assignee_type}:` : ""}{node.assignee_id}
        </div>
      ) : null}

      {node.latest_task_id ? (
        <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>{labels.latestTask}:</span>
          {onOpenTask ? (
            <button
              type="button"
              className="font-mono underline-offset-2 hover:underline"
              onClick={(event) => {
                event.stopPropagation();
                onOpenTask(node.latest_task_id!);
              }}
            >
              {node.latest_task_id}
            </button>
          ) : (
            <span className="font-mono">{node.latest_task_id}</span>
          )}
          {node.latest_task_status ? <span>{node.latest_task_status}</span> : null}
        </div>
      ) : null}

      {node.evaluation_verdict ? (
        <div className="mt-3 rounded-md border bg-muted/30 p-2.5 text-xs">
          <div className="font-medium">
            {labels.evaluation}: {node.evaluation_verdict}
          </div>
          {node.evaluation_target ? (
            <div className="mt-1 text-muted-foreground">
              {labels.recoveryTarget}: {node.evaluation_target}
            </div>
          ) : null}
          {node.evaluation_findings?.length ? (
            <ul className="mt-2 space-y-1 text-muted-foreground">
              {node.evaluation_findings.map((finding, index) => (
                <li key={`${finding.code}-${index}`} className="flex gap-2">
                  <span className="shrink-0 font-mono">{finding.severity}</span>
                  <span>{finding.summary}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}

      {node.approval_state ? (
        <div className="mt-3 rounded-md border bg-muted/30 p-2.5 text-xs">
          <div className="font-medium">
            {labels.approval}: {node.approval_state}
          </div>
          {node.approval_rationale ? (
            <div className="mt-1 text-muted-foreground">{node.approval_rationale}</div>
          ) : null}
        </div>
      ) : null}
    </>
  );

  if (!clickableIssue) {
    return <div className="rounded-lg border bg-card p-3">{issueBody}</div>;
  }

  return (
    <button
      type="button"
      className="w-full rounded-lg border bg-card p-3 text-left transition-colors hover:bg-muted/40"
      onClick={() => onOpenIssue?.(node.issue_id)}
    >
      {issueBody}
    </button>
  );
}

export function LoopDetailPanel({
  loop,
  labels,
  className,
  onOpenIssue,
  onOpenTask,
}: LoopDetailPanelProps) {
  const stages = projectLoopStages(loop);
  const current = currentLoopStage(loop);
  const progress = loopProgress(loop);

  return (
    <div className={cn("space-y-4", className)}>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-lg border bg-card p-3">
          <div className="text-xs text-muted-foreground">{labels.currentStage}</div>
          <div className="mt-1 text-lg font-semibold">{current?.stage ?? "—"}</div>
        </div>
        <div className="rounded-lg border bg-card p-3">
          <div className="text-xs text-muted-foreground">{labels.progress}</div>
          <div className="mt-1 text-lg font-semibold">
            {progress.completed}/{progress.total}
          </div>
        </div>
        <div className="rounded-lg border bg-card p-3">
          <div className="text-xs text-muted-foreground">{labels.retries}</div>
          <div className="mt-1 text-lg font-semibold">
            {loop.nodes.reduce((total, node) => total + node.retry_count, 0)}
          </div>
        </div>
      </div>

      <div className="space-y-3">
        {stages.map((stage) => {
          const visual = STATUS_VISUAL[stage.status];
          const Icon = visual.icon;
          return (
            <section key={stage.stage} className="rounded-xl border bg-background p-4">
              <div className="mb-3 flex items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <div className={cn("rounded-md border p-1.5", visual.className)}>
                    <Icon className={cn("h-4 w-4", stage.status === "running" && "animate-spin")} />
                  </div>
                  <div>
                    <div className="flex items-center gap-2 text-sm font-semibold">
                      <GitBranch className="h-3.5 w-3.5 text-muted-foreground" />
                      <span>{stage.stage}</span>
                    </div>
                    <div className="text-xs text-muted-foreground">{labels.states[stage.status]}</div>
                  </div>
                </div>
                {stage.retryCount > 0 ? (
                  <div className="text-xs text-muted-foreground">
                    {labels.retries}: {stage.retryCount}/{stage.maxRetries}
                  </div>
                ) : null}
              </div>

              <div className={cn("grid gap-3", stage.nodes.length > 1 && "lg:grid-cols-2")}>
                {stage.nodes.map((node) => (
                  <NodeCard
                    key={node.node_key}
                    node={node}
                    labels={labels}
                    onOpenIssue={onOpenIssue}
                    onOpenTask={onOpenTask}
                  />
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </div>
  );
}
