import { describe, expect, it } from 'vitest'
import { formatCostUsd } from '../format'

describe('formatCostUsd', () => {
  it('should return null for undefined', () => {
    expect(formatCostUsd(undefined)).toBe(null)
  })

  it('should return null for null', () => {
    expect(formatCostUsd(null)).toBe(null)
  })

  it('should return null for empty string', () => {
    expect(formatCostUsd('')).toBe(null)
  })

  it('should format number to 4 decimal places', () => {
    expect(formatCostUsd(0.1234)).toBe('0.1234')
    expect(formatCostUsd(0.12345)).toBe('0.1235') // rounds up
    expect(formatCostUsd(0.12344)).toBe('0.1234') // rounds down
    expect(formatCostUsd(1.5)).toBe('1.5000')
    expect(formatCostUsd(0)).toBe('0.0000')
  })

  it('should handle string numbers', () => {
    expect(formatCostUsd('0.1234')).toBe('0.1234')
    expect(formatCostUsd('1.5')).toBe('1.5000')
  })

  it('should return null for NaN strings', () => {
    expect(formatCostUsd('not a number')).toBe(null)
    expect(formatCostUsd('abc')).toBe(null)
  })

  it('should handle very small numbers', () => {
    expect(formatCostUsd(0.0001)).toBe('0.0001')
    expect(formatCostUsd(0.00005)).toBe('0.0001') // rounds up
  })

  it('should handle negative numbers', () => {
    expect(formatCostUsd(-0.1234)).toBe('-0.1234')
  })

  it('should handle large numbers', () => {
    expect(formatCostUsd(123.4567)).toBe('123.4567')
    expect(formatCostUsd(1000.1)).toBe('1000.1000')
  })
})
