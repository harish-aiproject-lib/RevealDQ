import { Component, Input } from '@angular/core';

/**
 * Reality Reveal platform mark.
 *
 * Renders the platform wordmark by default. Pass `product` ("DQ" | "VoC" …) to
 * render the product-module lockup, e.g. "Reveal DQ", used inside a project.
 */
@Component({
  selector: 'app-logo',
  standalone: true,
  template: `
    <div
      class="logo"
      [class.compact]="compact"
      [class.light]="onDark"
      [class.violet]="accent === 'violet'"
      [attr.aria-label]="product ? 'Reveal ' + product : 'Reality Reveal'"
    >
      <svg viewBox="0 0 64 64" class="mark" aria-hidden="true">
        <defs>
          <linearGradient id="rrPulse" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stop-color="#22d3ee" />
            <stop offset="60%" stop-color="#0ea5e9" />
            <stop offset="100%" stop-color="#6366f1" />
          </linearGradient>
          <linearGradient id="rrPulseViolet" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stop-color="#a78bfa" />
            <stop offset="60%" stop-color="#8b5cf6" />
            <stop offset="100%" stop-color="#ec4899" />
          </linearGradient>
        </defs>
        <!-- reveal lens: aperture arcs -->
        <path
          d="M32 6 A26 26 0 1 1 9.5 19"
          fill="none"
          [attr.stroke]="accent === 'violet' ? 'url(#rrPulseViolet)' : 'url(#rrPulse)'"
          stroke-width="5.5"
          stroke-linecap="round"
        />
        <path
          d="M32 17 A15 15 0 1 1 19 24.5"
          fill="none"
          [attr.stroke]="accent === 'violet' ? 'url(#rrPulseViolet)' : 'url(#rrPulse)'"
          stroke-width="4.5"
          stroke-linecap="round"
          opacity="0.55"
        />
        <!-- core -->
        <circle cx="32" cy="32" r="6" [attr.fill]="accent === 'violet' ? 'url(#rrPulseViolet)' : 'url(#rrPulse)'" />
        <!-- orange reveal spark at the aperture opening -->
        <circle cx="12" cy="13" r="4" fill="#f97316" />
      </svg>
      @if (!iconOnly) {
        <div class="wordmark">
          @if (product) {
            <span class="name">Reveal<span class="product"> {{ product }}</span></span>
            @if (!compact) {
              <span class="sub">A Reality Reveal product</span>
            }
          } @else {
            <span class="name">Reality<span class="product"> Reveal</span></span>
            @if (!compact) {
              <span class="sub">Multi-product reveal platform</span>
            }
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
        white-space: nowrap;
      }
      .compact .name {
        font-size: 1rem;
      }
      .light .name {
        color: #fff;
      }
      .product {
        background: var(--grad-hero);
        -webkit-background-clip: text;
        background-clip: text;
        color: transparent;
        font-weight: 700;
      }
      .violet .product {
        background: var(--grad-violet);
        -webkit-background-clip: text;
        background-clip: text;
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
  /** Product short name ("DQ", "VoC"). Omit for the platform wordmark. */
  @Input() product: string | null = null;
  @Input() accent: 'cyan' | 'violet' = 'cyan';
}
