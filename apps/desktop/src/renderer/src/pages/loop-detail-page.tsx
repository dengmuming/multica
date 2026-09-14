import { useParams } from "react-router-dom";
import { LoopDetailPage as LoopDetailView } from "@multica/views/loops";

export function LoopDetailPage() {
  const { parentIssueId } = useParams<{ parentIssueId: string }>();
  if (!parentIssueId) return null;
  return <LoopDetailView parentIssueId={parentIssueId} />;
}
