import { describe, expect, it } from 'vitest';
import { collectAllRoutes, maxExportPages, RevisionChangedError, type RoutePage } from './routeExport';
import type { RouteSourceView } from './TripRoutes';

// Synthetic saved results: 7 graph variants served 5 + 2, 1 Gemini variant.
function source(id: string, offset: number, count: number, hasMore: boolean): RouteSourceView {
  return { id, stage: 'awaiting_schedules', outcome: '', durationMs: 0, incomplete: true, total: 7, offset, hasMore, warnings: [],
    routes: Array.from({ length: count }, (_, i) => ({ steps: [{ description: `${id} ${offset + i + 1}`, mode: 'train', evidence: '' }], warnings: [] })) };
}

describe('collectAllRoutes', () => {
  it('reads every page, advancing by returned variants', async () => {
    const calls: string[] = [];
    const all = await collectAllRoutes(['graph', 'gemini'], async (id, offset): Promise<RoutePage> => {
      calls.push(`${id}@${offset}`);
      if (id === 'gemini') return { revision: 3n, source: source(id, 0, 1, false) };
      return { revision: 3n, source: offset === 0 ? source(id, 0, 5, true) : source(id, offset, 2, false) };
    });
    expect(calls).toEqual(['graph@0', 'graph@5', 'gemini@0']);
    expect(all.complete).toBe(true);
    expect(all.sources.map(s => s.routes.length)).toEqual([7, 1]);
    expect(all.sources[0].routes[6].steps[0].description).toBe('graph 7');
  });
  it('refuses to mix pages from different revisions', async () => {
    let revision = 1n;
    const fetch = async (id: string, offset: number): Promise<RoutePage> => ({ revision: revision++, source: source(id, offset, 5, offset === 0) });
    await expect(collectAllRoutes(['graph'], fetch)).rejects.toBeInstanceOf(RevisionChangedError);
  });
  it('marks the export partial at the page limit or on an empty page', async () => {
    const endless = await collectAllRoutes(['graph'], async (id, offset) => ({ revision: 1n, source: source(id, offset, 1, true) }));
    expect(endless.complete).toBe(false);
    expect(endless.sources[0].routes).toHaveLength(maxExportPages);
    const stalled = await collectAllRoutes(['graph'], async (id, offset) => ({ revision: 1n, source: source(id, offset, 0, true) }));
    expect(stalled.complete).toBe(false);
  });
});
