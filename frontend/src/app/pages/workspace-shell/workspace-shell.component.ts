import { Component, OnInit, computed, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from '../../core/auth.service';
import { ProductModule, productFor } from '../../core/products';
import { ThemeService } from '../../core/theme.service';
import { Workspace, WorkspaceService } from '../../core/workspace.service';
import { LogoComponent } from '../../shared/logo.component';
import { IconComponent } from '../../shared/icon.component';

/**
 * Workspace (project) shell for the Reality Reveal platform.
 *
 * The left ribbon and banner are rendered from the product module the project
 * belongs to (see core/products.ts) so a Reveal DQ project shows DQ agents and
 * pipelines while a Reveal VoC project shows the Voice of Customer surfaces.
 */
@Component({
  selector: 'app-workspace-shell',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive, LogoComponent, IconComponent],
  templateUrl: './workspace-shell.component.html',
  styleUrl: './workspace-shell.component.scss',
})
export class WorkspaceShellComponent implements OnInit {
  id = 0;
  workspace = signal<Workspace | null>(null);
  ribbonCollapsed = signal(false);

  constructor(
    private route: ActivatedRoute,
    private workspaces: WorkspaceService,
    public auth: AuthService,
    public theme: ThemeService,
    private router: Router,
  ) {}

  /** Product module driving the navigation. Defaults to DQ until loaded. */
  product = computed<ProductModule>(() => productFor(this.workspace()?.productCode));

  productStatus = computed(() => this.workspace()?.productStatus || 'ACTIVE');

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

  ngOnInit(): void {
    this.id = Number(this.route.snapshot.paramMap.get('id'));
    this.workspaces.get(this.id).subscribe({
      next: (ws) => {
        this.workspace.set(ws);
        this.workspaces.current.set(ws);
      },
      error: () => this.router.navigateByUrl('/workspaces'),
    });
  }

  logout(): void {
    this.auth.logout();
    this.router.navigateByUrl('/login');
  }
}
