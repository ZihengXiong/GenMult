import type { BotsBot } from '@memohai/sdk'

export function isVisibleBot(bot: BotsBot): boolean {
  return Boolean(bot)
}

export function visibleBots(bots: BotsBot[]): BotsBot[] {
  return bots.filter(isVisibleBot)
}
