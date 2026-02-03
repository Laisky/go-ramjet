import * as Dialog from '@radix-ui/react-dialog'
import { Download, Mail, Pencil, Save, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { setPageFavicon, setPageTitle } from '@/utils/dom'

import { parseCvContent } from './cv-helpers'
import { CvMarkdown } from './cv-markdown'

type CvContentPayload = {
  content: string
  updated_at?: string
  is_default: boolean
}
const AUTH_TOKEN_STORAGE_KEY = 'cv_sso_token'

/**
 * readAuthTokenFromURL pulls the SSO token from the current URL query string.
 */
function readAuthTokenFromURL(): string | null {
  const params = new URLSearchParams(window.location.search)
  const token = params.get('sso_token')
  if (!token) {
    return null
  }
  const trimmed = token.trim()
  return trimmed.length > 0 ? trimmed : null
}

/**
 * removeAuthTokenFromURL strips the SSO token from the browser URL.
 */
function removeAuthTokenFromURL() {
  const url = new URL(window.location.href)
  url.searchParams.delete('sso_token')
  const next = `${url.pathname}${url.search}${url.hash}`
  window.history.replaceState({}, document.title, next)
}

/**
 * readStoredAuthToken returns the SSO token stored in localStorage.
 */
function readStoredAuthToken(): string | null {
  const token = window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)
  if (!token) {
    return null
  }
  const trimmed = token.trim()
  return trimmed.length > 0 ? trimmed : null
}

/**
 * persistAuthToken saves the SSO token into localStorage.
 */
function persistAuthToken(token: string) {
  window.localStorage.setItem(AUTH_TOKEN_STORAGE_KEY, token)
}

/**
 * clearAuthToken removes the SSO token from localStorage.
 */
function clearAuthToken() {
  window.localStorage.removeItem(AUTH_TOKEN_STORAGE_KEY)
}

/**
 * buildAuthHeaders creates an Authorization header when a token is available.
 */
function buildAuthHeaders(token: string | null): HeadersInit {
  if (!token) {
    return {}
  }
  return {
    Authorization: `Bearer ${token}`,
  }
}

/**
 * setMetaTag updates or inserts a meta tag with the given attribute and value.
 */
function setMetaTag(key: 'name' | 'property', value: string, content: string) {
  const selector = `meta[${key}="${value}"]`
  let tag = document.querySelector<HTMLMetaElement>(selector)
  if (!tag) {
    tag = document.createElement('meta')
    tag.setAttribute(key, value)
    document.head.appendChild(tag)
  }
  tag.setAttribute('content', content)
}

/**
 * CVPage renders the CV presentation with an editor modal.
 */
export function CVPage() {
  const [content, setContent] = useState('')
  const [savedContent, setSavedContent] = useState('')
  const [lastSavedAt, setLastSavedAt] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [downloadBusy, setDownloadBusy] = useState(false)
  const [editorOpen, setEditorOpen] = useState(false)
  const [authToken, setAuthToken] = useState<string | null>(null)
  const [authMessage, setAuthMessage] = useState<string | null>(null)

  const parsed = useMemo(() => parseCvContent(content), [content])
  const isDirty = content !== savedContent
  const isEmpty = content.trim().length === 0
  const canEdit = Boolean(authToken)

  // loadContent fetches CV markdown from the backend API.
  const loadContent = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true)
      try {
        const response = await fetch('/cv/content', {
          signal,
          headers: buildAuthHeaders(authToken),
        })
        if (!response.ok) {
          throw new Error('Failed to load CV content')
        }
        const payload = (await response.json()) as CvContentPayload
        setContent(payload.content)
        setSavedContent(payload.content)
        setLastSavedAt(payload.updated_at ?? null)
      } catch (err) {
        console.error('[CV] Failed to load content')
      } finally {
        setLoading(false)
      }
    },
    [authToken],
  )

  // handleSave persists the current markdown to the backend API.
  const handleSave = useCallback(async () => {
    if (!authToken) {
      setAuthMessage('SSO token required to edit this CV.')
      return
    }
    if (saving) {
      return
    }
    setSaving(true)
    setAuthMessage(null)
    try {
      const response = await fetch('/cv/content', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          ...buildAuthHeaders(authToken),
        },
        body: JSON.stringify({ content }),
      })
      if (response.status === 401) {
        throw new Error('Unauthorized')
      }
      if (!response.ok) {
        throw new Error('Failed to save CV content')
      }
      const payload = (await response.json()) as CvContentPayload
      setSavedContent(payload.content)
      setLastSavedAt(payload.updated_at ?? null)
      setEditorOpen(false)
    } catch (err) {
      if (err instanceof Error && err.message === 'Unauthorized') {
        clearAuthToken()
        setAuthToken(null)
        setAuthMessage('SSO token expired. Please sign in again.')
        console.warn('[CV] Unauthorized SSO token')
      } else {
        console.error('[CV] Failed to save content')
      }
    } finally {
      setSaving(false)
    }
  }, [authToken, content, saving])

  // handleCancelEdit discards draft changes and closes the editor.
  const handleCancelEdit = useCallback(() => {
    setContent(savedContent)
    setEditorOpen(false)
    setAuthMessage(null)
  }, [savedContent])

  // handleDownloadPdf downloads the PDF asset or falls back to print.
  const handleDownloadPdf = useCallback(async () => {
    if (downloadBusy) {
      return
    }
    setDownloadBusy(true)
    try {
      const response = await fetch('/cv/pdf', {
        headers: buildAuthHeaders(authToken),
      })
      if (!response.ok) {
        throw new Error('PDF not available')
      }
      const blob = await response.blob()
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `${parsed.title.replace(/\s+/g, '-')}-CV.pdf`
      link.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      console.warn('[CV] PDF download failed, falling back to print')
      window.print()
    } finally {
      setDownloadBusy(false)
    }
  }, [authToken, downloadBusy, parsed.title])

  // handleCopyEmail copies the contact email to the clipboard.
  const handleCopyEmail = useCallback(async () => {
    if (!parsed.email) {
      return
    }
    try {
      await navigator.clipboard.writeText(parsed.email)
    } catch (err) {
      console.warn('[CV] Failed to copy email')
    }
  }, [parsed.email])

  // handleEditorOpenChange syncs editor open state and clears draft on close.
  const handleEditorOpenChange = useCallback(
    (open: boolean) => {
      setEditorOpen(open)
      if (!open) {
        setContent(savedContent)
        setAuthMessage(null)
      }
    },
    [savedContent],
  )

  useEffect(() => {
    const tokenFromURL = readAuthTokenFromURL()
    if (tokenFromURL) {
      persistAuthToken(tokenFromURL)
      setAuthToken(tokenFromURL)
      removeAuthTokenFromURL()
      return
    }
    const storedToken = readStoredAuthToken()
    if (storedToken) {
      setAuthToken(storedToken)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    loadContent(controller.signal)
    return () => controller.abort()
  }, [loadContent])

  useEffect(() => {
    if (!content) {
      return
    }
    const title = `${parsed.title} | Senior Software Engineer`
    setPageTitle(title)
    setPageFavicon('https://s3.laisky.com/uploads/2025/12/favicon.ico')
    setMetaTag('name', 'description', parsed.summaryLine)
    setMetaTag('property', 'og:title', title)
    setMetaTag('property', 'og:description', parsed.summaryLine)
    setMetaTag('property', 'og:type', 'profile')
    setMetaTag(
      'property',
      'og:image',
      'https://s3.laisky.com/uploads/2025/12/favicon.ico',
    )
  }, [content, parsed.summaryLine, parsed.title])

  return (
    <div className="cv-page">
      <div className="cv-shell">
        <header className="cv-hero cv-animate-in">
          <div className="cv-hero-text">
            <span className="cv-kicker">Curriculum Vitae</span>
            <h1 className="cv-title">{parsed.title}</h1>
            <p className="cv-subtitle">{parsed.subtitle}</p>
            <p className="cv-summary">{parsed.summaryLine}</p>
            <div className="cv-badges">
              {parsed.badges.map((badge) => (
                <span key={badge} className="cv-badge">
                  {badge}
                </span>
              ))}
            </div>
          </div>
          <div className="cv-hero-actions cv-no-print">
            <Button
              className="cv-primary-action"
              onClick={handleDownloadPdf}
              disabled={downloadBusy}
            >
              <Download className="h-4 w-4" />
              Download PDF
            </Button>
            <Dialog.Root
              open={editorOpen}
              onOpenChange={handleEditorOpenChange}
            >
              <Dialog.Trigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="cv-edit-button"
                  disabled={!canEdit}
                  title={
                    canEdit ? 'Edit CV content' : 'Sign in via SSO to edit'
                  }
                >
                  <Pencil className="h-4 w-4" />
                  Edit
                </Button>
              </Dialog.Trigger>
              <Dialog.Portal>
                <Dialog.Overlay className="cv-modal-overlay" />
                <Dialog.Content className="cv-modal-content">
                  <div className="cv-modal-header">
                    <div>
                      <Dialog.Title className="cv-modal-title">
                        Edit CV
                      </Dialog.Title>
                      <Dialog.Description className="cv-modal-description">
                        Update the markdown and save to refresh the live CV and
                        PDF.
                      </Dialog.Description>
                    </div>
                    <Dialog.Close asChild>
                      <button
                        type="button"
                        className="cv-modal-close"
                        aria-label="Close"
                      >
                        <X className="h-4 w-4" />
                      </button>
                    </Dialog.Close>
                  </div>
                  <div className="cv-modal-status">
                    <span>
                      {isDirty ? 'Unsaved changes' : 'All changes saved'}
                    </span>
                    <span>
                      {lastSavedAt
                        ? `Last saved ${new Date(lastSavedAt).toLocaleString()}`
                        : 'No saved data yet'}
                    </span>
                  </div>
                  {authMessage ? (
                    <div className="cv-modal-alert">{authMessage}</div>
                  ) : null}
                  <Textarea
                    className="cv-editor-textarea"
                    value={content}
                    onChange={(event) => setContent(event.target.value)}
                    spellCheck={false}
                    disabled={loading || !canEdit}
                    placeholder={
                      loading ? 'Loading...' : 'Write your CV in markdown'
                    }
                  />
                  <div className="cv-modal-actions">
                    <Button
                      variant="outline"
                      onClick={handleCancelEdit}
                      disabled={saving}
                    >
                      <X className="h-4 w-4" />
                      Cancel
                    </Button>
                    <Button
                      onClick={handleSave}
                      disabled={saving || loading || !isDirty || isEmpty}
                    >
                      <Save className="h-4 w-4" />
                      {saving ? 'Saving' : 'Save'}
                    </Button>
                  </div>
                </Dialog.Content>
              </Dialog.Portal>
            </Dialog.Root>
          </div>
        </header>

        <div className="cv-grid">
          <aside className="cv-aside cv-no-print">
            <div className="cv-card">
              <div className="cv-card-title">Contact</div>
              <div className="cv-card-body">
                <div className="cv-contact-email">
                  <Mail className="h-4 w-4" />
                  {parsed.email ?? 'job@laisky.com'}
                </div>
                <Button variant="outline" size="sm" onClick={handleCopyEmail}>
                  Copy Email
                </Button>
              </div>
              <div className="cv-link-list">
                {parsed.links.map((link) => (
                  <a key={link.href} href={link.href} className="cv-link-item">
                    {link.label}
                  </a>
                ))}
              </div>
            </div>
            <div className="cv-card">
              <div className="cv-card-title">Sections</div>
              <nav className="cv-nav">
                {parsed.navItems.map((item) => (
                  <a key={item.id} href={`#${item.id}`}>
                    {item.label}
                  </a>
                ))}
              </nav>
            </div>
            <div className="cv-card cv-meta-card">
              <div className="cv-card-title">Status</div>
              <div className="cv-meta">
                {loading
                  ? 'Loading content...'
                  : lastSavedAt
                    ? `Updated ${new Date(lastSavedAt).toLocaleString()}`
                    : 'Draft in memory'}
              </div>
            </div>
          </aside>

          <main className="cv-main cv-print cv-animate-in cv-animate-delay">
            {loading ? (
              <div className="cv-loading">Loading...</div>
            ) : (
              <div className="cv-content prose prose-slate max-w-none">
                <CvMarkdown content={parsed.previewContent} />
              </div>
            )}
            <div id="contact-actions" className="cv-contact-panel">
              <div className="cv-contact-title">Let’s talk</div>
              <div className="cv-contact-copy">
                <span>{parsed.email ?? 'job@laisky.com'}</span>
                <Button variant="outline" size="sm" onClick={handleCopyEmail}>
                  Copy Email
                </Button>
              </div>
              <p className="cv-contact-note">
                Best reached asynchronously via email.
              </p>
            </div>
          </main>
        </div>
      </div>
    </div>
  )
}
