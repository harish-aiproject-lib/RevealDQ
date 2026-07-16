import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { AuthService } from '../../core/auth.service';
import { ThemeService } from '../../core/theme.service';
import { LogoComponent } from '../../shared/logo.component';
import { IconComponent } from '../../shared/icon.component';

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

  constructor(
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
    this.reload();
  }

  reload(): void {
    this.api.listWorkspaces().subscribe({
      next: (list) => this.workspaces.set(list || []),
      error: (err) => (this.error = err?.error?.error || 'Failed to load workspaces'),
    });
  }

  create(): void {
    if (!this.workspaceName.trim()) return;
    this.api
      .createWorkspace({
        workspaceName: this.workspaceName,
        customerName: this.customerName || this.workspaceName,
        description: this.description,
      })
      .subscribe({
        next: (res) => {
          this.workspaceName = '';
          this.customerName = '';
          this.description = '';
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
