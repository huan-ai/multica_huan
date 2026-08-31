"use client";

import React, { useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@multica/ui/components/ui/button";
import { ActorAvatar } from "../../common/actor-avatar";
import {
  PickerEmpty,
  PickerItem,
  PickerSection,
  PropertyPicker,
} from "../../issues/components/pickers/property-picker";
import { matchesPinyin } from "../../editor/extensions/pinyin-match";
import type { Agent, ChatSession, MemberWithUser } from "@multica/core/types";
import { isAgentRuntimeBound } from "@multica/core/agents";
import { useWorkspaceId } from "@multica/core/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import { chatKeys, chatSessionsOptions } from "@multica/core/chat/queries";
import { useCreateDirectChatSession } from "@multica/core/chat/mutations";
import { useChatStore } from "@multica/core/chat";
import { useAuthStore } from "@multica/core/auth";
import { toast } from "sonner";
import { useT } from "../../i18n";

export function AgentPicker({
  agents,
  userId,
  currentAgentId,
  onSelect,
  onSelectMember,
  trigger,
  triggerRender,
  side = "bottom",
  align = "start",
}: {
  agents: Agent[];
  userId: string | undefined;
  currentAgentId?: string;
  onSelect: (agent: Agent) => void;
  onSelectMember?: (session: ChatSession) => void;
  trigger: React.ReactNode;
  triggerRender: React.ReactElement;
  side?: "top" | "bottom";
  align?: "start" | "center" | "end";
}) {
  const { t } = useT("chat");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const createDirectChat = useCreateDirectChatSession();
  const setActiveSession = useChatStore((s) => s.setActiveSession);
  const currentUserId = useAuthStore((s) => s.user?.id);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: sessions = [] } = useQuery(chatSessionsOptions(wsId));

  const otherMembers = useMemo(
    () => members.filter((m) => m.user_id !== userId),
    [members, userId],
  );

  const { mine, others } = useMemo(() => {
    const mine: Agent[] = [];
    const others: Agent[] = [];
    for (const a of agents) {
      if (a.owner_id === userId) mine.push(a);
      else others.push(a);
    }
    return { mine, others };
  }, [agents, userId]);

  const query = filter.trim().toLowerCase();
  const matches = (name: string) =>
    !query || name.toLowerCase().includes(query) || matchesPinyin(name, query);

  const filteredMine = mine.filter((agent) => matches(agent.name));
  const filteredOthers = others.filter((agent) => matches(agent.name));
  const filteredMembers = otherMembers.filter(
    (member) => matches(member.name || member.email),
  );

  const handlePickAgent = (agent: Agent) => {
    onSelect(agent);
    setOpen(false);
  };

  const handlePickMember = (member: MemberWithUser) => {
    setOpen(false);
    // Reuse an existing active direct chat session with this member instead of
    // always creating a new one. Match both directions: I am creator or target.
    const existing = sessions.find(
      (s) =>
        s.status !== "archived" &&
        !s.agent_id &&
        ((s.creator_id === currentUserId && s.target_user_id === member.user_id) ||
          (s.creator_id === member.user_id && s.target_user_id === currentUserId)),
    );
    if (existing) {
      setActiveSession(existing.id);
      onSelectMember?.(existing);
      return;
    }
    createDirectChat.mutate(
      { target_user_id: member.user_id },
      {
        onSuccess: (session) => {
          qc.invalidateQueries({ queryKey: chatKeys.sessions(wsId) });
          setActiveSession(session.id);
          onSelectMember?.(session);
        },
        onError: (err) => {
          toast.error("无法发起私聊: " + (err as Error).message);
        },
      },
    );
  };

  const hasAnyItems =
    filteredMine.length > 0 || filteredOthers.length > 0 || filteredMembers.length > 0;

  return (
    <PropertyPicker
      open={open}
      onOpenChange={setOpen}
      width="w-64"
      align={align}
      side={side}
      searchable
      searchPlaceholder={t(($) => $.window.agent_filter_placeholder)}
      onSearchChange={setFilter}
      triggerRender={triggerRender}
      trigger={trigger}
    >
      {!hasAnyItems ? (
        <PickerEmpty />
      ) : (
        <>
          {filteredMine.length > 0 && (
            <PickerSection label={t(($) => $.window.my_agents)}>
              {filteredMine.map((agent) => (
                <AgentPickerItem
                  key={agent.id}
                  agent={agent}
                  isCurrent={agent.id === currentAgentId}
                  onSelect={handlePickAgent}
                />
              ))}
            </PickerSection>
          )}
          {filteredOthers.length > 0 && (
            <PickerSection label={t(($) => $.window.others)}>
              {filteredOthers.map((agent) => (
                <AgentPickerItem
                  key={agent.id}
                  agent={agent}
                  isCurrent={agent.id === currentAgentId}
                  onSelect={handlePickAgent}
                />
              ))}
            </PickerSection>
          )}
          {filteredMembers.length > 0 && (
            <PickerSection label="成员私聊">
              {filteredMembers.map((member) => (
                <MemberPickerItem
                  key={member.user_id}
                  member={member}
                  onSelect={handlePickMember}
                />
              ))}
            </PickerSection>
          )}
        </>
      )}
    </PropertyPicker>
  );
}

function AgentPickerItem({
  agent,
  isCurrent,
  onSelect,
}: {
  agent: Agent;
  isCurrent: boolean;
  onSelect: (agent: Agent) => void;
}) {
  const { t } = useT("chat");
  const runtimeBound = isAgentRuntimeBound(agent);
  return (
    <PickerItem
      selected={isCurrent}
      disabled={!runtimeBound}
      tooltip={
        runtimeBound ? undefined : t(($) => $.window.agent_needs_runtime_hint)
      }
      onClick={() => onSelect(agent)}
    >
      <ActorAvatar
        actorType="agent"
        actorId={agent.id}
        size="md"
        enableHoverCard
        showStatusDot
      />
      <span className="truncate flex-1">{agent.name}</span>
      {!runtimeBound && (
        <span className="shrink-0 text-micro text-amber-600 dark:text-amber-400">
          {t(($) => $.window.agent_needs_runtime)}
        </span>
      )}
    </PickerItem>
  );
}

function MemberPickerItem({
  member,
  onSelect,
}: {
  member: MemberWithUser;
  onSelect: (member: MemberWithUser) => void;
}) {
  return (
    <PickerItem selected={false} onClick={() => onSelect(member)}>
      <ActorAvatar
        actorType="member"
        actorId={member.user_id}
        size="md"
        showStatusDot
      />
      <div className="flex flex-col truncate flex-1 min-w-0">
        <span className="truncate text-body font-medium">{member.name || member.email}</span>
        {member.email && member.name && (
          <span className="truncate text-caption text-muted-foreground">{member.email}</span>
        )}
      </div>
    </PickerItem>
  );
}

export function NewChatButton({
  agents,
  userId,
  onStart,
  onSelectMember,
  side = "bottom",
}: {
  agents: Agent[];
  userId: string | undefined;
  onStart: (agent: Agent | null) => void;
  onSelectMember?: (session: ChatSession) => void;
  side?: "top" | "bottom";
}) {
  const { t } = useT("chat");
  const label = t(($) => $.window.new_chat_tooltip);

  return (
    <AgentPicker
      agents={agents}
      userId={userId}
      onSelect={(agent) => onStart(agent)}
      onSelectMember={onSelectMember}
      side={side}
      align="start"
      triggerRender={
        <Button
          variant="ghost"
          size="icon-sm"
          className="rounded-full text-muted-foreground"
          aria-label={label}
        />
      }
      trigger={<Plus />}
    />
  );
}
