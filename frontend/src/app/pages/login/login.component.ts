import { Component, computed } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { AuthService } from '../../core/auth.service';
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
export class LoginComponent {
  email = 'admin@revealdq.local';
  password = 'Admin@123';
  displayName = '';
  mode: 'login' | 'register' = 'login';
  error = '';
  loading = false;

  loggedIn = computed(() => !!this.auth.user());

  constructor(
    public auth: AuthService,
    private router: Router,
    public theme: ThemeService,
  ) {}

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
