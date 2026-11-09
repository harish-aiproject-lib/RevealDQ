import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { AuthService } from '../../core/auth.service';
import { PRODUCT_LIST, ProductCatalogEntry, ProductModule, ProductStatus } from '../../core/products';
import { ThemeService } from '../../core/theme.service';
import { LogoComponent } from '../../shared/logo.component';
import { IconComponent } from '../../shared/icon.component';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [FormsModule, LogoComponent, IconComponent],
  templateUrl: './login.component.html',
  styleUrl: './login.component.scss',
})
export class LoginComponent implements OnInit {
  email = '';
  password = '';
  displayName = '';
  mode: 'login' | 'register' = 'login';
  error = '';
  loading = false;

  /** Presentation registry (static) — the API catalog only adds availability status. */
  readonly products: ProductModule[] = PRODUCT_LIST;
  private catalog = signal<Record<string, ProductStatus>>({ DQ: 'ACTIVE', VOC: 'PREVIEW' });

  loggedIn = computed(() => !!this.auth.user());

  constructor(
    public auth: AuthService,
    private api: ApiService,
    private router: Router,
    public theme: ThemeService,
  ) {}

  ngOnInit(): void {
    // The catalog endpoint is authenticated; only refresh statuses when signed in.
    if (this.loggedIn()) {
      this.api.listProducts().subscribe({
        next: (list: ProductCatalogEntry[]) => {
          const map: Record<string, ProductStatus> = { ...this.catalog() };
          for (const p of list || []) map[String(p.productCode).toUpperCase()] = p.status;
          this.catalog.set(map);
        },
        error: () => {},
      });
    }
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

  openApp(): void {
    this.router.navigateByUrl('/workspaces');
  }

  signOut(): void {
    this.auth.logout();
  }

  toggleMode(): void {
    this.mode = this.mode === 'login' ? 'register' : 'login';
    this.error = '';
  }

  submit(): void {
    this.error = '';
    this.loading = true;
    if (this.mode === 'login') {
      this.auth.login(this.email, this.password).subscribe({
        next: () => {
          this.loading = false;
          this.router.navigateByUrl('/workspaces');
        },
        error: (err) => {
          this.loading = false;
          this.error = err?.error?.error || 'Login failed';
        },
      });
    } else {
      this.auth.register(this.email, this.password, this.displayName || this.email).subscribe({
        next: () => {
          this.mode = 'login';
          this.loading = false;
        },
        error: (err) => {
          this.loading = false;
          this.error = err?.error?.error || 'Register failed';
        },
      });
    }
  }
}
