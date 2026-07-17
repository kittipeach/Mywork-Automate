import { describe, it, expect } from 'vitest';
import { fmtDuration, fmtBytes, fmtDateTime, stepDotColor } from './format';

describe('fmtDuration', () => {
  it('renders em dash for 0 / negative', () => {
    expect(fmtDuration(0)).toBe('—');
    expect(fmtDuration(-5)).toBe('—');
  });
  it('renders milliseconds under a second', () => {
    expect(fmtDuration(5)).toBe('5ms');
    expect(fmtDuration(999)).toBe('999ms');
  });
  it('renders seconds under a minute', () => {
    expect(fmtDuration(2103)).toBe('2s');
    expect(fmtDuration(59000)).toBe('59s');
  });
  it('renders minutes + seconds at/over a minute', () => {
    expect(fmtDuration(48213)).toBe('48s');
    expect(fmtDuration(65000)).toBe('1m 5s');
    expect(fmtDuration(600000)).toBe('10m 0s');
  });
});

describe('fmtBytes', () => {
  it('em dash for 0 / negative', () => {
    expect(fmtBytes(0)).toBe('—');
    expect(fmtBytes(-1)).toBe('—');
  });
  it('bytes without decimals', () => {
    expect(fmtBytes(512)).toBe('512 B');
  });
  it('scales into KB/MB/GB with one decimal', () => {
    expect(fmtBytes(1024)).toBe('1 KB');
    expect(fmtBytes(1536)).toBe('1.5 KB');
    expect(fmtBytes(1048576)).toBe('1 MB');
    expect(fmtBytes(1073741824)).toBe('1 GB');
  });
});

describe('fmtDateTime', () => {
  it('formats an ISO string to YYYY-MM-DD HH:MM', () => {
    expect(fmtDateTime('2026-07-14T06:00:00.000Z')).toBe('2026-07-14 06:00');
  });
  it('em dash for empty / invalid', () => {
    expect(fmtDateTime('')).toBe('—');
    expect(fmtDateTime('not-a-date')).toBe('—');
  });
});

describe('stepDotColor', () => {
  it('maps each status to a class', () => {
    expect(stepDotColor('success')).toBe('bg-success');
    expect(stepDotColor('failed')).toBe('bg-danger');
    expect(stepDotColor('running')).toContain('bg-info');
    expect(stepDotColor('running')).toContain('animate-pulse');
    expect(stepDotColor('cancelled')).toBe('bg-ink-subtle');
    expect(stepDotColor('skipped')).toBe('bg-ink-subtle');
    expect(stepDotColor('queued')).toBe('bg-border');
    expect(stepDotColor('draft')).toBe('bg-border');
  });
});
