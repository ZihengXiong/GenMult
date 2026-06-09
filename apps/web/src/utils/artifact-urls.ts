const URL_PATTERN = /https?:\/\/[^\s)<>\]"'`]+/g
const PREVIEWABLE_EXTENSIONS = /\.(html?|htm)$/i

// extractArtifactUrls pulls previewable artifact URLs out of message text so the
// chat can render an ArtifactPreviewCard (iframe) for them. A URL is previewable
// when it points at an .html page or a known preview host (localhost / vercel /
// netlify / github.io). Shared by home chat and AgentHub.
export function extractArtifactUrls(text?: string): string[] {
  if (!text) return []
  const matches = text.match(URL_PATTERN)
  if (!matches) return []
  const seen = new Set<string>()
  return matches.filter((url) => {
    if (seen.has(url)) return false
    seen.add(url)
    try {
      const u = new URL(url)
      return PREVIEWABLE_EXTENSIONS.test(u.pathname)
        || u.hostname === 'localhost'
        || u.hostname.endsWith('.vercel.app')
        || u.hostname.endsWith('.netlify.app')
        || u.hostname.endsWith('.github.io')
    } catch {
      return false
    }
  })
}
