import { API_BASE } from '@/utils/api'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { useEffect, useState } from 'react'

type VersionSetting = { Key: string; Value: string }
type VersionResponse = { Settings?: VersionSetting[] }

/**
 * useVersionCheck checks for server version updates.
 */
export function useVersionCheck() {
  const [upgradeInfo, setUpgradeInfo] = useState<{
    from: string
    to: string
  } | null>(null)

  useEffect(() => {
    let cancelled = false

    const checkUpgrade = async () => {
      try {
        const resp = await fetch(`${API_BASE}/version`, { cache: 'no-cache' })
        if (!resp.ok) return
        const data = (await resp.json()) as VersionResponse
        const serverVer = data.Settings?.find(
          (item) => item.Key === 'vcs.time',
        )?.Value
        if (!serverVer) return
        const localVer = await kvGet<string>(StorageKeys.VERSION_DATE)
        if (cancelled) return
        await kvSet(StorageKeys.VERSION_DATE, serverVer)
        if (localVer && localVer !== serverVer) {
          setUpgradeInfo({ from: localVer, to: serverVer })
        }
      } catch (err) {
        console.warn('Failed to check version:', err)
      }
    }

    checkUpgrade()
    return () => {
      cancelled = true
    }
  }, [])

  return {
    upgradeInfo,
    setUpgradeInfo,
  }
}
