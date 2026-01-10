import { API_BASE } from '@/utils/api'
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

  const checkUpgrade = useCallback(async () => {
    try {
      const resp = await fetch(`${API_BASE}/version`, { cache: 'no-cache' })
      if (!resp.ok) return
      const data = (await resp.json()) as VersionResponse
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
        if (serverVer !== ignoredVer) {
          setUpgradeInfo({ from: runningVersionRef.current, to: serverVer })
        } else {
          setUpgradeInfo(null)
        }
      } else {
        setUpgradeInfo(null)
      }
    } catch (err) {
      console.warn('Failed to check version:', err)
    }
  }, [])

  useEffect(() => {
    checkUpgrade()
    const timer = setInterval(checkUpgrade, CHECK_INTERVAL)
    return () => clearInterval(timer)
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
