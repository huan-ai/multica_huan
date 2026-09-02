#!/usr/bin/env bash
# ddd-flow.sh — CLI helper for DDD-aicoding 2.1 flow management via Multica API.
# Usage: ./ddd-flow.sh <command> [args...]
#
# Commands:
#   init <parent-issue-id> [project-id]    Initialize DDD flow (stages 0-2)
#   status <parent-issue-id>               Show flow status
#   expand <parent-issue-id> <scenario> [path]  Expand flow after scenario routing
#   review <parent-issue-id> <stage>       Check review status of a stage
#   wait <parent-issue-id> <stage>         Block until stage is approved
#   submit <stage-issue-id> <file> <reviewer-id> [reviewer-type]  Submit artifact for review
#
# Environment:
#   MULTICA_API_URL       Base URL of the Multica instance
#   MULTICA_API_TOKEN     Bearer token for authentication
#   MULTICA_WORKSPACE_ID  Workspace UUID

set -euo pipefail

: "${MULTICA_API_URL:?MULTICA_API_URL is required}"
: "${MULTICA_API_TOKEN:?MULTICA_API_TOKEN is required}"
: "${MULTICA_WORKSPACE_ID:?MULTICA_WORKSPACE_ID is required}"

api() {
  local method=$1 path=$2
  shift 2
  curl -sf -X "$method" "${MULTICA_API_URL}${path}" \
    -H "Authorization: Bearer $MULTICA_API_TOKEN" \
    -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" \
    "$@"
}

cmd_init() {
  local parent_id=${1:?parent-issue-id required}
  local project_id=${2:-}
  local body="{\"parent_issue_id\":\"$parent_id\""
  if [ -n "$project_id" ]; then
    body="$body,\"project_id\":\"$project_id\""
  fi
  body="$body}"
  api POST /api/ddd-flows -H "Content-Type: application/json" -d "$body" | jq .
}

cmd_status() {
  local parent_id=${1:?parent-issue-id required}
  api GET "/api/ddd-flows/$parent_id" | jq .
}

cmd_expand() {
  local parent_id=${1:?parent-issue-id required}
  local scenario=${2:?scenario required (greenfield/existing/legacy)}
  local path=${3:-}
  local body="{\"scenario\":\"$scenario\""
  if [ -n "$path" ]; then
    body="$body,\"path\":\"$path\""
  fi
  body="$body}"
  api POST "/api/ddd-flows/$parent_id/expand" -H "Content-Type: application/json" -d "$body" | jq .
}

cmd_review() {
  local parent_id=${1:?parent-issue-id required}
  local stage=${2:?stage number required}
  api GET "/api/ddd-flows/$parent_id/stages/$stage/review-status" | jq .
}

cmd_wait() {
  local parent_id=${1:?parent-issue-id required}
  local stage=${2:?stage number required}
  local interval=${3:-30}
  echo "Waiting for stage $stage approval..."
  while true; do
    local result
    result=$(api GET "/api/ddd-flows/$parent_id/stages/$stage/review-status" 2>/dev/null || echo '{}')
    local approved
    approved=$(echo "$result" | jq -r '.approved // false')
    if [ "$approved" = "true" ]; then
      echo "Stage $stage approved."
      echo "$result" | jq .
      return 0
    fi
    local status
    status=$(echo "$result" | jq -r '.status // "unknown"')
    echo "  status=$status, waiting ${interval}s..."
    sleep "$interval"
  done
}

cmd_submit() {
  local stage_issue_id=${1:?stage-issue-id required}
  local file=${2:?artifact file path required}
  local reviewer_id=${3:?reviewer-id required}
  local reviewer_type=${4:-member}

  if [ ! -f "$file" ]; then
    echo "Error: file not found: $file" >&2
    exit 1
  fi

  # Upload the artifact.
  echo "Uploading artifact: $file"
  local upload_result
  upload_result=$(api POST /api/upload-file -F "file=@$file")
  local attachment_id
  attachment_id=$(echo "$upload_result" | jq -r '.id')

  if [ -z "$attachment_id" ] || [ "$attachment_id" = "null" ]; then
    echo "Error: upload failed" >&2
    echo "$upload_result" >&2
    exit 1
  fi
  echo "Uploaded: attachment_id=$attachment_id"

  # Create review comment with @mention.
  local mention_url="mention://$reviewer_type/$reviewer_id"
  local comment="阶段产出物已提交，请评审。\n\n[@评审专家]($mention_url)"
  api POST "/api/issues/$stage_issue_id/comments" \
    -H "Content-Type: application/json" \
    -d "{\"content\":\"$comment\",\"attachment_ids\":[\"$attachment_id\"]}" | jq .

  # Update stage status to in_review.
  api PUT "/api/issues/$stage_issue_id" \
    -H "Content-Type: application/json" \
    -d '{"status":"in_review"}' > /dev/null

  echo "Submitted for review. Stage issue $stage_issue_id is now in_review."
}

case "${1:-help}" in
  init)    shift; cmd_init "$@" ;;
  status)  shift; cmd_status "$@" ;;
  expand)  shift; cmd_expand "$@" ;;
  review)  shift; cmd_review "$@" ;;
  wait)    shift; cmd_wait "$@" ;;
  submit)  shift; cmd_submit "$@" ;;
  help|*)
    echo "Usage: $0 <command> [args...]"
    echo ""
    echo "Commands:"
    echo "  init <parent-issue-id> [project-id]"
    echo "  status <parent-issue-id>"
    echo "  expand <parent-issue-id> <scenario> [path]"
    echo "  review <parent-issue-id> <stage>"
    echo "  wait <parent-issue-id> <stage>"
    echo "  submit <stage-issue-id> <file> <reviewer-id> [reviewer-type]"
    ;;
esac
