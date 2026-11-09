import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';
import { CATEGORIES, SENTIMENTS, relTime, sentimentMeta, titleCase } from './voc-ui';

/**
 * Reveal VoC — Signals.
 *
 * Every LinkedIn post / comment stored in VOC_DB for the project, with the
 * agent's classification (sentiment, category, themes, urgency, suggested
 * action). Filters are reflected in the URL so links from other pages work.
 */
@Component({
  selector: 'app-voc-signals',
  standalone: true,
  imports: [FormsModule, IconComponent],
  templateUrl: './voc-signals.component.html',
  styleUrls: ['./voc-shared.scss', './voc-signals.component.scss'],
})
export class VocSignalsComponent implements OnInit {
  wsId = 0;
  companies = signal<any[]>([]);
  items = signal<any[]>([]);
  total = signal(0);
  loading = signal(false);
  error = '';
  message = '';

  filter = { companyId: 0, sentiment: '', category: '', kind: '', q: '' };
  page = 0;
  readonly pageSize = 40;

  readonly sentiments = SENTIMENTS;
  readonly categories = CATEGORIES;
  readonly meta = sentimentMeta;
  readonly relTime = relTime;
  readonly titleCase = titleCase;
  expanded = signal<Set<number>>(new Set());

  constructor(
    private route: ActivatedRoute,
    private router: Router,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.route.queryParamMap.subscribe((q) => {
      this.filter.companyId = Number(q.get('companyId') || 0);
      this.filter.sentiment = q.get('sentiment') || '';
      this.filter.category = q.get('category') || '';
      this.filter.kind = q.get('kind') || '';
      this.filter.q = q.get('q') || '';
      this.page = Number(q.get('page') || 0);
      this.load();
    });
    this.api.vocCompanies(this.wsId).subscribe({ next: (c) => this.companies.set(c) });
  }

  apply(resetPage = true): void {
    if (resetPage) this.page = 0;
    const f = this.filter;
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: {
        companyId: f.companyId || null,
        sentiment: f.sentiment || null,
        category: f.category || null,
        kind: f.kind || null,
        q: f.q || null,
        page: this.page || null,
      },
    });
  }

  clear(): void {
    this.filter = { companyId: 0, sentiment: '', category: '', kind: '', q: '' };
    this.apply();
  }

  load(): void {
    this.loading.set(true);
    this.error = '';
    this.api
      .vocPosts(this.wsId, { ...this.filter, limit: this.pageSize, offset: this.page * this.pageSize })
      .subscribe({
        next: (r) => {
          this.items.set(r.items);
          this.total.set(r.total);
          this.loading.set(false);
        },
        error: (e) => {
          this.error = e?.error?.error || 'Could not load signals';
          this.loading.set(false);
        },
      });
  }

  pages(): number {
    return Math.max(1, Math.ceil(this.total() / this.pageSize));
  }

  go(delta: number): void {
    this.page = Math.min(this.pages() - 1, Math.max(0, this.page + delta));
    this.apply(false);
  }

  toggle(id: number): void {
    const s = new Set(this.expanded());
    s.has(id) ? s.delete(id) : s.add(id);
    this.expanded.set(s);
  }

  reanalyse(): void {
    this.message = '';
    this.loading.set(true);
    this.api.vocReanalyse(this.wsId, this.filter.companyId || undefined).subscribe({
      next: (r) => {
        this.message = `Re-analysed ${r.itemsAnalysed} signals · ${r.actionsCreated} new actions`;
        if (r.errors?.length) this.error = r.errors.join(' · ');
        this.load();
      },
      error: (e) => {
        this.error = e?.error?.error || 'Re-analysis failed';
        this.loading.set(false);
      },
    });
  }

  scoreLabel(p: any): string {
    if (p.sentimentScore === null || p.sentimentScore === undefined) return '';
    const v = Number(p.sentimentScore);
    return (v > 0 ? '+' : '') + v.toFixed(2);
  }
}
