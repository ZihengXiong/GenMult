import type { BotsBot } from '@memohai/sdk'

export function isCodexSmokeBotName(name?: string | null): boolean {
  return /^codex-smoke-/i.test((name ?? '').trim())
}

export function resolveBotLabel(bot?: Pick<BotsBot, 'display_name' | 'id'> | null): string {
  const displayName = (bot?.display_name ?? '').trim()
  if (isCodexSmokeBotName(displayName)) return 'Codex'
  return displayName || (bot?.id ?? '').trim()
}
