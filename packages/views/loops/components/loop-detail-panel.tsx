"use client";

import { CheckCircle2, CircleDashed, GitBranch, ShieldCheck, TriangleAlert } from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import { groupLoopNodesByStage, workflowRetryTotal } from "../loop-utils";
import type { LoopArtifactView, LoopDetailView, LoopNodeView, LoopViewCopy } from "../types";

export interface LoopDetailPanelProps {
  loop: LoopDetailView;
  artifacts?: LoopArtifactView[];
  copy: LoopViewCopy;
  className?: string;
  onOpenIssue?: (issueId: string) => void;
  onOpenArtifact?: (artifact: LoopArtifactView) => void;
}

export function LoopDetailPanel({
  loop,
  artifacts = [],
  copy,
  className,
  onOpenIssue,
  onOpenArtifact,
}: LoopDetailPanelProps) {
  const stages = groupLoopNodesByStage(loop.nodes);
  const retryTotal = workflowRetryTotal(loop);

  return (
    <section className={cn("flex min-w-0 flex-col gap-5", className)}>
      <header className="rounded-xl border bg-card p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>{loop.template_key}@{loop.template_version}</span>
              <span>·</span>
              <span>{loop.instance_key}</span>
            </div>
            <h1 className="mt-2 truncate text-xl font-semibold text-foreground">{loop.title}</h1>
            {loop.description ? (
              <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{loop.description}</p>
            ) : null}
          </div>
          <div className="flex items-center gap-2">
            <StatePill state={loop.state} copy={copy.state} />
            {retryTotal > 0 ? (
              <span className="rounded-full border px-2.5 py-1 text-xs text-muted-foreground">
                {copy.retries}: {retryTotal}
              </span>
            ) : null}
          </div>
        </div>
      </header>

      <div className="overflow-x-auto rounded-xl border bg-card p-4">
        <div className="flex min-w-max items-stretch gap-3">
          {stages.map((stage, index) => (
            <div key={stage.stage} className="flex items-stretch gap-3">
              <div className="w-72 rounded-lg border bg-background p-3">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                    <GitBranch className="h-3.5 w-3.5" />
                    <span>{copy.stage} {stage.stage}</span>
                  </div>
                  <StageState active={stage.active} satisfied={stage.satisfied} />
                </div>
                <div className="flex flex-col gap-2">
                  {stage.nodes.map((node) => (
                    <NodeCard
                      key={node.node_key}
                      node={node}
                      copy={copy}
                      onOpenIssue={onOpenIssue}
                    />
                  ))}
                </div>
              </div>
              {index < stages.length - 1 ? (
                <div className="flex w-5 items-center justify-center text-muted-foreground" aria-hidden="true">
                  →
                </div>
              ) : null}
            </div>
          ))}
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <EvidencePanel loop={loop} copy={copy} />
        <ArtifactPanel artifacts={artifacts} copy={copy} onOpenArtifact={onOpenArtifact} />
      </div>
    </section>
  );
}

function NodeCard({
  node,
  copy,
  onOpenIssue,
}: {
  node: LoopNodeView;
  copy: LoopViewCopy;
  onOpenIssue?: (issueId: string) => void;
}) {
  const interactive = Boolean(onOpenIssue);
  return (
    <button
      type="button"
      disabled={!interactive}
      onClick={() => onOpenIssue?.(node.issue_id)}
      className={cn(
        "w-full rounded-md border p-3 text-left",
        interactive && "cursor-pointer transition-colors hover:bg-muted/50",
        !interactive && "cursor-default",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">{node.title}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">{node.node_key}</p>
        </div>
        <NodeStatus status={node.status} />
      </div>

      <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        {node.role ? <span>{copy.role}: {node.role}</span> : null}
        <span>{node.required ? copy.required : copy.optional}</span>
        {node.retry_count > 0 ? <span>{copy.retries}: {node.retry_count}/{node.max_retries}</span> : null}
      </div>

      {node.assignee_type && node.assignee_id ? (
        <p className="mt-2 truncate text-xs text-muted-foreground">
          {copy.assignee}: {node.assignee_type}:{node.assignee_id}
        </p>
      ) : null}
      {node.latest_task_status ? (
        <p className="mt-1 truncate text-xs text-muted-foreground">
          {copy.task}: {node.latest_task_status}
        </p>
      ) : null}
      {node.evaluation_verdict ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {copy.evaluation}: {node.evaluation_verdict}
          {node.evaluation_target ? ` → ${node.evaluation_target}` : ""}
        </p>
      ) : null}
      {node.approval_state ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {copy.approval}: {node.approval_state}
        </p>
      ) : null}
    </button>
  );
}

function EvidencePanel({ loop, copy }: { loop: LoopDetailView; copy: LoopViewCopy }) {
  const findings = loop.nodes.flatMap((node) =>
    (node.evaluation_findings ?? []).map((finding) => ({ node, finding })),
  );
  const evidence = loop.nodes.flatMap((node) =>
    (node.evaluation_evidence ?? []).map((ref) => ({ node, ref })),
  );

  return (
    <section className="rounded-xl border bg-card p-4">
      <div className="flex items-center gap-2 text-sm font-medium text-foreground">
        <TriangleAlert className="h-4 w-4" />
        <span>{copy.findings}</span>
      </div>
      <div className="mt-3 space-y-2">
        {findings.length === 0 ? (
          <p className="text-sm text-muted-foreground">{copy.noFindings}</p>
        ) : findings.map(({ node, finding }, index) => (
          <div key={`${node.node_key}:${finding.code ?? index}`} className="rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>{node.node_key}</span>
              {finding.severity ? <span>· {finding.severity}</span> : null}
              {finding.owner_node_key ? <span>→ {finding.owner_node_key}</span> : null}
            </div>
            <p className="mt-1 text-sm text-foreground">{finding.summary}</p>
          </div>
        ))}
      </div>

      <div className="mt-5 flex items-center gap-2 text-sm font-medium text-foreground">
        <ShieldCheck className="h-4 w-4" />
        <span>{copy.evidence}</span>
      </div>
      <div className="mt-3 space-y-1">
        {evidence.length === 0 ? (
          <p className="text-sm text-muted-foreground">{copy.noEvidence}</p>
        ) : evidence.map(({ node, ref }) => (
          <p key={`${node.node_key}:${ref}`} className="break-all text-xs text-muted-foreground">
            {node.node_key}: {ref}
          </p>
        ))}
      </div>
    </section>
  );
}

function ArtifactPanel({
  artifacts,
  copy,
  onOpenArtifact,
}: {
  artifacts: LoopArtifactView[];
  copy: LoopViewCopy;
  onOpenArtifact?: (artifact: LoopArtifactView) => void;
}) {
  return (
    <section className="rounded-xl border bg-card p-4">
      <div className="flex items-center gap-2 text-sm font-medium text-foreground">
        <GitBranch className="h-4 w-4" />
        <span>{copy.artifacts}</span>
      </div>
      <div className="mt-3 space-y-2">
        {artifacts.length === 0 ? (
          <p className="text-sm text-muted-foreground">{copy.noArtifacts}</p>
        ) : artifacts.map((artifact) => (
          <button
            type="button"
            key={artifact.id}
            disabled={!onOpenArtifact}
            onClick={() => onOpenArtifact?.(artifact)}
            className={cn(
              "w-full rounded-md border p-3 text-left",
              onOpenArtifact && "transition-colors hover:bg-muted/50",
            )}
          >
            <div className="flex items-center justify-between gap-3">
              <p className="truncate text-sm font-medium text-foreground">{artifact.title}</p>
              <span className="shrink-0 text-xs text-muted-foreground">{artifact.artifact_type}</span>
            </div>
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {artifact.relation} · {artifact.ref_kind} · {artifact.ref_id ?? artifact.ref_uri ?? artifact.id}
            </p>
          </button>
        ))}
      </div>
    </section>
  );
}

function NodeStatus({ status }: { status: string }) {
  if (status === "done") return <CheckCircle2 className="h-4 w-4 text-emerald-500" aria-hidden="true" />;
  if (status === "backlog") return <CircleDashed className="h-4 w-4 text-muted-foreground" aria-hidden="true" />;
  if (status === "cancelled") return <TriangleAlert className="h-4 w-4 text-muted-foreground" aria-hidden="true" />;
  return <CircleDashed className="h-4 w-4 animate-pulse text-foreground" aria-hidden="true" />;
}

function StageState({ active, satisfied }: { active: boolean; satisfied: boolean }) {
  if (satisfied) return <CheckCircle2 className="h-4 w-4 text-emerald-500" aria-hidden="true" />;
  return <CircleDashed className={cn("h-4 w-4", active ? "animate-pulse text-foreground" : "text-muted-foreground")} aria-hidden="true" />;
}

function StatePill({ state, copy }: { state: string; copy: string }) {
  return (
    <span className="rounded-full border bg-background px-2.5 py-1 text-xs text-foreground">
      {copy}: {state}
    </span>
  );
}
