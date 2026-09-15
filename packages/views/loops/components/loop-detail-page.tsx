"use client";

import { useQuery } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { loopArtifactsOptions, loopDetailOptions, loopWorkGraphOptions } from "@multica/core/loops/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { useNavigation } from "../../navigation";
import { LoopDetailPanel } from "./loop-detail-panel";
import { WorkGraph } from "./work-graph";

const DETAIL_COPY = { stage: "Stage", state: "State", role: "Role", assignee: "Assignee", task: "Task", retries: "Workflow retries", evaluation: "Evaluation", findings: "Findings", approval: "Approval", evidence: "Evidence", artifacts: "Artifacts", noFindings: "No findings", noEvidence: "No evidence", noArtifacts: "No artifacts", required: "Required", optional: "Optional" };

export function LoopDetailPage({ parentIssueId }: { parentIssueId: string }) {
  const workspaceId = useWorkspaceId(); const workspacePaths = useWorkspacePaths(); const navigation = useNavigation();
  const loopQuery = useQuery(loopDetailOptions(workspaceId, parentIssueId));
  const graphQuery = useQuery(loopWorkGraphOptions(workspaceId, parentIssueId));
  const artifactQuery = useQuery(loopArtifactsOptions(workspaceId, parentIssueId));
  return <div className="flex min-h-0 flex-1 flex-col overflow-auto">
    <div className="border-b px-6 py-3"><Button variant="ghost" size="sm" className="gap-2" onClick={() => navigation.push(workspacePaths.loops())}><ArrowLeft className="size-4" aria-hidden="true" />Engineering Loops</Button></div>
    <div className="mx-auto w-full max-w-[1600px] p-6">
      {loopQuery.isLoading ? <div className="space-y-4" aria-label="Loading engineering loop"><Skeleton className="h-32 w-full rounded-xl" /><Skeleton className="h-80 w-full rounded-xl" /></div> : loopQuery.isError || !loopQuery.data ? <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-6 text-sm text-destructive">Failed to load this engineering loop.</div> : <div className="space-y-5">
        {graphQuery.isLoading ? <Skeleton className="h-52 w-full rounded-xl" /> : graphQuery.data ? <WorkGraph graph={graphQuery.data} /> : <div className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">Work Graph is temporarily unavailable. Loop execution detail remains available below.</div>}
        <LoopDetailPanel loop={loopQuery.data} artifacts={artifactQuery.data ?? []} copy={DETAIL_COPY} onOpenIssue={(issueId) => navigation.push(workspacePaths.issueDetail(issueId))} onOpenArtifact={(artifact) => { if (artifact.ref_uri && typeof window !== "undefined") window.open(artifact.ref_uri, "_blank", "noopener,noreferrer"); }} />
      </div>}
    </div>
  </div>;
}
