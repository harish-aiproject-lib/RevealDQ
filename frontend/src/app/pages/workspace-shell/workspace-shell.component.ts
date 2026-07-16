import { Component, OnInit, computed, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { AuthService } from '../../core/auth.service';
import { ThemeService } from '../../core/theme.service';
import { LogoComponent } from '../../shared/logo.component';
import { IconComponent } from '../../shared/icon.component';

@Component({
  selector: 'app-workspace-shell',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive, LogoComponent, IconComponent],
  templateUrl: './workspace-shell.component.html',
  styleUrl: './workspace-shell.component.scss',
})
export class WorkspaceShellComponent implements OnInit {
  id = 0;
  workspace = signal<any>(null);
  ribbonCollapsed = signal(false);

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
    public auth: AuthService,
    public theme: ThemeService,
    private router: Router,
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

  ngOnInit(): void {
    this.id = Number(this.route.snapshot.paramMap.get('id'));
    this.api.getWorkspace(this.id).subscribe({
      next: (ws) => this.workspace.set(ws),
    });
  }

  logout(): void {
    this.auth.logout();
    this.router.navigateByUrl('/login');
  }
}
