import { Component, OnInit } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { IconComponent } from '../../shared/icon.component';

@Component({
  selector: 'app-stub',
  standalone: true,
  imports: [IconComponent],
  template: `
    <section class="panel stub">
      <span class="badge"><app-icon name="spark" [size]="24" /></span>
      <h2>{{ title }}</h2>
      <p class="muted">{{ message }}</p>
      <span class="tag orange">Coming soon</span>
    </section>
  `,
  styles: [
    `
      .stub {
        padding: 3rem 2rem;
        display: grid;
        justify-items: center;
        gap: 0.7rem;
        text-align: center;
        max-width: 560px;
        margin: 8vh auto 0;
      }
      .badge {
        width: 58px;
        height: 58px;
        border-radius: 18px;
        display: grid;
        place-items: center;
        background: var(--cyan-soft);
        color: var(--cyan-ink);
      }
      h2 {
        margin: 0;
        font-size: 1.25rem;
      }
      p {
        margin: 0;
        line-height: 1.6;
      }
    `,
  ],
})
export class StubComponent implements OnInit {
  title = 'Coming soon';
  message = 'This capability is stubbed for the MVP.';

  constructor(private route: ActivatedRoute) {}

  ngOnInit(): void {
    this.title = this.route.snapshot.data['title'] || this.title;
    this.message = this.route.snapshot.data['message'] || this.message;
  }
}
