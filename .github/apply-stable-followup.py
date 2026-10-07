from pathlib import Path


def replace(name, old, new):
    """replace applies a reviewed source transformation only to its exact target."""
    path = Path(name)
    text = path.read_text()
    if text.count(old) != 1:
        raise ValueError(f'{name}: expected one target {old[:75]!r}, found {text.count(old)}')
    path.write_text(text.replace(old, new))


replace('internal/tasks/cv/pdf.go', 'errors.WithStack(errors.New(fonts.ExceptionDetails.Error()))', 'errors.WithStack(&chromedp.ExceptionError{ExceptionDetails: fonts.ExceptionDetails})')
replace('web/src/utils/storage/idb-kv.ts', 'throw new Error(`Storage quota exceeded: ${message}`)', 'throw new Error(`Storage quota exceeded: ${message}`, { cause: err })')
replace('web/src/utils/storage/migration.ts', 'let PouchDBCtor: PouchCtor | null = null', 'let PouchDBCtor: PouchCtor')
replace('web/src/pages/gptchat/utils/build-api-messages.ts', 'let content: string | ContentPart[] = msg.content', 'let content: string | ContentPart[]')
replace('web/src/components/markdown.tsx', '    setRenderError(null)\n', '')
replace('web/src/components/markdown.tsx', '''        if (!containerRef.current || cancelled) {
          return
        }

        // Parse''', '''        if (!containerRef.current || cancelled) {
          return
        }
        setRenderError(null)

        // Parse''')
replace('web/src/pages/gptchat/payment.tsx', '  const [clientSecret, setClientSecret] = useState<string | null>(null)', '''  const [clientSecret, setClientSecret] = useState<string | null>(() =>
    new URLSearchParams(window.location.search).get('payment_intent_client_secret'),
  )''')
replace('web/src/pages/gptchat/payment.tsx', '''  // Check for client secret in URL (redirect from Stripe)
  useEffect(() => {
    const secret = new URLSearchParams(window.location.search).get(
      'payment_intent_client_secret',
    )
    if (secret) {
      setClientSecret(secret)
    }
  }, [])

''', '')
replace('web/src/pages/gptchat/hooks/use-prompt-shortcuts.ts', '    setPromptShortcuts(shortcuts)\n  }, [])', '    return shortcuts\n  }, [])')
replace('web/src/pages/gptchat/hooks/use-prompt-shortcuts.ts', '''    if (!configLoading) {
      loadPromptShortcuts()
    }
''', '''    if (configLoading) return
    let active = true
    void loadPromptShortcuts().then((shortcuts) => {
      if (active) setPromptShortcuts(shortcuts)
    }).catch((error: unknown) => {
      if (active) console.warn('Failed to load prompt shortcuts:', error)
    })
    return () => { active = false }
''')
replace('web/src/pages/gptchat/components/chat-search.tsx', '''  // Keep selectedSessionIds in sync with sessions list changes
  useEffect(() => {
    setSelectedSessionIds((prev) => {''', '''  // Reconcile changed input during render, before any stale filter reaches the DOM.
  const [previousSessions, setPreviousSessions] = useState(sessions)
  if (previousSessions !== sessions) {
    setPreviousSessions(sessions)
    setSelectedSessionIds((prev) => {''')
replace('web/src/pages/gptchat/components/chat-search.tsx', '''    })
  }, [sessions])

  // Open''', '''    })
  }

  // Open''')
replace('web/src/pages/gptchat/hooks/use-chat-scroll.ts', '  const [visibleCount, setVisibleCount] = useState(pageSize)', '''  const [visibleCount, setVisibleCount] = useState(() =>
    messages.length === 0 ? pageSize : Math.min(pageSize, messages.length),
  )
  const [previousBounds, setPreviousBounds] = useState({ length: messages.length, pageSize })
  if (previousBounds.length !== messages.length || previousBounds.pageSize !== pageSize) {
    setPreviousBounds({ length: messages.length, pageSize })
    setVisibleCount((previous) => messages.length === 0 ? pageSize :
      Math.min(messages.length, Math.max(Math.min(pageSize, messages.length), previous)))
  }''')
replace('web/src/pages/gptchat/hooks/use-chat-scroll.ts', '''  useEffect(() => {
    setVisibleCount((prev) => {
      // eslint-disable-line react-hooks/set-state-in-effect -- clamp visible count to message bounds
      if (messages.length === 0) {
        return pageSize
      }

      const desired = Math.min(pageSize, messages.length)

      if (prev < desired) {
        return desired
      }

      if (prev > messages.length) {
        return messages.length
      }

      return prev
    })
  }, [messages.length, pageSize])

''', '')
replace('web/src/pages/gptchat/index.tsx', '''    if (exists) {
      setPendingScrollTarget(null)
      scrollToMessage(chatId, role)
    }''', '''    if (!exists) return
    // Wait for committed layout, and cancel navigation when its target changes.
    const frame = requestAnimationFrame(() => {
      scrollToMessage(chatId, role)
      setPendingScrollTarget((current) => current === pendingScrollTarget ? null : current)
    })
    return () => cancelAnimationFrame(frame)''')
replace('web/src/pages/cv/index.tsx', '  const [authToken, setAuthToken] = useState<string | null>(null)', '  const [authToken, setAuthToken] = useState<string | null>(() => readAuthTokenFromURL() ?? readStoredAuthToken())')
replace('web/src/pages/cv/index.tsx', '  const [loading, setLoading] = useState(true)', '  const [loadedToken, setLoadedToken] = useState<{ token: string | null } | null>(null)')
replace('web/src/pages/cv/index.tsx', '  const cvPageRef = useRef<HTMLDivElement>(null)', '  const [cvPageElement, setCvPageElement] = useState<HTMLDivElement | null>(null)\n  const [copySequence, setCopySequence] = useState(0)\n  const loading = loadedToken === null || loadedToken.token !== authToken')
path = Path('web/src/pages/cv/index.tsx')
text = path.read_text().replace('ref={cvPageRef}', 'ref={setCvPageElement}').replace('container={cvPageRef.current}', 'container={cvPageElement}').replace('key={Date.now()}', 'key={copySequence}')
path.write_text(text)
replace('web/src/pages/cv/index.tsx', "      setCopyState('copied')", "      setCopyState('copied')\n      setCopySequence((sequence) => sequence + 1)")
replace('web/src/pages/cv/index.tsx', '''      setAuthToken(tokenFromURL)
      removeAuthTokenFromURL()''', '      removeAuthTokenFromURL()')
replace('web/src/pages/cv/index.tsx', '''    const storedToken = readStoredAuthToken()
    if (storedToken) {
      setAuthToken(storedToken)
    }
''', '')
path = Path('web/src/pages/cv/index.tsx')
text = path.read_text()
start = text.index('  // loadContent fetches')
end = text.index('  // handleSave persists', start)
text = text[:start] + '''  // loadContent retrieves a CV snapshot without changing component state.
  const loadContent = useCallback(async (signal: AbortSignal): Promise<CvContentPayload> => {
    const response = await fetch('/cv/content', { signal, headers: buildAuthHeaders(authToken) })
    if (!response.ok) throw new Error('Failed to load CV content')
    return response.json() as Promise<CvContentPayload>
  }, [authToken])

''' + text[end:]
start = text.index('  // loadHistory fetches')
end = text.index('  // handleCancelEdit', start)
text = text[:start] + '''  // loadHistory retrieves revisions without publishing stale results after the editor closes.
  const loadHistory = useCallback(async (signal: AbortSignal): Promise<CvContentHistoryPayload> => {
    const response = await fetch('/cv/content/history', { signal, headers: buildAuthHeaders(authToken) })
    if (response.status === 401) throw new Error('Unauthorized')
    if (!response.ok) throw new Error('Failed to load CV history')
    return response.json() as Promise<CvContentHistoryPayload>
  }, [authToken])

''' + text[end:]
path.write_text(text)
replace('web/src/pages/cv/index.tsx', '''      setEditorOpen(open)
      if (!open) {''', '''      setEditorOpen(open)
      setHistoryLoading(open)
      setHistoryMessage(null)
      if (!open) {''')
replace('web/src/pages/cv/index.tsx', '''    loadContent(controller.signal)
    return () => controller.abort()
  }, [loadContent])''', '''    void loadContent(controller.signal).then((payload) => {
      if (controller.signal.aborted) return
      setContent(payload.content)
      setSavedContent(payload.content)
      setLastSavedAt(payload.updated_at ?? null)
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) console.error('[CV] Failed to load content:', error)
    }).finally(() => {
      if (!controller.signal.aborted) setLoadedToken({ token: authToken })
    })
    return () => controller.abort()
  }, [authToken, loadContent])''')
replace('web/src/pages/cv/index.tsx', '''    loadHistory(controller.signal)
    return () => controller.abort()''', '''    void loadHistory(controller.signal).then((payload) => {
      if (!controller.signal.aborted) setHistoryEntries(payload.items)
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return
      if (error instanceof Error && error.message === 'Unauthorized') {
        clearAuthToken()
        setAuthToken(null)
        setAuthMessage('SSO token expired. Please sign in again.')
        setHistoryEntries([])
      } else {
        console.error('[CV] Failed to load history:', error)
        setHistoryMessage('Failed to load saved history.')
      }
    }).finally(() => {
      if (!controller.signal.aborted) setHistoryLoading(false)
    })
    return () => controller.abort()''')
replace('web/src/pages/gptchat/components/dataset-manager.tsx', '  const [isDatasetLoading, setIsDatasetLoading] = useState(false)', '''  const [isDatasetMutationLoading, setIsDatasetLoading] = useState(false)
  const requestKey = JSON.stringify([datasetKey, config.api_token, config.api_base])
  const [loadedRequestKey, setLoadedRequestKey] = useState<string | null>(null)
  const isDatasetLoading = isDatasetMutationLoading || Boolean(datasetKey && config.api_token && loadedRequestKey !== requestKey)''')
replace('web/src/pages/gptchat/components/dataset-manager.tsx', '''  useEffect(() => {
    if (!datasetKey) return
    refreshDatasets()
  }, [datasetKey, refreshDatasets])''', '''  useEffect(() => {
    if (!datasetKey || !config.api_token) return
    let active = true
    void api.listDatasets(datasetKey, config.api_token, config.api_base).then((response) => {
      if (!active) return
      setDatasets(response.datasets || [])
      setDatasetError(null)
    }).catch((error: unknown) => {
      if (active) setDatasetError(error instanceof Error ? error.message : String(error))
    }).finally(() => {
      if (active) setLoadedRequestKey(requestKey)
    })
    return () => { active = false }
  }, [datasetKey, config.api_token, config.api_base, requestKey])''')
