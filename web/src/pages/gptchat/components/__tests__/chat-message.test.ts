/**
 * Tests for chat-message.tsx utility functions
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { formatCostUsd } from '../../utils/format'

describe('formatCostUsd', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should return null for undefined', () => {
    expect(formatCostUsd(undefined)).toBeNull()
  })

  it('should return null for null', () => {
    expect(formatCostUsd(null)).toBeNull()
  })

  it('should return null for empty string', () => {
    expect(formatCostUsd('')).toBeNull()
  })

  it('should format a number correctly', () => {
    expect(formatCostUsd(0.0012)).toBe('0.0012')
    expect(formatCostUsd(0.00123456)).toBe('0.0012')
    expect(formatCostUsd(1.23456789)).toBe('1.2346')
    expect(formatCostUsd(0)).toBe('0.0000')
  })

  it('should format a numeric string correctly (backward compatibility)', () => {
    expect(formatCostUsd('0.0012')).toBe('0.0012')
    expect(formatCostUsd('0.00123456')).toBe('0.0012')
    expect(formatCostUsd('1.23456789')).toBe('1.2346')
    expect(formatCostUsd('0')).toBe('0.0000')
  })

  it('should return null for non-numeric strings', () => {
    // Mock console.debug to avoid noise in test output
    const debugSpy = vi.spyOn(console, 'debug').mockImplementation(() => {})

    expect(formatCostUsd('invalid')).toBeNull()
    expect(formatCostUsd('abc123')).toBeNull()
    expect(formatCostUsd('NaN')).toBeNull()

    expect(debugSpy).toHaveBeenCalled()
    debugSpy.mockRestore()
  })

  it('should handle NaN input', () => {
    const debugSpy = vi.spyOn(console, 'debug').mockImplementation(() => {})

    expect(formatCostUsd(NaN)).toBeNull()

    debugSpy.mockRestore()
  })

  it('should handle Infinity input', () => {
    // Infinity.toFixed() returns "Infinity" which is technically valid
    // but we should test the behavior
    expect(formatCostUsd(Infinity)).toBe('Infinity')
  })

  it('should handle negative numbers', () => {
    expect(formatCostUsd(-0.0012)).toBe('-0.0012')
  })

  it('should handle very small numbers', () => {
    expect(formatCostUsd(0.00001234)).toBe('0.0000')
  })

  it('should handle scientific notation strings', () => {
    expect(formatCostUsd('1.5e-3')).toBe('0.0015')
  })

  it('should handle boolean false (edge case)', () => {
    // Boolean false converts to 0 via Number()
    expect(formatCostUsd(false)).toBe('0.0000')
  })

  it('should return null for objects', () => {
    const debugSpy = vi.spyOn(console, 'debug').mockImplementation(() => {})

    expect(formatCostUsd({})).toBeNull()
    expect(formatCostUsd({ value: 0.1 })).toBeNull()

    debugSpy.mockRestore()
  })
})
