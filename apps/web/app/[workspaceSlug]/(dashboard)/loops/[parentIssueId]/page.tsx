"use client";

import { use } from "react";
import { LoopDetailPage } from "@multica/views/loops";

export default function Page({
  params,
}: {
  params: Promise<{ parentIssueId: string }>;
}) {
  const { parentIssueId } = use(params);
  return <LoopDetailPage parentIssueId={parentIssueId} />;
}
