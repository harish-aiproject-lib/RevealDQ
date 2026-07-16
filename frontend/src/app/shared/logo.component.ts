import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-logo',
  standalone: true,
  template: `
    <div class="logo" [class.compact]="compact" [class.light]="onDark" aria-label="Reveal DQ">
      <svg viewBox="0 0 64 64" class="mark" aria-hidden="true">
        <defs>
          <linearGradient id="rdqPulse" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stop-color="#22d3ee" />
            <stop offset="60%" stop-color="#0ea5e9" />
            <stop offset="100%" stop-color="#6366f1" />
          </linearGradient>
        </defs>
        <!-- reveal lens: aperture arcs -->
        <path
          d="M32 6 A26 26 0 1 1 9.5 19"
          fill="none"
          stroke="url(#rdqPulse)"
          stroke-width="5.5"
          stroke-linecap="round"
        />
        <path
          d="M32 17 A15 15 0 1 1 19 24.5"
          fill="none"
          stroke="url(#rdqPulse)"
          stroke-width="4.5"
          stroke-linecap="round"
          opacity="0.55"
        />
        <!-- core -->
        <circle cx="32" cy="32" r="6" fill="url(#rdqPulse)" />
        <!-- orange reveal spark at the aperture opening -->
        <circle cx="12" cy="13" r="4" fill="#f97316" />
      </svg>
      @if (!iconOnly) {
        <div class="wordmark">
          <span class="name">Reveal<span class="dq"> DQ</span></span>
          @if (!compact) {
            <span class="sub">Agentic Data Quality</span>
          }
        </div>
      }
    </div>
  `,
  styles: [
    `
      .logo {
        display: inline-flex;
        align-items: center;
        gap: 0.6rem;
      }
      .mark {
        width: 40px;
        height: 40px;
        flex: 0 0 auto;
      }
      .compact .mark {
        width: 32px;
        height: 32px;
      }
      .wordmark {
        display: grid;
        line-height: 1.1;
      }
      .name {
        font-family: var(--font-display);
        font-size: 1.18rem;
        font-weight: 600;
        letter-spacing: -0.02em;
        color: var(--ink);
      }
      .compact .name {
        font-size: 1rem;
      }
      .light .name {
        color: #fff;
      }
      .dq {
        background: var(--grad-hero);
        -webkit-background-clip: text;
        background-clip: text;
        color: transparent;
        font-weight: 700;
      }
      .sub {
        font-size: 0.64rem;
        letter-spacing: 0.18em;
        text-transform: uppercase;
        color: var(--muted);
        font-weight: 500;
      }
    `,
  ],
})
export class LogoComponent {
  @Input() compact = false;
  @Input() iconOnly = false;
  @Input() onDark = false;
}
