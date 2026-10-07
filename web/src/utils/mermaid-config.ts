import type { MermaidConfig } from 'mermaid'

/**
 * createMermaidConfig returns the secure diagram configuration for the requested
 * color scheme, preserving the flat dotted namespaces used by existing chats.
 */
export function createMermaidConfig(scheme: 'dark' | 'light'): MermaidConfig {
  return {
    startOnLoad: false,
    theme: scheme === 'dark' ? 'dark' : 'default',
    securityLevel: 'strict',
    fontFamily: 'inherit',
    class: {
      // Mermaid 11.15 defaults to nested groups for names such as api.v1.
      hierarchicalNamespaces: false,
    },
    flowchart: {
      useMaxWidth: true,
      htmlLabels: true,
      curve: 'basis',
    },
  }
}
