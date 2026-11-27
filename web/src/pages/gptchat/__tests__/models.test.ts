import { describe, expect, it } from 'vitest'

import {
  DefaultModel,
  isChatModel,
  isFreeModel,
  isImageModel,
  isVisionModel,
  ModelCategories,
} from '../models'

describe('GPTChat catalog routing', () => {
  it.each([
    ['OpenAI', 'gpt-6.2-astra'],
    ['OpenAI', 'gpt-6.2-sol'],
    ['Anthropic', 'claude-haiku-5-5'],
    ['Anthropic', 'claude-fable-5-2'],
    ['Anthropic', 'claude-mythos-5-2'],
    ['Google', 'gemini-3.1-flash-image'],
  ])('lists %s model %s for chat and vision', (category, model) => {
    expect(ModelCategories[category]).toContain(model)
    expect(isChatModel(model)).toBe(true)
    expect(isVisionModel(model)).toBe(true)
    expect(isImageModel(model)).toBe(false)
  })

  it.each([
    'gpt-image-2.6-flare',
    'gpt-image-2.6-sunburst',
    'black-forest-labs/flux-2-dev',
    'gpt-image-2.5-flare',
    'black-forest-labs/flux-dev',
  ])('routes %s to image generation, including retained IDs', (model) => {
    expect(ModelCategories.Image).toContain(model)
    expect(isImageModel(model)).toBe(true)
    expect(isChatModel(model)).toBe(false)
  })

  it('keeps the free default and both DeepSeek Flash IDs available', () => {
    expect(DefaultModel).toBe('gpt-4o-mini')
    expect(isFreeModel(DefaultModel)).toBe(true)
    for (const model of ['deepseek-v4-flash', 'deepseek-flash']) {
      expect(ModelCategories.Deepseek).toContain(model)
      expect(isChatModel(model)).toBe(true)
      expect(isFreeModel(model)).toBe(true)
    }
  })
})
