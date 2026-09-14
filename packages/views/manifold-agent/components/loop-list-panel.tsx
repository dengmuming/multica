"use client";

import { AlertTriangle, CheckCircle2, Circle, Clock3, GitBranch, ShieldCheck } from "lucide-react";
import type { ManifoldLoopSummary } from "@multica/core/types/manifold-agent";
import { cn } from "@multica/ui/lib/utils";

export interface LoopListPanelLabels {
  empty: string;
  template: string;
  updated: string;
  states: Record<string, string>;
}

export interface LoopListPanelProps {
  loops: ManifoldLoopSummary[];
  labels: LoopListPanelLabels;
  className?: string;
  onOpenLoop?: (parentIssueId: string) => void;
  formatTimestamp?: (value: string) => string;
}

function loopStateIcon(state: string) {
  switch (state) {
    case "completed":
      return { icon: CheckCircle2, className: "text-emerald-500" };
    case "waiting_approval":
      return { icon: ShieldCheck, className: "text-amber-500" };
    case "blocked":
    case "failed":
      return { icon: AlertTriangle, className: "text-destructive" };
    case "running":
      return { icon: Clock3, className: "text-blue-500" };
    default:
      return { icon: Circle, className: "text-muted-foreground" };
  }
}

export function LoopListPanel({
  loops,
  labels,
  className,
  onOpenLoop,
  formatTimestamp = (value) => value,
}: LoopListPanelProps) {
  if (loops.length === 0) {
    return (
      <div className={cn("rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground", className)}>
        {labels.empty}
      </div>
    );
  }

  return (
    <div className={cn("divide-y rounded-xl border bg-card", className)}>
      {loops.map((loop) => {
        const visual = loopStateIcon(loop.state);
        const Icon = visual.icon;
        const body = (
          <>
            <div className="flex min-w-0 items-start gap-3">
              <div className={cn("mt-0.5 rounded-md border p-1.5", visual.className)}>
                <Icon className="h-4 w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate text-sm font-semibold">{loop.title}</span>
                  <span className="rounded-full border px-2 py-0.5 text-[10px] text-muted-foreground">
                    {labels.states[loop.state] ?? loop.state}
                  </span>
                </div>
                {loop.description ? (
                  <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{loop.description}</p>
                ) : null}
                <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                  <span className="inline-flex items-center gap-1">
                    <GitBranch className="h-3.5 w-3.5" />
                    {labels.template}: {loop.template_key}@{loop.template_version}
                  </span>
                  <span>{labels.updated}: {formatTimestamp(loop.updated_at)}</span>
                </div>
              </div>
            </div>
          </>
        );

        if (!onOpenLoop) {
          return <div key={loop.parent_issue_id} className="p-4">{body}</div>;
        }
        return (
          <button
            key={loop.parent_issue_id}
            type="button"
            className="block w-full p-4 text-left transition-colors hover:bg-muted/40"
            onClick={() => onOpenLoop(loop.parent_issue_id)}
          >
            {body}
          </button>
        );
      })}
    </div>
  );
}
