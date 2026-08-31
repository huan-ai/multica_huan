ALTER TABLE chat_message DROP COLUMN IF EXISTS sender_id;
ALTER TABLE chat_session DROP COLUMN IF EXISTS target_user_id;
ALTER TABLE chat_session ALTER COLUMN agent_id SET NOT NULL;
