CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_session_target_user ON chat_session (workspace_id, target_user_id);
