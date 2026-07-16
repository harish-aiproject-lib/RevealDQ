import { Injectable, signal, effect } from '@angular/core';

export type ThemeMode = 'light' | 'dark';

@Injectable({ providedIn: 'root' })
export class ThemeService {
  readonly mode = signal<ThemeMode>(this.read());

  constructor() {
    effect(() => {
      const m = this.mode();
      document.documentElement.setAttribute('data-theme', m);
      localStorage.setItem('rdq_theme', m);
    });
  }

  private read(): ThemeMode {
    const v = localStorage.getItem('rdq_theme');
    return v === 'dark' ? 'dark' : 'light';
  }

  toggle(): void {
    this.mode.update((m) => (m === 'light' ? 'dark' : 'light'));
  }

  set(mode: ThemeMode): void {
    this.mode.set(mode);
  }
}
