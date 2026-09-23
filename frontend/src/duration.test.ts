import { describe, expect, it } from 'vitest';
import { formatMinutes } from './duration';

describe('formatMinutes', () => {
  it('shows hours and minutes', () => {
    expect(formatMinutes(661n)).toBe('11 ч 1 мин');
    expect(formatMinutes(180)).toBe('3 ч');
    expect(formatMinutes(45)).toBe('45 мин');
    expect(formatMinutes(0)).toBe('0 мин');
  });
});
