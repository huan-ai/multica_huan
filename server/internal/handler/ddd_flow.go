package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// DDD stage definitions matching DDD-aicoding 2.1.
type dddStageDef struct {
	Stage        int32
	SkillName    string
	Title        string
	ArtifactName string
	GateName     string
	AppliesTo    string // "all", "existing_only", "existing_light_only", "full_only"
}

var dddAllStages = []dddStageDef{
	{0, "ddd-orchestrator", "准入 + 环境预检", "00-编排状态.yaml + 00-FACTS.md", "g00", "all"},
	{1, "ddd-scenario-routing", "场景分流", "01-场景分流建议.md", "g01", "all"},
	{2, "ddd-facts-discovery", "事实基础", "02-事实基础报告.md", "g02", "all"},
	{3, "ddd-path-assessment", "路径判定", "03-新增需求路径判定.md", "g03", "existing_only"},
	{4, "ddd-light-path-design", "轻量路径设计", "04-轻量路径设计.md", "g04", "existing_light_only"},
	{5, "ddd-strategic-design", "战略设计", "05-战略设计.md", "g05", "full_only"},
	{6, "ddd-application-contract-design", "应用契约草案", "06-应用契约草案.md", "g06", "full_only"},
	{7, "ddd-tactical-modeling", "战术建模", "07-战术建模.md", "g09", "full_only"},
	{8, "ddd-web-design", "前端Web设计", "08-前端Web设计.md", "g09", "full_only"},
	{9, "ddd-convergence-review", "契约收敛复核", "09-契约与并行收敛复核.md", "g09", "full_only"},
	{10, "ddd-implementation-planning", "实现规划", "10-实现与重构计划.md", "g10", "all"},
	{11, "ddd-implementation-execution", "实现执行", "11-实现报告.md", "", "all"},
	{12, "ddd-verification", "验证", "12-验证报告.md", "g12", "all"},
	{13, "ddd-feedback-rewrite", "反馈回写", "13-反馈回写.md", "g13", "all"},
}

type dddStageResp struct {
	Stage        int32  `json:"stage"`
	Title        string `json:"title"`
	SkillName    string `json:"skill_name"`
	ArtifactName string `json:"artifact_name"`
	GateName     string `json:"gate_name"`
	IssueID      string `json:"issue_id"`
	Status       string `json:"status"`
}

type dddFlowResp struct {
	ParentIssueID string         `json:"parent_issue_id"`
	Scenario      string         `json:"scenario"`
	Path          string         `json:"path,omitempty"`
	Stages        []dddStageResp `json:"stages"`
}

// InitDDDFlow creates the initial DDD stage sub-issues (stages 0-2) under a
// parent issue. POST /api/ddd-flows
func (h *Handler) InitDDDFlow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentIssueID string `json:"parent_issue_id"`
		ProjectID     string `json:"project_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ParentIssueID == "" {
		writeError(w, http.StatusBadRequest, "parent_issue_id is required")
		return
	}

	wsID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace_id")
	if !ok {
		return
	}
	parentUUID, ok := parseUUIDOrBadRequest(w, req.ParentIssueID, "parent_issue_id")
	if !ok {
		return
	}
	creatorID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	parent, err := h.Queries.GetIssue(r.Context(), parentUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "parent issue not found")
		return
	}
	if util.UUIDToString(parent.WorkspaceID) != wsID {
		writeError(w, http.StatusForbidden, "issue does not belong to this workspace")
		return
	}

	projID := parent.ProjectID
	if req.ProjectID != "" {
		pid, ok := parseUUIDOrBadRequest(w, req.ProjectID, "project_id")
		if !ok {
			return
		}
		projID = pid
	}

	stages, err := h.createDDDStages(r.Context(), r, wsID, wsUUID, req.ParentIssueID, projID, parent.Priority, creatorID, dddAllStages[:3])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.setDDDMeta(r.Context(), parentUUID, wsUUID, "ddd_flow", "initialized")
	h.setDDDMeta(r.Context(), parentUUID, wsUUID, "ddd_scenario", "pending")

	writeJSON(w, http.StatusCreated, dddFlowResp{
		ParentIssueID: req.ParentIssueID,
		Scenario:      "pending",
		Stages:        stages,
	})
}

// GetDDDFlow returns the current state of a DDD flow.
// GET /api/ddd-flows/{issueId}
func (h *Handler) GetDDDFlow(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueId")
	issueUUID, ok := parseUUIDOrBadRequest(w, issueID, "issueId")
	if !ok {
		return
	}
	wsID := h.resolveWorkspaceID(r)

	issue, err := h.Queries.GetIssue(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if util.UUIDToString(issue.WorkspaceID) != wsID {
		writeError(w, http.StatusForbidden, "issue does not belong to this workspace")
		return
	}

	children, err := h.Queries.ListChildIssues(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list children")
		return
	}

	meta := parseIssueMetadata(issue.Metadata)
	scenario, _ := meta["ddd_scenario"].(string)
	path, _ := meta["ddd_path"].(string)

	stages := make([]dddStageResp, 0, len(children))
	for _, child := range children {
		stageNum := int32(0)
		if child.Stage.Valid {
			stageNum = child.Stage.Int32
		}
		def := dddStageLookup(stageNum)
		sr := dddStageResp{
			Stage:   stageNum,
			Title:   child.Title,
			IssueID: util.UUIDToString(child.ID),
			Status:  child.Status,
		}
		if def != nil {
			sr.SkillName = def.SkillName
			sr.ArtifactName = def.ArtifactName
			sr.GateName = def.GateName
		}
		stages = append(stages, sr)
	}

	writeJSON(w, http.StatusOK, dddFlowResp{
		ParentIssueID: issueID,
		Scenario:      scenario,
		Path:          path,
		Stages:        stages,
	})
}

// ExpandDDDFlow creates remaining stages after scenario and path are confirmed.
// POST /api/ddd-flows/{issueId}/expand
func (h *Handler) ExpandDDDFlow(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueId")
	issueUUID, ok := parseUUIDOrBadRequest(w, issueID, "issueId")
	if !ok {
		return
	}
	var req struct {
		Scenario string `json:"scenario"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Scenario != "greenfield" && req.Scenario != "existing" && req.Scenario != "legacy" {
		writeError(w, http.StatusBadRequest, "scenario must be greenfield, existing, or legacy")
		return
	}
	if req.Scenario == "existing" && req.Path != "full" && req.Path != "light" {
		writeError(w, http.StatusBadRequest, "path must be full or light for existing scenario")
		return
	}

	wsID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace_id")
	if !ok {
		return
	}
	creatorID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	parent, err := h.Queries.GetIssue(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "parent issue not found")
		return
	}
	if util.UUIDToString(parent.WorkspaceID) != wsID {
		writeError(w, http.StatusForbidden, "issue does not belong to this workspace")
		return
	}

	existingChildren, err := h.Queries.ListChildIssues(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list children")
		return
	}
	existingStageSet := map[int32]bool{}
	for _, c := range existingChildren {
		if c.Stage.Valid {
			existingStageSet[c.Stage.Int32] = true
		}
	}

	isLightSkip := func(stage int32) bool {
		return req.Scenario == "existing" && req.Path == "light" && stage >= 5 && stage <= 9
	}

	var newDefs []dddStageDef
	for _, def := range dddAllStages {
		if existingStageSet[def.Stage] {
			continue
		}
		switch def.AppliesTo {
		case "all":
			newDefs = append(newDefs, def)
		case "existing_only":
			if req.Scenario == "existing" {
				newDefs = append(newDefs, def)
			}
		case "existing_light_only":
			if req.Scenario == "existing" && req.Path == "light" {
				newDefs = append(newDefs, def)
			}
		case "full_only":
			if req.Scenario == "greenfield" || req.Scenario == "legacy" {
				newDefs = append(newDefs, def)
			} else if req.Scenario == "existing" {
				// full path: include normally; light path: include as cancelled
				newDefs = append(newDefs, def)
			}
		}
	}

	stages, err := h.createDDDStages(r.Context(), r, wsID, wsUUID, issueID, parent.ProjectID, parent.Priority, creatorID, newDefs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Mark light-path-skipped stages as cancelled.
	for i := range stages {
		if isLightSkip(stages[i].Stage) {
			stages[i].Status = "cancelled"
		}
	}

	h.setDDDMeta(r.Context(), issueUUID, wsUUID, "ddd_scenario", req.Scenario)
	if req.Path != "" {
		h.setDDDMeta(r.Context(), issueUUID, wsUUID, "ddd_path", req.Path)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"parent_issue_id": issueID,
		"scenario":        req.Scenario,
		"path":            req.Path,
		"new_stages":      stages,
	})
}

// ReviewStageStatus polls the review status of a DDD stage.
// GET /api/ddd-flows/{issueId}/stages/{stage}/review-status
func (h *Handler) ReviewStageStatus(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueId")
	stageStr := chi.URLParam(r, "stage")
	stage, err := strconv.Atoi(stageStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid stage number")
		return
	}
	issueUUID, ok := parseUUIDOrBadRequest(w, issueID, "issueId")
	if !ok {
		return
	}
	wsID := h.resolveWorkspaceID(r)

	parent, err := h.Queries.GetIssue(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if util.UUIDToString(parent.WorkspaceID) != wsID {
		writeError(w, http.StatusForbidden, "issue does not belong to this workspace")
		return
	}

	children, err := h.Queries.ListChildIssues(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list children")
		return
	}

	for _, child := range children {
		if !child.Stage.Valid || child.Stage.Int32 != int32(stage) {
			continue
		}
		isJoint := stage >= 7 && stage <= 9
		gateStatus := child.Status
		if isJoint {
			for _, c := range children {
				if c.Stage.Valid && c.Stage.Int32 == 9 {
					gateStatus = c.Status
					break
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"stage":         stage,
			"issue_id":      util.UUIDToString(child.ID),
			"status":        child.Status,
			"gate_status":   gateStatus,
			"is_joint_gate": isJoint,
			"approved":      child.Status == "done",
		})
		return
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("stage %d not found", stage))
}

// createDDDStages creates sub-issues for each stage definition.
func (h *Handler) createDDDStages(
	ctx context.Context,
	r *http.Request,
	wsID string,
	wsUUID pgtype.UUID,
	parentIssueID string,
	projectID pgtype.UUID,
	priority string,
	creatorID string,
	defs []dddStageDef,
) ([]dddStageResp, error) {
	parentUUID := parseUUID(parentIssueID)
	creatorUUID := parseUUID(creatorID)
	prefix := h.getIssuePrefix(ctx, wsUUID)

	out := make([]dddStageResp, 0, len(defs))
	for _, def := range defs {
		status := "blocked"
		if def.Stage == 0 {
			status = "todo"
		}

		desc := fmt.Sprintf("DDD 阶段 %d: %s\n\n技能: `%s`\n产出物: `%s`\n门禁: `%s`",
			def.Stage, def.Title, def.SkillName, def.ArtifactName, def.GateName)

		res, err := h.IssueService.Create(ctx, service.IssueCreateParams{
			WorkspaceID:    wsUUID,
			Title:          fmt.Sprintf("[DDD-%d] %s", def.Stage, def.Title),
			Description:    pgtype.Text{String: desc, Valid: true},
			Status:         status,
			Priority:       priority,
			CreatorType:    "member",
			CreatorID:      creatorUUID,
			ParentIssueID:  parentUUID,
			ProjectID:      projectID,
			Stage:          pgtype.Int4{Int32: def.Stage, Valid: true},
			AllowDuplicate: true,
		}, service.IssueCreateOpts{
			ActorID: creatorID,
			BroadcastPayload: func(issue db.Issue, _ []db.Attachment, _ []db.IssueLabel) map[string]any {
				return map[string]any{"issue": issueToResponse(issue, prefix)}
			},
		})
		if err != nil {
			return nil, fmt.Errorf("stage %d: %w", def.Stage, err)
		}

		out = append(out, dddStageResp{
			Stage:        def.Stage,
			Title:        def.Title,
			SkillName:    def.SkillName,
			ArtifactName: def.ArtifactName,
			GateName:     def.GateName,
			IssueID:      util.UUIDToString(res.Issue.ID),
			Status:       status,
		})
	}
	return out, nil
}

func (h *Handler) setDDDMeta(ctx context.Context, issueID, wsID pgtype.UUID, key, value string) {
	val, _ := json.Marshal(value)
	_, _ = h.Queries.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{
		ID: issueID, WorkspaceID: wsID, Key: key, Value: val,
	})
}

func dddStageLookup(stage int32) *dddStageDef {
	for i := range dddAllStages {
		if dddAllStages[i].Stage == stage {
			return &dddAllStages[i]
		}
	}
	return nil
}

// --- DDD Flow Enforcement ---

// DDDFlowCheck describes the result of a DDD flow enforcement check.
type DDDFlowCheck struct {
	Blocked bool   // true if the status transition is blocked
	Reason  string // human-readable reason (empty if not blocked)
}

// CheckDDDFlowStatusTransition enforces DDD flow rules on issue status changes.
// Called from UpdateIssue before the update is persisted.
//
// Rules enforced:
//  1. Parent issue with DDD flow cannot move to "done" unless all stage sub-issues
//     are "done" or "cancelled".
//  2. DDD stage sub-issue cannot move to "in_review" unless it has at least one
//     attachment (artifact).
//  3. DDD stage sub-issue cannot move to "done" unless a reviewer has posted an
//     approval comment (containing "通过" or "approved").
func (h *Handler) CheckDDDFlowStatusTransition(ctx context.Context, issue db.Issue, newStatus string) DDDFlowCheck {
	prevStatus := issue.Status

	// Only check when status is actually changing.
	if newStatus == "" || newStatus == prevStatus {
		return DDDFlowCheck{}
	}

	// Rule 1: Parent issue with DDD flow → done requires all stages complete.
	if newStatus == "done" {
		meta := parseIssueMetadata(issue.Metadata)
		if _, hasDDD := meta["ddd_flow"]; hasDDD {
			return h.checkDDDParentCompletion(ctx, issue)
		}
	}

	// Rules 2 & 3 apply to DDD stage sub-issues (children with a stage field).
	if !issue.Stage.Valid || !issue.ParentIssueID.Valid {
		return DDDFlowCheck{}
	}

	// Check if the parent has a DDD flow.
	parent, err := h.Queries.GetIssue(ctx, issue.ParentIssueID)
	if err != nil {
		return DDDFlowCheck{}
	}
	parentMeta := parseIssueMetadata(parent.Metadata)
	if _, hasDDD := parentMeta["ddd_flow"]; !hasDDD {
		return DDDFlowCheck{}
	}

	// Rule 2: Stage → in_review requires at least one attachment.
	if newStatus == "in_review" {
		return h.checkDDDStageHasArtifact(ctx, issue)
	}

	// Rule 3: Stage → done requires reviewer approval.
	if newStatus == "done" {
		return h.checkDDDStageReviewApproval(ctx, issue)
	}

	return DDDFlowCheck{}
}

// checkDDDParentCompletion checks that all DDD stage sub-issues are done or cancelled.
func (h *Handler) checkDDDParentCompletion(ctx context.Context, parent db.Issue) DDDFlowCheck {
	children, err := h.Queries.ListChildIssues(ctx, parent.ID)
	if err != nil {
		return DDDFlowCheck{}
	}

	var incomplete []string
	for _, child := range children {
		if !child.Stage.Valid {
			continue
		}
		if child.Status != "done" && child.Status != "cancelled" {
			def := dddStageLookup(child.Stage.Int32)
			title := child.Title
			if def != nil {
				title = fmt.Sprintf("阶段 %d: %s", def.Stage, def.Title)
			}
			incomplete = append(incomplete, fmt.Sprintf("%s (状态: %s)", title, child.Status))
		}
	}

	if len(incomplete) == 0 {
		return DDDFlowCheck{}
	}

	reason := fmt.Sprintf("DDD 流程未完成，以下阶段尚未通过评审：%s", joinWithComma(incomplete))
	return DDDFlowCheck{Blocked: true, Reason: reason}
}

// checkDDDStageHasArtifact checks that the stage sub-issue has at least one attachment.
func (h *Handler) checkDDDStageHasArtifact(ctx context.Context, issue db.Issue) DDDFlowCheck {
	atts, err := h.Queries.ListAttachmentsByIssue(ctx, db.ListAttachmentsByIssueParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil || len(atts) > 0 {
		return DDDFlowCheck{}
	}

	def := dddStageLookup(issue.Stage.Int32)
	artifactName := "产出物"
	if def != nil {
		artifactName = def.ArtifactName
	}
	return DDDFlowCheck{
		Blocked: true,
		Reason:  fmt.Sprintf("请先上传阶段产出物（%s）再提交评审", artifactName),
	}
}

// checkDDDStageReviewApproval checks that a reviewer has submitted an "approved" decision.
func (h *Handler) checkDDDStageReviewApproval(ctx context.Context, issue db.Issue) DDDFlowCheck {
	meta := parseIssueMetadata(issue.Metadata)
	decision, _ := meta["ddd_review_decision"].(string)
	if decision == "approved" {
		return DDDFlowCheck{}
	}

	return DDDFlowCheck{
		Blocked: true,
		Reason:  "该阶段尚未通过评审，请等待评审专家提交评审决策后再标记为 done",
	}
}

// SubmitDDDReview handles the review decision for a DDD stage sub-issue.
// POST /api/issues/{id}/ddd-review
func (h *Handler) SubmitDDDReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	issue, ok := h.loadIssueForUser(w, r, id)
	if !ok {
		return
	}

	var req struct {
		Decision string `json:"decision"` // approved, changes_requested, rejected
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision != "approved" && req.Decision != "changes_requested" && req.Decision != "rejected" {
		writeError(w, http.StatusBadRequest, "decision must be approved, changes_requested, or rejected")
		return
	}

	wsID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace_id")
	if !ok {
		return
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Store review decision in issue metadata.
	keys := map[string]string{
		"ddd_review_decision":  req.Decision,
		"ddd_review_comment":   req.Comment,
		"ddd_reviewer_id":      userID,
		"ddd_reviewer_type":    "member",
		"ddd_reviewed_at":      now,
	}
	for k, v := range keys {
		val, _ := json.Marshal(v)
		h.Queries.SetIssueMetadataKey(r.Context(), db.SetIssueMetadataKeyParams{
			ID: issue.ID, WorkspaceID: wsUUID, Key: k, Value: val,
		})
	}

	// Also post a comment with the review decision for audit trail.
	decisionLabel := map[string]string{
		"approved":          "✅ 评审通过",
		"changes_requested": "✏️ 需要修改",
		"rejected":          "❌ 评审驳回",
	}[req.Decision]
	commentContent := decisionLabel
	if req.Comment != "" {
		commentContent += "\n\n" + req.Comment
	}
	h.Queries.CreateComment(r.Context(), db.CreateCommentParams{
		IssueID:     issue.ID,
		AuthorType:  "member",
		AuthorID:    parseUUID(userID),
		Content:     commentContent,
		Type:        "comment",
		WorkspaceID: wsUUID,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"decision":      req.Decision,
		"comment":       req.Comment,
		"reviewer_id":   userID,
		"reviewer_type": "member",
		"reviewed_at":   now,
	})
}

// GetDDDReview returns the current review status for a DDD stage sub-issue.
// GET /api/issues/{id}/ddd-review
func (h *Handler) GetDDDReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	issue, ok := h.loadIssueForUser(w, r, id)
	if !ok {
		return
	}

	meta := parseIssueMetadata(issue.Metadata)

	decision, _ := meta["ddd_review_decision"].(string)
	comment, _ := meta["ddd_review_comment"].(string)
	reviewerID, _ := meta["ddd_reviewer_id"].(string)
	reviewerType, _ := meta["ddd_reviewer_type"].(string)
	reviewedAt, _ := meta["ddd_reviewed_at"].(string)

	writeJSON(w, http.StatusOK, map[string]any{
		"decision":      decision,
		"comment":       comment,
		"reviewer_id":   reviewerID,
		"reviewer_type": reviewerType,
		"reviewed_at":   reviewedAt,
		"has_review":    decision != "",
	})
}

func joinWithComma(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, "、")
}
