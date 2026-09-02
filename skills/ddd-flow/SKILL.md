---
name: ddd-flow
description: "DDD-aicoding 2.1 research paradigm flow: initialize a DDD flow on a Multica issue, create stage sub-issues, submit artifacts for expert review, and poll review gates. Enforces human review gates at each stage before allowing progression."
user-invocable: true
allowed-tools: Bash(curl *), Bash(jq *), Read
---

# DDD Flow Skill

Orchestrates the DDD-aicoding 2.1 research paradigm flow through Multica's
issue system. Each DDD stage becomes a sub-issue under a parent issue, with
human review gates enforced between stages.

## Prerequisites

- A Multica workspace with API access
- `MULTICA_API_URL` environment variable (e.g. `https://your-instance.multica.ai`)
- `MULTICA_API_TOKEN` environment variable (your API token or CLI token)
- `MULTICA_WORKSPACE_ID` environment variable (your workspace UUID)
- `curl` and `jq` available in the shell

## Workflow Overview

### 1. Initialize a DDD Flow

Create a parent issue for the requirement, then initialize the DDD flow:

```bash
# Create the flow — this generates stage 0-2 sub-issues
curl -s -X POST "$MULTICA_API_URL/api/ddd-flows" \
  -H "Authorization: Bearer $MULTICA_API_TOKEN" \
  -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{"parent_issue_id": "<PARENT_ISSUE_UUID>"}' | jq .
```

### 2. Execute Each Stage

For each stage, the DDD skill (ddd-orchestrator, ddd-scenario-routing, etc.)
produces an artifact. After the skill completes:

**a. Upload the artifact:**
```bash
curl -s -X POST "$MULTICA_API_URL/api/upload-file" \
  -H "Authorization: Bearer $MULTICA_API_TOKEN" \
  -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" \
  -F "file=@./ddd-artifacts/<artifact-file>.md" | jq .
```

**b. Submit for review (create a comment on the stage sub-issue with @mention):**
```bash
curl -s -X POST "$MULTICA_API_URL/api/issues/<STAGE_ISSUE_ID>/comments" \
  -H "Authorization: Bearer $MULTICA_API_TOKEN" \
  -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "阶段产出物已提交，请评审。\n\n[@评审专家](mention://member/<REVIEWER_UUID>)",
    "attachment_ids": ["<ATTACHMENT_UUID>"]
  }' | jq .
```

**c. Update the stage sub-issue status to in_review:**
```bash
curl -s -X PUT "$MULTICA_API_URL/api/issues/<STAGE_ISSUE_ID>" \
  -H "Authorization: Bearer $MULTICA_API_TOKEN" \
  -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{"status": "in_review"}' | jq .
```

### 3. Poll Review Status

Wait for the reviewer to approve (set sub-issue to `done`):

```bash
curl -s "$MULTICA_API_URL/api/ddd-flows/<PARENT_ISSUE_ID>/stages/<STAGE_NUMBER>/review-status" \
  -H "Authorization: Bearer $MULTICA_API_TOKEN" \
  -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" | jq .
```

Response:
```json
{
  "stage": 1,
  "issue_id": "...",
  "status": "done",
  "gate_status": "done",
  "is_joint_gate": false,
  "approved": true
}
```

**Block until approved:**
```bash
while true; do
  STATUS=$(curl -s "$MULTICA_API_URL/api/ddd-flows/<PARENT_ID>/stages/<N>/review-status" \
    -H "Authorization: Bearer $MULTICA_API_TOKEN" \
    -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" | jq -r '.approved')
  if [ "$STATUS" = "true" ]; then
    echo "Stage approved, proceeding to next stage."
    break
  fi
  echo "Waiting for review approval..."
  sleep 30
done
```

### 4. Expand Flow After Scenario Routing

After g01 (scenario routing) confirms the scenario type, expand the flow
to create remaining stages:

```bash
curl -s -X POST "$MULTICA_API_URL/api/ddd-flows/<PARENT_ISSUE_ID>/expand" \
  -H "Authorization: Bearer $MULTICA_API_TOKEN" \
  -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{"scenario": "existing", "path": "full"}' | jq .
```

Scenario values: `greenfield`, `existing`, `legacy`
Path values (existing only): `full`, `light`

### 5. Query Flow State

Get a summary of all stages and their statuses:

```bash
curl -s "$MULTICA_API_URL/api/ddd-flows/<PARENT_ISSUE_ID>" \
  -H "Authorization: Bearer $MULTICA_API_TOKEN" \
  -H "X-Workspace-ID: $MULTICA_WORKSPACE_ID" | jq .
```

## Stage Reference

| Stage | Skill | Gate | Applies To |
|-------|-------|------|-----------|
| 0 | ddd-orchestrator | g00 | All |
| 1 | ddd-scenario-routing | g01 | All |
| 2 | ddd-facts-discovery | g02 | All |
| 3 | ddd-path-assessment | g03 | Existing only |
| 4 | ddd-light-path-design | g04 | Existing+light only |
| 5 | ddd-strategic-design | g05 | Full path only |
| 6 | ddd-application-contract-design | g06 | Full path only |
| 7 | ddd-tactical-modeling | g09 (joint) | Full path only |
| 8 | ddd-web-design | g09 (joint) | Full path only |
| 9 | ddd-convergence-review | g09 (joint) | Full path only |
| 10 | ddd-implementation-planning | g10 | All |
| 11 | ddd-implementation-execution | — | All |
| 12 | ddd-verification | g12 | All |
| 13 | ddd-feedback-rewrite | g13 | All |

## Joint Gate g09

Stages 7 (tactical modeling) and 8 (web design) run in parallel. Both, plus
stage 9 (convergence review), are approved through a single joint gate. The
`review-status` endpoint reports the joint gate status for any of these three
stages based on stage 9's status.

## Lightweight Path

For `existing` scenarios with `light` path, stages 5-9 are created with
`cancelled` status ("已跳过"). The flow proceeds directly from stage 4 to
stage 10 (implementation planning).

## Review Expert Selection

When submitting for review, the commenter chooses the reviewer by @mentioning
them. Reviewers can be:
- **Workspace members**: `[@Name](mention://member/<user-uuid>)`
- **AI agents**: `[@AgentName](mention://agent/<agent-uuid>)`

The reviewer receives both an inbox notification and an email notification
(when email is configured).
