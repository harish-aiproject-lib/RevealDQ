import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { AuthService } from '../../core/auth.service';
import {
  DEFAULT_PRODUCT,
  PRODUCT_LIST,
  ProductCatalogEntry,
  ProductCode,
  ProductModule,
  ProductStatus,
  productFor,
} from '../../core/products';
import { ThemeService } from '../../core/theme.service';
import { WorkspaceService } from '../../core/workspace.service';
import { LogoComponent } from '../../shared/logo.component';
import { IconComponent } from '../../shared/icon.component';

type Filter = 'ALL' | ProductCode;

@Component({
  selector: 'app-workspaces',
  standalone: true,
  imports: [FormsModule, RouterLink, LogoComponent, IconComponent],
  templateUrl: './workspaces.component.html',
  styleUrl: './workspaces.component.scss',
})
export class WorkspacesComponent implements OnInit {
  workspaces = signal<any[]>([]);
  workspaceName = '';
  customerName = '';
  description = '';
  error = '';
  ribbonCollapsed = signal(false);

  /** Product-module picker. */
  readonly products: ProductModule[] = PRODUCT_LIST;
  selectedProduct = signal<ProductCode>(DEFAULT_PRODUCT);
  catalog = signal<Record<string, ProductStatus>>({ DQ: 'ACTIVE', VOC: 'PREVIEW' });

  filter = signal<Filter>('ALL');

  constructor(
    private api: ApiService,
    public auth: AuthService,
    public theme: ThemeService,
    private router: Router,
    private wsCache: WorkspaceService,
  ) {}

  initials = computed(() => {
    const u = this.auth.user();
    const name = u?.displayName || u?.email || '?';
    return name
      .split(/[\s@._-]+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((p) => p[0]?.toUpperCase() || '')
      .join('');
  });

  selected = computed(() => productFor(this.selectedProduct()));

  filtered = computed(() => {
    const f = this.filter();
    const list = this.workspaces();
    return f === 'ALL' ? list : list.filter((w) => String(w.productCode || 'DQ').toUpperCase() === f);
  });

  /** Explorer groups: one per product that has at least one project. */
  grouped = computed(() =>
    this.products
      .map((p) => ({
        product: p,
        items: this.workspaces().filter((w) => String(w.productCode || 'DQ').toUpperCase() === p.code),
      }))
      .filter((g) => g.items.length > 0),
  );

  countFor(code: Filter): number {
    if (code === 'ALL') return this.workspaces().length;
    return this.workspaces().filter((w) => String(w.productCode || 'DQ').toUpperCase() === code).length;
  }

  ngOnInit(): void {
    this.reload();
    this.api.listProducts().subscribe({
      next: (list: ProductCatalogEntry[]) => {
        const map: Record<string, ProductStatus> = { ...this.catalog() };
        for (const p of list || []) map[String(p.productCode).toUpperCase()] = p.status;
        this.catalog.set(map);
      },
      error: () => {},
    });
  }

  reload(): void {
    this.api.listWorkspaces().subscribe({
      next: (list) => this.workspaces.set(list || []),
      error: (err) => (this.error = err?.error?.error || 'Failed to load projects'),
    });
  }

  productOf(ws: any): ProductModule {
    return productFor(ws?.productCode);
  }

  /** Exposed for the template's empty-state copy. */
  productFor(code: string): ProductModule {
    return productFor(code);
  }

  statusOf(code: string): ProductStatus {
    return this.catalog()[code] || 'ACTIVE';
  }

  statusLabel(code: string): string {
    switch (this.statusOf(code)) {
      case 'PREVIEW':
        return 'Preview';
      case 'COMING_SOON':
        return 'Coming soon';
      default:
        return 'Live';
    }
  }

  canCreate(code: string): boolean {
    return this.statusOf(code) !== 'COMING_SOON';
  }

  pick(p: ProductModule): void {
    if (!this.canCreate(p.code)) return;
    this.selectedProduct.set(p.code);
    this.error = '';
  }

  create(): void {
    if (!this.workspaceName.trim()) return;
    const product = this.selected();
    this.api
      .createWorkspace({
        workspaceName: this.workspaceName,
        customerName: this.customerName || this.workspaceName,
        description: this.description,
        productCode: product.code,
      })
      .subscribe({
        next: (res) => {
          this.workspaceName = '';
          this.customerName = '';
          this.description = '';
          this.wsCache.invalidate();
          this.router.navigate(['/workspaces', res.workspaceId]);
        },
        error: (err) => (this.error = err?.error?.error || 'Create failed'),
      });
  }

  logout(): void {
    this.auth.logout();
    this.router.navigateByUrl('/login');
  }
}
