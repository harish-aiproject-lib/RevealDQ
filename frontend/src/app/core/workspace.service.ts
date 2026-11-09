import { Injectable, signal } from '@angular/core';
import { Observable, of, shareReplay, tap } from 'rxjs';
import { ApiService } from './api.service';
import { ProductModule, productFor } from './products';

export interface Workspace {
  workspaceId: number;
  workspaceName: string;
  customerName: string;
  description: string;
  createdAt: string;
  role?: string;
  productCode: string;
  productName: string;
  productStatus?: string;
}

/**
 * Small cache in front of `GET /workspaces/:id` so the shell, the product
 * guard and module pages can all agree on which product a project belongs to
 * without hitting the API once per consumer.
 */
@Injectable({ providedIn: 'root' })
export class WorkspaceService {
  private cache = new Map<number, Observable<Workspace>>();
  private snapshot = new Map<number, Workspace>();

  /** Currently opened workspace (set by the shell). */
  readonly current = signal<Workspace | null>(null);

  constructor(private api: ApiService) {}

  get(id: number, force = false): Observable<Workspace> {
    if (!force) {
      const hit = this.snapshot.get(id);
      if (hit) return of(hit);
      const pending = this.cache.get(id);
      if (pending) return pending;
    }
    const req = this.api.getWorkspace(id).pipe(
      tap((ws: Workspace) => this.snapshot.set(id, ws)),
      shareReplay(1),
    );
    this.cache.set(id, req);
    return req;
  }

  invalidate(id?: number): void {
    if (id == null) {
      this.cache.clear();
      this.snapshot.clear();
      return;
    }
    this.cache.delete(id);
    this.snapshot.delete(id);
  }

  productOf(ws: Workspace | null | undefined): ProductModule {
    return productFor(ws?.productCode);
  }
}
