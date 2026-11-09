import { inject } from '@angular/core';
import { ActivatedRouteSnapshot, CanActivateFn, Router, UrlTree } from '@angular/router';
import { catchError, map, of } from 'rxjs';
import { productFor, routeBelongsTo } from './products';
import { WorkspaceService } from './workspace.service';

function workspaceIdFrom(route: ActivatedRouteSnapshot): number {
  let r: ActivatedRouteSnapshot | null = route;
  while (r) {
    const id = r.paramMap.get('id');
    if (id) return Number(id);
    r = r.parent;
  }
  return 0;
}

/** Child path relative to the workspace shell, e.g. "management/rules". */
function childPath(route: ActivatedRouteSnapshot): string {
  const chain = route.pathFromRoot;
  const shellIdx = chain.findIndex((r) => r.routeConfig?.path === 'workspaces/:id');
  const below = shellIdx >= 0 ? chain.slice(shellIdx + 1) : chain;
  return below
    .map((r) => r.url.map((u) => u.path).join('/'))
    .filter(Boolean)
    .join('/');
}

/**
 * Keeps each project inside its own product module.
 *
 * - Opening `/workspaces/:id` (no child) lands on the module's default route.
 * - Opening a route that belongs to another module (e.g. a DQ screen inside a
 *   Reveal VoC project) redirects to the module home instead of rendering a
 *   half-working page.
 */
export const productGuard: CanActivateFn = (route) => {
  const router = inject(Router);
  const workspaces = inject(WorkspaceService);
  const id = workspaceIdFrom(route);
  if (!id) return true;

  const path = childPath(route);

  return workspaces.get(id).pipe(
    map((ws): boolean | UrlTree => {
      const product = productFor(ws.productCode);
      if (!path) return router.createUrlTree(['/workspaces', id, ...product.defaultRoute.split('/')]);
      if (routeBelongsTo(product, path)) return true;
      return router.createUrlTree(['/workspaces', id, ...product.defaultRoute.split('/')]);
    }),
    catchError(() => of(router.createUrlTree(['/workspaces']))),
  );
};
