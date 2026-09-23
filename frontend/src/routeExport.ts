import type { RouteSourceView } from './TripRoutes';

export type RoutePage = { revision: bigint; source?: RouteSourceView };
export type AllRoutes = { sources: RouteSourceView[]; complete: boolean };
// Bounds one export: 20 pages of up to 10 variants per source.
export const maxExportPages = 20;

export class RevisionChangedError extends Error {
  constructor() { super('Saved routes changed during export'); }
}

// Reads every saved page of each source. All pages must come from one revision:
// if route building progressed meanwhile, the export fails instead of mixing results.
export async function collectAllRoutes(ids: string[], fetchPage: (plannerId: string, offset: number) => Promise<RoutePage>): Promise<AllRoutes> {
  let revision: bigint | undefined;
  let complete = true;
  const sources: RouteSourceView[] = [];
  for (const id of ids) {
    let merged: RouteSourceView | undefined;
    let offset = 0;
    for (let page = 0; ; page++) {
      if (page >= maxExportPages) { complete = false; break; }
      const result = await fetchPage(id, offset);
      if (revision === undefined) revision = result.revision;
      else if (result.revision !== revision) throw new RevisionChangedError();
      const source = result.source;
      if (!source) break;
      merged = merged ? { ...merged, routes: [...merged.routes, ...source.routes], hasMore: source.hasMore } : source;
      if (!source.hasMore) break;
      // Advance by what was returned, not by the requested page size.
      if (source.routes.length === 0) { complete = false; break; }
      offset += source.routes.length;
    }
    if (merged) sources.push(merged);
  }
  return { sources, complete };
}
