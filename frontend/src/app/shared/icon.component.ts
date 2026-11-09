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
        @case ('mic') {
          <rect x="9" y="3" width="6" height="11" rx="3" />
          <path d="M5 11a7 7 0 0 0 14 0" />
          <path d="M12 18v3M9 21h6" />
        }
        @case ('modules') {
          <path d="M12 3l8 4.5-8 4.5-8-4.5L12 3z" />
          <path d="M4 12l8 4.5 8-4.5" />
          <path d="M4 16.5L12 21l8-4.5" />
        }
        @case ('users') {
          <circle cx="9" cy="8" r="3.2" />
          <path d="M3 19c0-3.2 2.7-5.5 6-5.5s6 2.3 6 5.5" />
          <circle cx="17" cy="9" r="2.4" />
          <path d="M16.5 13.6c2.6.3 4.5 2.2 4.5 4.9" />
        }
        @case ('wave') {
          <path d="M3 12h2l2-5 3 10 3-14 3 12 2-3h3" />
        }
        @case ('building') {
          <rect x="4" y="3" width="16" height="18" rx="2" />
          <path d="M9 7h2M13 7h2M9 11h2M13 11h2M9 15h2M13 15h2M10 21v-3h4v3" />
        }
        @case ('refresh') {
          <path d="M20 12a8 8 0 1 1-2.3-5.7" />
          <path d="M20 4v5h-5" />
        }
        @case ('inbox') {
          <path d="M4 4h16v16H4z" />
          <path d="M4 14h4l2 3h4l2-3h4" />
        }
        @case ('flag') {
          <path d="M5 21V4" />
          <path d="M5 4h12l-2 4 2 4H5" />
        }
        @case ('link') {
          <path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1.2 1.2" />
          <path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1.2-1.2" />
        }
        @case ('globe') {
          <circle cx="12" cy="12" r="9" />
          <path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />
        }
        @case ('trash') {
          <path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13" />
          <path d="M10 11v6M14 11v6" />
        }
        @case ('search') {
          <circle cx="11" cy="11" r="6.5" />
          <path d="M20 20l-4-4" />
        }
        @case ('clock') {
          <circle cx="12" cy="12" r="8.5" />
          <path d="M12 7v5l3.5 2" />
        }
        @case ('smile') {
          <circle cx="12" cy="12" r="8.5" />
          <path d="M8.5 14.5c1 1.3 2.2 2 3.5 2s2.5-.7 3.5-2" />
          <circle cx="9.5" cy="10" r="0.9" fill="currentColor" stroke="none" />
          <circle cx="14.5" cy="10" r="0.9" fill="currentColor" stroke="none" />
        }
        @case ('frown') {
          <circle cx="12" cy="12" r="8.5" />
          <path d="M8.5 16.5c1-1.3 2.2-2 3.5-2s2.5.7 3.5 2" />
          <circle cx="9.5" cy="10" r="0.9" fill="currentColor" stroke="none" />
          <circle cx="14.5" cy="10" r="0.9" fill="currentColor" stroke="none" />
        }
        @case ('megaphone') {
          <path d="M4 10v4a1 1 0 0 0 1 1h2l4 3.5V5.5L7 9H5a1 1 0 0 0-1 1z" />
          <path d="M15 9.5a3 3 0 0 1 0 5M17.5 7a6.5 6.5 0 0 1 0 10" />
        }
        @case ('upload') {
          <path d="M12 16V5M7.5 9.5 12 5l4.5 4.5" />
          <path d="M4 17v2a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-2" />
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
