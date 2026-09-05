import type { AudioPluginProps } from './plugin-types'
import { resolveAudioPlugin } from './plugin-registry'

interface AudioPluginControlProps extends AudioPluginProps {
  pluginID?: string
}

/** AudioPluginControl renders the selected implementation behind one stable contract. */
export function AudioPluginControl({
  pluginID,
  ...props
}: AudioPluginControlProps) {
  const definition = resolveAudioPlugin(pluginID)
  const Component = definition.component
  return <Component {...props} />
}
