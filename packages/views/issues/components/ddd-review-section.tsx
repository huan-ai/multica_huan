"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronRight, Edit3, X } from "lucide-react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { issueKeys } from "@multica/core/issues/queries";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";

interface DDDReviewSectionProps {
  issue: Issue;
  wsId: string;
}

export function DDDReviewSection({ issue, wsId }: DDDReviewSectionProps) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(true);
  const [comment, setComment] = useState("");
  const [selectedDecision, setSelectedDecision] = useState<string | null>(null);

  const isDDDStage =
    issue.stage !== null &&
    issue.stage !== undefined &&
    issue.parent_issue_id !== null &&
    issue.metadata?.ddd_flow === undefined;

  const hasDDDFlow = issue.metadata?.ddd_flow !== undefined;

  const isReviewable = isDDDStage && issue.status === "in_review";

  const reviewQuery = useQuery({
    queryKey: ["ddd-review", issue.id],
    queryFn: () => api.getDDDReview(issue.id),
    enabled: isDDDStage,
  });

  const submitReview = useMutation({
    mutationFn: (data: { decision: string; comment?: string }) =>
      api.submitDDDReview(issue.id, data),
    onSuccess: (result) => {
      const labels: Record<string, string> = {
        approved: "评审通过",
        changes_requested: "需要修改",
        rejected: "评审驳回",
      };
      toast.success(labels[result.decision] ?? result.decision);
      queryClient.invalidateQueries({ queryKey: ["ddd-review", issue.id] });
      queryClient.invalidateQueries({
        queryKey: issueKeys.detail(wsId, issue.id),
      });
      setComment("");
      setSelectedDecision(null);
    },
    onError: () => {
      toast.error("提交评审失败");
    },
  });

  if (!isDDDStage && !hasDDDFlow) return null;
  if (hasDDDFlow && !isDDDStage) return null;

  const review = reviewQuery.data;
  const hasExistingReview = review?.has_review === true;

  return (
    <div className="flex flex-col gap-1">
      <button
        type="button"
        className="flex items-center gap-1 text-caption font-medium text-muted-foreground hover:text-foreground py-1"
        onClick={() => setOpen(!open)}
      >
        <ChevronRight
          className={cn(
            "size-3.5 transition-transform",
            open && "rotate-90",
          )}
        />
        DDD 评审
      </button>

      {open && (
        <div className="pl-5 flex flex-col gap-3">
          {hasExistingReview && (
            <div className="flex flex-col gap-1 text-body">
              <div className="flex items-center gap-2">
                <DecisionBadge decision={review.decision} />
              </div>
              {review.comment && (
                <p className="text-caption text-muted-foreground">
                  {review.comment}
                </p>
              )}
              <p className="text-caption text-muted-foreground">
                {review.reviewed_at
                  ? new Date(review.reviewed_at).toLocaleString("zh-CN")
                  : ""}
              </p>
            </div>
          )}

          {isReviewable && (
            <div className="flex flex-col gap-2">
              <div className="flex gap-2">
                <Button
                  variant={
                    selectedDecision === "approved" ? "default" : "outline"
                  }
                  size="sm"
                  onClick={() => setSelectedDecision("approved")}
                  className="gap-1"
                >
                  <Check className="size-3.5" />
                  通过
                </Button>
                <Button
                  variant={
                    selectedDecision === "changes_requested"
                      ? "default"
                      : "outline"
                  }
                  size="sm"
                  onClick={() => setSelectedDecision("changes_requested")}
                  className="gap-1"
                >
                  <Edit3 className="size-3.5" />
                  需修改
                </Button>
                <Button
                  variant={
                    selectedDecision === "rejected"
                      ? "destructive"
                      : "outline"
                  }
                  size="sm"
                  onClick={() => setSelectedDecision("rejected")}
                  className="gap-1"
                >
                  <X className="size-3.5" />
                  驳回
                </Button>
              </div>

              {selectedDecision && (
                <>
                  <Textarea
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    placeholder="评审意见（可选）"
                    rows={3}
                    className="text-body"
                  />
                  <Button
                    size="sm"
                    disabled={submitReview.isPending}
                    onClick={() =>
                      submitReview.mutate({
                        decision: selectedDecision,
                        comment: comment || undefined,
                      })
                    }
                  >
                    {submitReview.isPending ? "提交中..." : "提交评审"}
                  </Button>
                </>
              )}
            </div>
          )}

          {!isReviewable && !hasExistingReview && (
            <p className="text-caption text-muted-foreground">
              {issue.status === "blocked"
                ? "等待前序阶段完成"
                : issue.status === "done"
                  ? "该阶段已完成"
                  : issue.status === "cancelled"
                    ? "该阶段已跳过"
                    : "等待提交评审"}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function DecisionBadge({ decision }: { decision: string }) {
  const config: Record<string, { label: string; className: string }> = {
    approved: {
      label: "✅ 评审通过",
      className: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
    },
    changes_requested: {
      label: "✏️ 需要修改",
      className: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
    },
    rejected: {
      label: "❌ 评审驳回",
      className: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
    },
  };
  const c = config[decision] ?? {
    label: decision,
    className: "bg-muted text-muted-foreground",
  };
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-md px-2 py-0.5 text-caption font-medium",
        c.className,
      )}
    >
      {c.label}
    </span>
  );
}
