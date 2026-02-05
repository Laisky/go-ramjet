export type CvNavItem = {
  id: string
  label: string
}

export type CvLink = {
  label: string
  href: string
}

export type ParsedCv = {
  title: string
  subtitle: string
  summaryLine: string
  previewContent: string
  navItems: CvNavItem[]
  badges: string[]
  links: CvLink[]
  email?: string
}

const FALLBACK_TITLE = 'Zhonghua (Laisky) Cai'
const FALLBACK_SUBTITLE = 'Ottawa, ON, Canada | Open to remote (Canada/US)'
const FALLBACK_SUMMARY =
  'Senior Software Engineer | Backend and Infrastructure | PaaS and Platform'
const FALLBACK_BADGES = [
  '10+ years',
  'Go / Python / JavaScript',
  'Backend / Infra / Linux',
  'Security / TEE',
]

/**
 * slugify converts a heading label into a URL-friendly anchor ID.
 */
export function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, '')
    .trim()
    .replace(/\s+/g, '-')
}

/**
 * stripLeadingTitleBlock removes the first H1 block and returns title, subtitle, and remaining markdown.
 */
function stripLeadingTitleBlock(content: string): {
  title: string
  subtitle: string
  rest: string
} {
  const lines = content.split('\n')
  let title = FALLBACK_TITLE
  let subtitle = ''
  let startIndex = -1

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i].trim()
    if (line.startsWith('# ')) {
      title = line.replace(/^#\s+/, '').trim()
      startIndex = i + 1
      break
    }
  }

  if (startIndex < 0) {
    return { title, subtitle, rest: content }
  }

  let cursor = startIndex
  while (cursor < lines.length && lines[cursor].trim() === '') {
    cursor += 1
  }

  const subtitleLines: string[] = []
  while (cursor < lines.length) {
    const line = lines[cursor]
    if (line.trim() === '') {
      cursor += 1
      break
    }
    if (line.trim().startsWith('#')) {
      break
    }
    subtitleLines.push(line.trim())
    cursor += 1
  }

  if (subtitleLines.length > 0) {
    subtitle = subtitleLines.join(' ')
  }

  const rest = lines.slice(cursor).join('\n').replace(/^\n+/, '')
  return { title, subtitle, rest }
}

/**
 * extractSection removes a section by heading and returns its lines with the remainder.
 */
function extractSection(
  content: string,
  heading: string,
): {
  sectionLines: string[]
  rest: string
} {
  const lines = content.split('\n')
  const normalizedHeading = heading.toLowerCase()
  let start = -1
  let end = lines.length

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i].trim()
    if (line.toLowerCase().startsWith('## ')) {
      const title = line
        .replace(/^##\s+/, '')
        .trim()
        .toLowerCase()
      if (title === normalizedHeading) {
        start = i
        break
      }
    }
  }

  if (start < 0) {
    return { sectionLines: [], rest: content }
  }

  for (let i = start + 1; i < lines.length; i += 1) {
    if (lines[i].trim().toLowerCase().startsWith('## ')) {
      end = i
      break
    }
  }

  const sectionLines = lines.slice(start + 1, end)
  const restLines = [...lines.slice(0, start), ...lines.slice(end)]
  return { sectionLines, rest: restLines.join('\n').replace(/^\n+/, '') }
}

/**
 * extractSummaryLine returns the first non-empty line in the Summary section.
 */
function extractSummaryLine(content: string): string {
  const { sectionLines } = extractSection(content, 'summary')
  for (const line of sectionLines) {
    const trimmed = line.trim()
    if (trimmed) {
      return trimmed
    }
  }
  return ''
}

/**
 * extractNavItems collects H2 headings as anchor navigation items.
 */
function extractNavItems(content: string): CvNavItem[] {
  return content
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.startsWith('## '))
    .map((line) => line.replace(/^##\s+/, '').trim())
    .filter((label) => label.length > 0)
    .map((label) => ({ label, id: slugify(label) }))
}

/**
 * extractBadges parses bullet lines into badge strings.
 */
function extractBadges(lines: string[]): string[] {
  const badges = lines
    .map((line) => line.trim())
    .filter((line) => line.startsWith('- '))
    .map((line) => line.replace(/^-\s+/, '').trim())
    .filter((line) => line.length > 0)

  return badges.length > 0 ? badges : FALLBACK_BADGES
}

/**
 * extractLinks parses URLs and email addresses from the content.
 */
function extractLinks(content: string): { links: CvLink[]; email?: string } {
  const emailMatch = content.match(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i)
  const email = emailMatch ? emailMatch[0] : undefined

  const urlMatches = content.match(/https?:\/\/[^\s)]+/g) ?? []
  const links: CvLink[] = []

  const addLink = (label: string, href: string) => {
    if (
      label === 'GitHub' &&
      links.some(
        (item) => item.label === 'GitHub' || item.href.includes('github.com'),
      )
    ) {
      return
    }
    if (!links.some((item) => item.href === href)) {
      links.push({ label, href })
    }
  }

  urlMatches.forEach((url) => {
    if (url.includes('linkedin.com')) {
      addLink('LinkedIn', url)
      return
    }
    if (url.includes('github.com')) {
      addLink('GitHub', url)
      return
    }
    if (url.includes('blog')) {
      addLink('Blog', url)
      return
    }
    addLink('Link', url)
  })

  if (email) {
    addLink('Email', `mailto:${email}`)
  }

  return { links, email }
}

/**
 * parseCvContent derives hero metadata, navigation, and preview markdown from CV content.
 */
export function parseCvContent(content: string): ParsedCv {
  const { title, subtitle, rest } = stripLeadingTitleBlock(content)
  const summaryLine = extractSummaryLine(rest) || FALLBACK_SUMMARY
  const { sectionLines: badgeLines, rest: withoutProof } = extractSection(
    rest,
    'proof',
  )
  const badges = extractBadges(badgeLines)
  const navItems = extractNavItems(withoutProof)
  const { links, email } = extractLinks(content)

  return {
    title: title || FALLBACK_TITLE,
    subtitle: subtitle || FALLBACK_SUBTITLE,
    summaryLine,
    previewContent: withoutProof,
    navItems,
    badges,
    links,
    email,
  }
}
