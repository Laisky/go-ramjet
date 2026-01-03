import { Button } from '@/components/ui/button'

interface UpgradeNotificationProps {
  from: string
  to: string
  onClose: () => void
}

/**
 * UpgradeNotification displays a notification when a new version is available.
 */
export function UpgradeNotification({
  from,
  to,
  onClose,
}: UpgradeNotificationProps) {
  return (
    <div className="fixed bottom-4 right-4 z-50 max-w-sm rounded-lg border theme-border theme-elevated p-4 shadow-lg">
      <p className="text-sm font-medium">New version available</p>
      <p className="theme-text-muted text-xs">
        {from} → {to}
      </p>
      <div className="mt-3 flex gap-2">
        <Button size="sm" onClick={() => window.location.reload()}>
          Reload now
        </Button>
        <Button variant="ghost" size="sm" onClick={onClose}>
          Later
        </Button>
      </div>
    </div>
  )
}
