import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

import { StreamingPcmResampler } from '../pcm-resampler'

// The browser loads this exact file from /audio/pcm-worklet.js; read it from disk
// so the test covers what ships rather than a bundled re-creation of it.
const workletPath = path.resolve(process.cwd(), 'public/audio/pcm-worklet.js')
const source = readFileSync(workletPath, 'utf8')

/** evaluateWorklet runs the shipped worklet in a scope that mimics AudioWorkletGlobalScope. */
function evaluateWorklet(sampleRate: number) {
  const registered = new Map<string, new () => WorkletProcessor>()
  class FakeProcessor {
    port = { postMessage: () => {} }
  }
  const run = new Function(
    'AudioWorkletProcessor',
    'registerProcessor',
    'sampleRate',
    source,
  )
  run(
    FakeProcessor,
    (name: string, ctor: new () => WorkletProcessor) =>
      registered.set(name, ctor),
    sampleRate,
  )
  return registered
}

interface WorkletProcessor {
  process(inputs: Float32Array[][]): boolean
  port: { postMessage: (data: ArrayBuffer, transfer: ArrayBuffer[]) => void }
}

describe('shipped microphone worklet', () => {
  it('contains no module syntax, which AudioWorkletGlobalScope cannot execute', () => {
    const code = source
      .split('\n')
      .filter((line) => !line.trimStart().startsWith('//'))
      .join('\n')
    expect(code).not.toMatch(/^\s*import\s/m)
    expect(code).not.toMatch(/^\s*export\s/m)
    expect(code).not.toMatch(/\bimport\s*\(/)
  })

  it('registers the processor name the client instantiates', () => {
    const registered = evaluateWorklet(48_000)
    // realtime-client.ts constructs new AudioWorkletNode(context, 'gptchat-microphone').
    expect([...registered.keys()]).toEqual(['gptchat-microphone'])
  })

  it('resamples identically to the shared StreamingPcmResampler', () => {
    const registered = evaluateWorklet(48_000)
    const Processor = registered.get('gptchat-microphone')!
    const frames: number[] = []
    const processor = new Processor() as unknown as WorkletProcessor
    processor.port.postMessage = (buffer: ArrayBuffer) => {
      frames.push(...new Int16Array(buffer))
    }

    const reference = new StreamingPcmResampler(48_000)
    const expected: number[] = []
    for (let block = 0; block < 40; block += 1) {
      const input = new Float32Array(128)
      for (let i = 0; i < input.length; i += 1) {
        input[i] = Math.sin((block * 128 + i) / 7)
      }
      processor.process([[input]])
      expected.push(...reference.append(input))
    }

    // The worklet only emits whole 480-sample frames, so compare that prefix.
    expect(frames.length).toBeGreaterThan(0)
    expect(frames).toEqual(expected.slice(0, frames.length))
  })
})
