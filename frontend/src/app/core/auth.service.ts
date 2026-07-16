import { Injectable, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { tap } from 'rxjs/operators';
import { environment } from '../../environments/environment';

export interface LoginResponse {
  token: string;
  userId: number;
  email: string;
  displayName: string;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  readonly user = signal<LoginResponse | null>(this.readSession());

  constructor(private http: HttpClient) {}

  private readSession(): LoginResponse | null {
    const token = localStorage.getItem('rdq_token');
    const raw = localStorage.getItem('rdq_user');
    if (!token || !raw) return null;
    try {
      return { ...JSON.parse(raw), token };
    } catch {
      return null;
    }
  }

  isLoggedIn(): boolean {
    return !!localStorage.getItem('rdq_token');
  }

  login(email: string, password: string) {
    return this.http
      .post<LoginResponse>(`${environment.apiUrl}/auth/login`, { email, password })
      .pipe(
        tap((res) => {
          localStorage.setItem('rdq_token', res.token);
          localStorage.setItem(
            'rdq_user',
            JSON.stringify({
              userId: res.userId,
              email: res.email,
              displayName: res.displayName,
            }),
          );
          this.user.set(res);
        }),
      );
  }

  register(email: string, password: string, displayName: string) {
    return this.http.post(`${environment.apiUrl}/auth/register`, {
      email,
      password,
      displayName,
    });
  }

  logout(): void {
    localStorage.removeItem('rdq_token');
    localStorage.removeItem('rdq_user');
    this.user.set(null);
  }
}
