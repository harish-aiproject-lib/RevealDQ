import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-icon',
  standalone: true,
  template: `
    <svg
      [attr.width]="size"
      [attr.height]="size"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      @switch (name) {
        @case ('home') {
          <path d="M3 10.5 12 3l9 7.5" />
          <path d="M5 9.5V21h5v-6h4v6h5V9.5" />
        }
        @case ('agent') {
          <rect x="5" y="8" width="14" height="11" rx="3" />
          <path d="M12 8V4M8 4h8" />
          <circle cx="9.5" cy="13" r="1" fill="currentColor" stroke="none" />
          <circle cx="14.5" cy="13" r="1" fill="currentColor" stroke="none" />
          <path d="M9.5 16.5h5" />
        }
        @case ('shield') {
          <path d="M12 3l7 3v5c0 4.5-3 8.2-7 10-4-1.8-7-5.5-7-10V6l7-3z" />
          <path d="M9 12l2 2 4-4" />
        }
        @case ('flow') {
          <circle cx="5" cy="6" r="2.5" />
          <circle cx="19" cy="6" r="2.5" />
          <circle cx="12" cy="18" r="2.5" />
          <path d="M7 7.5 10.5 16M17 7.5 13.5 16" />
        }
        @case ('convert') {
          <path d="M4 8h12l-3-3M20 16H8l3 3" />
        }
        @case ('gauge') {
          <path d="M4 14a8 8 0 1 1 16 0" />
          <path d="M12 14l4-4" />
          <path d="M3 19h18" />
        }
        @case ('pipeline') {
          <rect x="3" y="4" width="18" height="5" rx="2" />
          <rect x="3" y="15" width="18" height="5" rx="2" />
          <path d="M7 9v6M17 9v6" />
        }
        @case ('database') {
          <ellipse cx="12" cy="5.5" rx="7" ry="2.8" />
          <path d="M5 5.5V18.5c0 1.5 3.1 2.8 7 2.8s7-1.3 7-2.8V5.5" />
          <path d="M5 12c0 1.5 3.1 2.8 7 2.8s7-1.3 7-2.8" />
        }
        @case ('settings') {
          <circle cx="12" cy="12" r="3" />
          <path
            d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.2a1.7 1.7 0 0 0-1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.2a1.7 1.7 0 0 0 1.5-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3h.1a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.2a1.7 1.7 0 0 0 1 1.5h.1a1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9v.1a1.7 1.7 0 0 0 1.5 1h.2a2 2 0 1 1 0 4h-.2a1.7 1.7 0 0 0-1.5 1z"
          />
        }
        @case ('grid') {
          <rect x="3" y="3" width="7" height="7" rx="1.5" />
          <rect x="14" y="3" width="7" height="7" rx="1.5" />
          <rect x="3" y="14" width="7" height="7" rx="1.5" />
          <rect x="14" y="14" width="7" height="7" rx="1.5" />
        }
        @case ('sun') {
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
        }
        @case ('moon') {
          <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
        }
        @case ('chart') {
          <path d="M3 3v18h18" />
          <rect x="7" y="12" width="3" height="6" rx="0.8" />
          <rect x="12" y="8" width="3" height="10" rx="0.8" />
          <rect x="17" y="5" width="3" height="13" rx="0.8" />
        }
        @case ('spark') {
          <path d="M12 2l2.2 6.3L21 10l-6.8 1.7L12 18l-2.2-6.3L3 10l6.8-1.7L12 2z" />
        }
        @case ('plus') {
          <path d="M12 5v14M5 12h14" />
        }
        @case ('logout') {
          <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
          <path d="M16 17l5-5-5-5" />
          <path d="M21 12H9" />
        }
        @case ('play') {
          <path d="M6 4l14 8-14 8V4z" />
        }
        @case ('arrow-right') {
          <path d="M5 12h14" />
          <path d="M13 6l6 6-6 6" />
        }
        @case ('check') {
          <path d="M4 12.5l5 5L20 7" />
        }
        @case ('lock') {
          <rect x="4" y="11" width="16" height="9" rx="2" />
          <path d="M8 11V7a4 4 0 0 1 8 0v4" />
        }
        @case ('table') {
          <rect x="3" y="4" width="18" height="16" rx="2" />
          <path d="M3 10h18M9 10v10M15 10v10" />
        }
      }
    </svg>
  `,
  styles: [
    `
      :host {
        display: inline-flex;
        line-height: 0;
      }
    `,
  ],
})
export class IconComponent {
  @Input({ required: true }) name = '';
  @Input() size = 18;
}
