import mermaid from 'mermaid'
import { describe, expect, it } from 'vitest'

import { createMermaidConfig } from './mermaid-config'

interface LayoutNode {
  id: string
  label?: string
  isGroup?: boolean
  parentId?: string
}

interface ClassLayoutDatabase {
  /** getData returns the nodes Mermaid passes to its layout renderer. */
  getData(): { nodes: LayoutNode[] }
}

const classDiagram = `classDiagram
namespace api.v1 {
  class User
}
namespace api.v2 {
  class Account
}
User --> Account
`

describe('Mermaid dependency compatibility', () => {
  it.each(['light', 'dark'] as const)(
    'preserves flat dotted namespaces in the %s theme',
    async (scheme) => {
      mermaid.initialize(createMermaidConfig(scheme))
      const diagram = await mermaid.mermaidAPI.getDiagramFromText(classDiagram)
      const database = diagram.db as unknown as ClassLayoutDatabase
      const { nodes } = database.getData()
      const groups = nodes.filter((node) => node.isGroup)

      expect(groups.map((node) => node.id)).toEqual(['api.v1', 'api.v2'])
      expect(groups.map((node) => node.label)).toEqual(['api.v1', 'api.v2'])
      expect(groups.every((node) => node.parentId === undefined)).toBe(true)
      expect(nodes.find((node) => node.id === 'User')?.parentId).toBe('api.v1')
      expect(nodes.find((node) => node.id === 'Account')?.parentId).toBe('api.v2')
    },
  )

  it.each([
    ['flowchart', 'flowchart TD\nA[Start] --> B[Finish]'],
    ['sequence', 'sequenceDiagram\nAlice->>Bob: Hello'],
  ])('continues parsing a %s diagram', async (_name, source) => {
    mermaid.initialize(createMermaidConfig('light'))
    await expect(mermaid.parse(source)).resolves.toBeTruthy()
  })

  it('rejects malformed diagrams instead of accepting invalid output', async () => {
    mermaid.initialize(createMermaidConfig('light'))
    await expect(mermaid.parse('not-a-diagram')).rejects.toThrow()
  })

  it.each(['light', 'dark'] as const)(
    'retains strict security and the expected %s theme',
    (scheme) => {
      const config = createMermaidConfig(scheme)
      expect(config.securityLevel).toBe('strict')
      expect(config.startOnLoad).toBe(false)
      expect(config.theme).toBe(scheme === 'dark' ? 'dark' : 'default')
      expect(config.flowchart).toEqual({
        useMaxWidth: true,
        htmlLabels: true,
        curve: 'basis',
      })
    },
  )
})
