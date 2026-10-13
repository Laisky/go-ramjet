import { browserTelemetry } from './browser-sentry'

// Initialize before application imports; missing configuration leaves capture disabled.
browserTelemetry.initialize(
  {
    enabled: import.meta.env.VITE_CHAT_SENTRY_ENABLED,
    dsn: import.meta.env.VITE_CHAT_SENTRY_DSN,
    environment: import.meta.env.VITE_CHAT_SENTRY_ENVIRONMENT,
    release: import.meta.env.VITE_CHAT_SENTRY_RELEASE,
  },
  window.location.hostname,
)
