"use client";

import { useQuery } from "@tanstack/react-query";
import { GitBranch } from "lucide-react";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { loopListOptions } from "@multica/core/loops/queries";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { useNavigation } from "../../navigation";
import { LoopList } from "./loop-list";

const LIST_COPY = {
  state: "State",
  template: "Template",
  updated: "Updated",
  empty: "No engineering loops yet.",
};

export function LoopsPage() {
  const workspaceId = useWorkspaceId();
  const workspacePaths = useWorkspacePaths();
  const navigation = useNavigation();
  const query = useQuery(loopListOptions(workspaceId));

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-auto">
      <div className="border-b px-6 py-5">
        <div className="flex items-center gap-3">
          <div className="flex size-9 items-center justify-center rounded-lg border bg-card">
            <GitBranch className="size-4" aria-hidden="true" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">Engineering Loops</h1>
            <p className="text-sm text-muted-foreground">
              Deterministic delivery flows from intent through approval and release.
            </p>
          </div>
        </div>
      </div>

      <div className="mx-auto w-full max-w-6xl p-6">
        {query.isLoading ? (
          <div className="space-y-2" aria-label="Loading engineering loops">
            <Skeleton className="h-20 w-full rounded-xl" />
            <Skeleton className="h-20 w-full rounded-xl" />
            <Skeleton className="h-20 w-full rounded-xl" />
          </div>
        ) : query.isError ? (
          <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-6 text-sm text-destructive">
            Failed to load engineering loops.
          </div>
        ) : (
          <LoopList
            loops={query.data ?? []}
            copy={LIST_COPY}
            formatDate={(value) => new Date(value).toLocaleString()}
            onSelectLoop={(parentIssueId) => navigation.push(workspacePaths.loopDetail(parentIssueId))}
          />
        )}
      </div>
    </div>
  );
}
