import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';
import { DateRange, RANGE_PRESETS, lastDays, rangeDays, relTime, syncSummary, ymd } from './voc-ui';

/**
 * Reveal VoC — Companies.
 *
 * The user registers the companies whose LinkedIn voice they want to capture:
 * official handle (vanity name or page URL) plus the hashtags customers use.
 *
 * Extraction is a Playwright scrape of LinkedIn's public pages (no
 * credentials, no API): the company feed, third-party posts naming the
 * company, and the comments under them — inside the chosen date window
 * (default: the last 5 days). Each company can be scraped on demand; the
 * scheduler refreshes all of them incrementally.
 */
@Component({
  selector: 'app-voc-companies',
  standalone: true,
  imports: [FormsModule, RouterLink, IconComponent],
  templateUrl: './voc-companies.component.html',
  styleUrls: ['./voc-shared.scss', './voc-companies.component.scss'],
})
export class VocCompaniesComponent implements OnInit {
  wsId = 0;
  companies = signal<any[]>([]);
  sources = signal<any[]>([]);
  loading = signal(false);
  syncing = signal<number | 'all' | null>(null);
  message = '';
  notes: string[] = [];
  error = '';

  /** Scrape window applied to "Scrape now" / "Scrape all" and to the first sync of a new company. */
  range: DateRange = lastDays(5);
  readonly presets = RANGE_PRESETS;
  readonly today = ymd(new Date());

  form = { companyName: '', linkedinHandle: '', hashtags: '', syncNow: true };
  readonly relTime = relTime;

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.api.vocCompanies(this.wsId).subscribe({
      next: (list) => {
        this.companies.set(list);
        this.loading.set(false);
      },
      error: (e) => {
        this.error = e?.error?.error || 'Could not load companies';
        this.loading.set(false);
      },
    });
    this.api.vocSources(this.wsId).subscribe({ next: (r) => this.sources.set(r.sources || []) });
  }

  /** Name of the extraction path that will be used (Playwright by default). */
  extractionLabel(): string {
    const active = this.sources().filter((s) => s.isActive && s.provider !== 'IMPORT');
    if (!active.length || active.some((s) => s.provider === 'PLAYWRIGHT')) return 'LinkedIn public pages · Playwright scraper (no credentials)';
    const s = active[0];
    return s.label || s.provider;
  }

  // ---- date range ------------------------------------------------------

  preset(days: number): void {
    this.range = lastDays(days);
  }

  isPreset(days: number): boolean {
    const r = lastDays(days);
    return r.from === this.range.from && r.to === this.range.to;
  }

  rangeDays(): number {
    return rangeDays(this.range);
  }

  rangeError(): string {
    if (!this.range.from || !this.range.to) return 'Pick both dates.';
    if (this.rangeDays() === 0) return '"From" must be on or before "To".';
    if (this.range.to > this.today) return '"To" cannot be in the future.';
    if (this.rangeDays() > 90) return 'Keep the window to 90 days or less — LinkedIn only exposes recent public posts.';
    return '';
  }

  // ---- companies -------------------------------------------------------

  handlePreview(): string {
    const h = this.form.linkedinHandle.trim();
    if (!h) return '';
    const m = h.toLowerCase().match(/linkedin\.com\/(?:company|school|showcase)\/([^/?#]+)/);
    return (m ? m[1] : h.replace(/^@/, '')).toLowerCase();
  }

  create(): void {
    this.message = '';
    this.notes = [];
    this.error = '';
    if (!this.form.companyName.trim() || !this.form.linkedinHandle.trim()) {
      this.error = 'Company name and LinkedIn handle are required.';
      return;
    }
    if (this.form.syncNow && this.rangeError()) {
      this.error = this.rangeError();
      return;
    }
    const hashtags = this.form.hashtags
      .split(/[,\s]+/)
      .map((t) => t.trim().replace(/^#/, ''))
      .filter(Boolean);
    const syncNow = this.form.syncNow;
    this.loading.set(true);
    this.api
      .vocCreateCompany(this.wsId, {
        companyName: this.form.companyName.trim(),
        linkedinHandle: this.form.linkedinHandle.trim(),
        hashtags,
        // the scrape is triggered separately below so the selected window applies
        syncNow: false,
      })
      .subscribe({
        next: (r) => {
          const c = r.company;
          this.message = `Added ${c.companyName} (@${c.linkedinHandle}).`;
          this.form = { companyName: '', linkedinHandle: '', hashtags: '', syncNow: true };
          this.load();
          if (syncNow) this.sync(c.companyId, `Added ${c.companyName}. `);
        },
        error: (e) => {
          this.error = e?.error?.error || 'Could not add company';
          this.loading.set(false);
        },
      });
  }

  /** Scrape one company (or all) inside the selected window, then analyse. */
  sync(companyId?: number, prefix = ''): void {
    const rangeErr = this.rangeError();
    if (rangeErr) {
      this.error = rangeErr;
      return;
    }
    this.message = '';
    this.notes = [];
    this.error = '';
    this.syncing.set(companyId ?? 'all');
    this.api.vocSync(this.wsId, companyId, this.range).subscribe({
      next: (r) => {
        this.message = prefix + syncSummary(r);
        this.notes = r.notes || [];
        if (r.errors?.length) this.error = r.errors.join(' · ');
        this.syncing.set(null);
        this.load();
      },
      error: (e) => {
        this.error = e?.error?.error || 'Scrape failed';
        this.syncing.set(null);
        this.load();
      },
    });
  }

  toggle(c: any): void {
    this.api.vocUpdateCompany(this.wsId, c.companyId, { isActive: !c.isActive }).subscribe({
      next: () => this.load(),
      error: (e) => (this.error = e?.error?.error || 'Update failed'),
    });
  }

  remove(c: any): void {
    if (!confirm(`Remove ${c.companyName} and all ${c.postCount} stored signals?`)) return;
    this.api.vocDeleteCompany(this.wsId, c.companyId).subscribe({
      next: () => {
        this.message = `Removed ${c.companyName}.`;
        this.load();
      },
      error: (e) => (this.error = e?.error?.error || 'Delete failed'),
    });
  }
}
