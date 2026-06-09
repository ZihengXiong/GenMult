import type { Component } from 'vue'
import type { AttachmentBlock } from '@/store/chat-list'
import type { StoredTool } from './utils/tool-bridge'

export type { StoredTool }

export interface AgentItem {
  id: string
  name: string
  kind: string
  status: 'online' | 'busy' | 'draft'
  icon: Component
  tone: string
  capabilities: string[]
  botId?: string
  framework?: string
}

export interface TimelineEvent {
  id: string
  time: string
  kind: string
  title: string
  body: string
  thinking?: string
  tools?: StoredTool[]
  icon: Component
  tone: string
  actions?: string[]
  // Sender identity (from the source message) so the room can resolve which
  // agent to re-run on regenerate and whether the bubble is a user message.
  senderId?: string
  senderType?: string
  // Reply reference rendered at the top of the bubble (from metadata.reply_to).
  replyTo?: { sender: string, preview: string }
  // Message-level attachments (from metadata.attachments) — partial Memoh demo.
  attachments?: AttachmentBlock
}
