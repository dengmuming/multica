"use client";

import { CheckCircle2, CircleDashed, ShieldCheck, TriangleAlert } from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import type { LoopListCopy, LoopSummaryView } from "../types";

export interface LoopListProps {
  loops: LoopSummaryView[];
  copy: LoopListCopy;
  className?: string;
  selectedLoopId?: string;
  onSelectLoop?: (parentIssueId: string) => void;
  formatDate?: (iso: string) => string;
}

export function LoopList({
  loops,
  copy,
  className,
  selectedLoopId,
  onSelectLoop,
  formatDate = (value) => value,
}: LoopListProps) {
  if (loops.length === 0) {
    return (
      <div className={cn("rounded-xl border bg-card p-8 text-center text-sm text-muted-foreground", className)}>
        {copy.empty}
      </div>
    );
  }

  return (
    <div className={cn("divide-y overflow-hidden rounded-xl border bg-card", className)}>
      {loops.map((loop) => (
        <button
          key={loop.parent_issue_id}
          type="button"
          onClick={() => onSelectLoop?.(loop.parent_issue_id)}
          className={cn(
            "flex w-full items-center gap-4 p-4 text-left transition-colors",
            onSelectLoop && "hover:bg-muted/50",
            selectedLoopId === loop.parent_issue_id && "bg-muted/60",
          )}
        >
          <LoopStateIcon state={loop.state} />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <p className="truncate text-sm font-medium text-foreground">{loop.title}</p>
              <span className="text-xs text-muted-foreground">{loop.instance_key}</span>
            </div>
            <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>{copy.state}: {loop.state}</span>
              <span>{copy.template}: {loop.template_key}@{loop.template_version}</span>
              <span>{copy.updated}: {formatDate(loop.updated_at)}</span>
            </div>
          </div>
        </button>
      ))}
    </div>
  );
}

function LoopStateIcon({ state }: { state: string }) {
  if (state === "completed") return <CheckCircle2 className="h-5 w-5 shrink-0 text-emerald-500" aria-hidden="true" />;
  if (state === "waiting_approval") return <ShieldCheck className="h-5 w-5 shrink-0 text-amber-500" aria-hidden="true" />;
  if (state === "blocked" || state === "failed") return <TriangleAlert className="h-5 w-5 shrink-0 text-destructive" aria-hidden="true" />;
  return <CircleDashed className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />;
}
