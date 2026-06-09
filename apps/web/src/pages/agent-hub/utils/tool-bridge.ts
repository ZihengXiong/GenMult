import type { ToolCallBlock } from '@/store/chat-list'

// StoredTool is the shape AgentHub persists into a message's metadata.tools
// (see index.vue requestAgentReply): the WS stream's tool messages flattened to
// name + input + output. We bridge it back to home's ToolCallBlock so the
// existing tool-call rendering suite (diff view, exec output, web cards, …) can
// render AgentHub tools 1:1 instead of a raw <pre>JSON</pre>.
export interface StoredTool {
  name: string
  input: unknown
  output?: unknown
}

// Registry routing keys (tool-call-registry.ts getToolDisplay switch). Memoh
// tool names already match these; provider bridges (codex/claude) capitalize
// and rename, so we alias the common ones and let everything else fall through
// to the registry's generic branch.
const TOOL_NAME_ALIASES: Record<string, string> = {
  bash: 'exec',
  shell: 'exec',
  run: 'exec',
  webfetch: 'web_fetch',
  fetch: 'web_fetch',
  websearch: 'web_search',
  search: 'web_search',
  ls: 'list',
  glob: 'list',
}

// normalizeToolName lowercases the raw tool name and maps known provider
// aliases onto registry keys. Unknown names pass through lowercased and hit the
// registry's generic (expandable) fallback — never an error.
export function normalizeToolName(raw: string): string {
  const lower = (raw ?? '').trim().toLowerCase()
  return TOOL_NAME_ALIASES[lower] ?? lower
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

// normalizeEditInput maps provider edit-tool input keys onto the canonical
// path/old_text/new_text that ToolCallDetailEdit (and apply-edit) read, so a
// claude/codex Edit renders a real diff instead of an empty one. Memoh already
// uses the canonical keys, so the ?? chain is a no-op there.
function normalizeEditInput(input: unknown): unknown {
  const obj = asRecord(input)
  const path = obj.path ?? obj.file_path ?? obj.filename
  const oldText = obj.old_text ?? obj.old_string ?? obj.oldText
  const newText = obj.new_text ?? obj.new_string ?? obj.newText
  if (path === undefined && oldText === undefined && newText === undefined) {
    return input
  }
  return {
    ...obj,
    ...(path !== undefined ? { path } : {}),
    ...(oldText !== undefined ? { old_text: oldText } : {}),
    ...(newText !== undefined ? { new_text: newText } : {}),
  }
}

// toToolBlock adapts a StoredTool into a settled ToolCallBlock. AgentHub only
// renders completed turns, so done=true / running=false and there is never a
// pending approval (interactive approval is out of scope for the bridge path).
export function toToolBlock(tool: StoredTool, index: number): ToolCallBlock {
  const toolName = normalizeToolName(tool.name)
  const input = toolName === 'edit' ? normalizeEditInput(tool.input) : tool.input
  return {
    id: index,
    type: 'tool',
    name: tool.name,
    input,
    output: tool.output,
    tool_call_id: '',
    running: false,
    toolCallId: '',
    toolName,
    result: tool.output ?? null,
    done: true,
  }
}
