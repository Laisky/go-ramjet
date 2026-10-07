import { getApiBase } from '@/utils/api'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { useCallback, useEffect, useRef, useState } from 'react'

type VersionSetting = { Key: string; Value: string }
type VersionResponse = { Settings?: VersionSetting[] }

const CHECK_INTERVAL = 3600000 // 1 hour

/**
 * useVersionCheck checks for server version updates.
 */
export function useVersionCheck() {
  const [upgradeInfo, setUpgradeInfo] = useState<{
    from: string
    to: string
  } | null>(null)
  const runningVersionRef = useRef<string | null>(null)

  const checkUpgrade = useCallback(async (signal: AbortSignal) => {
    if (signal.aborted) return
    try {
      const path = `${getApiBase()}/version`
      const url =
        typeof window !== 'undefined' && window.location?.origin
          ? new URL(path, window.location.origin).toString()
          : path
      const resp = await fetch(url, { cache: 'no-cache', signal })
      if (signal.aborted || !resp.ok) return
      const data = (await resp.json()) as VersionResponse
      if (signal.aborted) return
      const serverVer = data.Settings?.find(
        (item) => item.Key === 'vcs.time',
      )?.Value
      if (!serverVer) return

      // On first load, record the current version as running version
      if (!runningVersionRef.current) {
        runningVersionRef.current = serverVer
        await kvSet(StorageKeys.VERSION_DATE, serverVer)
        return
      }

      // If server version changed while app is running
      if (serverVer !== runningVersionRef.current) {
        const ignoredVer = await kvGet<string>(StorageKeys.IGNORED_VERSION)
        if (signal.aborted) return
        if (serverVer !== ignoredVer) {
          setUpgradeInfo({ from: runningVersionRef.current, to: serverVer })
        } else {
          setUpgradeInfo(null)
        }
      } else {
        setUpgradeInfo(null)
      }
    } catch (err) {
      if (!signal.aborted) {
        console.warn('Failed to check version:', err)
      }
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void checkUpgrade(controller.signal)
    const timer = setInterval(() => {
      void checkUpgrade(controller.signal)
    }, CHECK_INTERVAL)
    return () => {
      controller.abort()
      clearInterval(timer)
    }
  }, [checkUpgrade])

  const ignoreVersion = useCallback(async (version: string) => {
    await kvSet(StorageKeys.IGNORED_VERSION, version)
    setUpgradeInfo(null)
  }, [])

  return {
    upgradeInfo,
    setUpgradeInfo,
    ignoreVersion,
  }
}
