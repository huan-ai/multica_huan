-- Allow agent_id to be NULL for human member-to-member direct chats
ALTER TABLE chat_session ALTER COLUMN agent_id DROP NOT NULL;

-- Add target_user_id for 1-on-1 member direct chats
ALTER TABLE chat_session ADD COLUMN IF NOT EXISTS target_user_id UUID;

-- Add sender_id to chat_message for recording which user sent the message in direct chats
ALTER TABLE chat_message ADD COLUMN IF NOT EXISTS sender_id UUID;
