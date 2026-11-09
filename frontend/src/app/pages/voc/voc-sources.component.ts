import { SlicePipe } from '@angular/common';
import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';
import { DateRange, RANGE_PRESETS, lastDays, rangeDays, relTime, syncSummary, ymd } from './voc-ui';

type Provider = 'PLAYWRIGHT' | 'OFFICIAL_API' | 'GENERIC_JSON' | 'IMPORT' | 'SAMPLE';

/**
 * Reveal VoC — Sources & sync (the automation control panel).
 *
 *  - LinkedIn public pages (Playwright): headless-browser scrape of what
 *    LinkedIn serves logged-out visitors — no credentials, no API. Default.
 *  - Data provider: any HTTP endpoint returning normalised signals (for
 *    hashtag-wide listening LinkedIn's API does not expose).
 *  - Import: paste/upload JSON exported from elsewhere.
 *  - Sample: deterministic generator to exercise the pipeline.
 *  - Schedule: periodic refresh handled by the API's scheduler.
 */
@Component({
  selector: 'app-voc-sources',
  standalone: true,
  imports: [FormsModule, SlicePipe, IconComponent],
  templateUrl: './voc-sources.component.html',
  styleUrls: ['./voc-shared.scss', './voc-sources.component.scss'],
})
export class VocSourcesComponent implements OnInit {
  wsId = 0;
  sources = signal<any[]>([]);
  companies = signal<any[]>([]);
  runs = signal<any[]>([]);
  schedule: any = { enabled: false, cadenceMinutes: 360 };
  oauthReady = signal(false);
  accountEmail = signal('');
  redirectUri = signal('');
  busy = signal(false);
  message = '';
  error = '';

  readonly relTime = relTime;
  readonly providers: { id: Provider; label: string; icon: string; note: string }[] = [
    {
      id: 'PLAYWRIGHT',
      label: 'LinkedIn — public pages (Playwright scraper)',
      icon: 'globe',
      note: 'Headless Chromium visits the pages LinkedIn serves without a login: the company feed, third-party posts naming the company or its hashtags (found through the web index) and the comments under them, inside the date window you choose. No credentials, no LinkedIn API. Volume is limited to what LinkedIn exposes publicly.',
    },
    {
      id: 'GENERIC_JSON',
      label: 'LinkedIn — data provider endpoint',
      icon: 'database',
      note: 'Any HTTP endpoint that returns signals in the Reveal VoC item format (e.g. a third-party LinkedIn listening service or your own crawler). Placeholders: {handle} {hashtags} {company} {since} {until}.',
    },
    {
      id: 'IMPORT',
      label: 'Manual import (JSON)',
      icon: 'upload',
      note: 'Paste an exported JSON array of signals for a company. Useful for one-off analyses.',
    },
    {
      id: 'SAMPLE',
      label: 'Sample generator',
      icon: 'spark',
      note: 'Realistic synthetic LinkedIn signals for demos and testing — exercises the full extraction → analysis → actions loop without touching LinkedIn.',
    },
  ];

  /** Kept for rendering legacy rows; no longer offered in the picker. */
  private readonly legacyMeta = {
    id: 'OFFICIAL_API' as Provider,
    label: 'LinkedIn — official API (OAuth)',
    icon: 'link',
    note: 'Legacy: LinkedIn Community Management API via OAuth.',
  };

  /** Scrape window for “Sync all companies now”. */
  range: DateRange = lastDays(5);
  readonly presets = RANGE_PRESETS;
  readonly today = ymd(new Date());
  notes: string[] = [];

  selected: Provider = 'PLAYWRIGHT';
  form = { label: '', endpoint: '', method: 'GET', headerName: 'Authorization', headerValue: '' };
  importForm = { sourceId: 0, companyId: 0, payload: '' };
  cadenceOptions = [
    { v: 60, l: 'Every hour' },
    { v: 180, l: 'Every 3 hours' },
    { v: 360, l: 'Every 6 hours' },
    { v: 720, l: 'Every 12 hours' },
    { v: 1440, l: 'Daily' },
    { v: 10080, l: 'Weekly' },
  ];

  constructor(
    private route: ActivatedRoute,
    private router: Router,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    const q = this.route.snapshot.queryParamMap;
    if (q.get('linkedin') === 'connected') this.message = 'LinkedIn connected. You can sync companies now.';
    if (q.get('linkedin') === 'error') this.error = 'LinkedIn connection failed: ' + (q.get('message') || 'unknown error');
    if (q.has('linkedin')) this.router.navigate([], { relativeTo: this.route, queryParams: {}, replaceUrl: true });
    this.load();
  }

  load(): void {
    this.api.vocSources(this.wsId).subscribe({
      next: (r) => {
        this.sources.set(r.sources || []);
        this.oauthReady.set(!!r.oauthReady);
        this.accountEmail.set(r.accountEmail || '');
        this.redirectUri.set(r.redirectUri || '');
        const imp = this.sources().find((s) => s.provider === 'IMPORT');
        if (imp && !this.importForm.sourceId) this.importForm.sourceId = imp.sourceId;
      },
      error: (e) => (this.error = e?.error?.error || 'Could not load sources'),
    });
    this.api.vocCompanies(this.wsId).subscribe({
      next: (c) => {
        this.companies.set(c);
        if (c.length && !this.importForm.companyId) this.importForm.companyId = c[0].companyId;
      },
    });
    this.api.vocRuns(this.wsId, 25).subscribe({ next: (r) => this.runs.set(r) });
    this.api.vocSchedule(this.wsId).subscribe({ next: (s) => (this.schedule = s) });
  }

  providerMeta(id: string) {
    if (id === 'OFFICIAL_API') return this.legacyMeta;
    return this.providers.find((p) => p.id === id) || this.providers[0];
  }

  // ---- scrape window -----------------------------------------------------

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
    if (this.rangeDays() > 90) return 'Keep the window to 90 days or less.';
    return '';
  }

  has(provider: Provider): boolean {
    return this.sources().some((s) => s.provider === provider);
  }

  addSource(): void {
    this.error = '';
    this.message = '';
    const config: Record<string, any> = {};
    if (this.selected === 'GENERIC_JSON') {
      if (!this.form.endpoint.trim()) {
        this.error = 'Endpoint URL is required for a data provider source.';
        return;
      }
      Object.assign(config, {
        endpoint: this.form.endpoint.trim(),
        method: this.form.method,
        headerName: this.form.headerValue ? this.form.headerName : '',
        headerValue: this.form.headerValue,
      });
    }
    this.busy.set(true);
    this.api
      .vocCreateSource(this.wsId, { provider: this.selected, label: this.form.label.trim() || undefined, config })
      .subscribe({
        next: (src) => {
          this.busy.set(false);
          this.form = { label: '', endpoint: '', method: 'GET', headerName: 'Authorization', headerValue: '' };
          this.message = `${src.label} added.`;
          this.load();
        },
        error: (e) => {
          this.busy.set(false);
          this.error = e?.error?.error || 'Could not add source';
        },
      });
  }

  connect(src: any): void {
    this.error = '';
    this.api.vocOAuthStart(this.wsId, src.sourceId).subscribe({
      next: (r) => (window.location.href = r.url),
      error: (e) => (this.error = e?.error?.error || 'Could not start LinkedIn OAuth'),
    });
  }

  remove(src: any): void {
    if (!confirm(`Remove source "${src.label}"?`)) return;
    this.api.vocDeleteSource(this.wsId, src.sourceId).subscribe({
      next: () => this.load(),
      error: (e) => (this.error = e?.error?.error || 'Delete failed'),
    });
  }

  syncAll(): void {
    const rangeErr = this.rangeError();
    if (rangeErr) {
      this.error = rangeErr;
      return;
    }
    this.busy.set(true);
    this.message = '';
    this.notes = [];
    this.error = '';
    this.api.vocSync(this.wsId, undefined, this.range).subscribe({
      next: (r) => {
        this.busy.set(false);
        this.message = syncSummary(r) + (r.engine ? ` · analysis ${r.engine}` : '');
        this.notes = r.notes || [];
        if (r.errors?.length) this.error = r.errors.join(' · ');
        this.load();
      },
      error: (e) => {
        this.busy.set(false);
        this.error = e?.error?.error || 'Sync failed';
        this.load();
      },
    });
  }

  saveSchedule(): void {
    const s = this.schedule;
    this.api.vocPutSchedule(this.wsId, { enabled: !!s.enabled, cadenceMinutes: Number(s.cadenceMinutes) }).subscribe({
      next: (r) => {
        this.schedule = r;
        this.message = r.enabled
          ? `Scheduled refresh on — next run ${relTime(r.nextRunAt).replace('ago', 'from now')}.`
          : 'Scheduled refresh off.';
      },
      error: (e) => (this.error = e?.error?.error || 'Could not save schedule'),
    });
  }

  runImport(): void {
    this.error = '';
    this.message = '';
    let payload: any;
    try {
      payload = JSON.parse(this.importForm.payload);
    } catch {
      this.error = 'Payload must be valid JSON (an array of items or {"items":[...]}).';
      return;
    }
    if (!this.importForm.sourceId || !this.importForm.companyId) {
      this.error = 'Add an Import source and pick a company first.';
      return;
    }
    this.busy.set(true);
    this.api.vocImport(this.wsId, this.importForm.sourceId, this.importForm.companyId, payload).subscribe({
      next: (r) => {
        this.busy.set(false);
        this.message = r.message;
        this.importForm.payload = '';
        this.load();
      },
      error: (e) => {
        this.busy.set(false);
        this.error = e?.error?.error || 'Import failed';
      },
    });
  }

  onFile(ev: Event): void {
    const file = (ev.target as HTMLInputElement).files?.[0];
    if (!file) return;
    file.text().then((t) => (this.importForm.payload = t));
  }

  readonly sampleItem = JSON.stringify(
    [
      {
        externalId: 'urn:li:ugcPost:123',
        kind: 'POST',
        authorName: 'Jane Doe',
        authorHeadline: 'Head of Operations',
        authorType: 'PERSON',
        content: 'Great experience with @acme support today! #Acme',
        url: 'https://www.linkedin.com/feed/update/urn:li:ugcPost:123',
        likeCount: 12,
        commentCount: 2,
        publishedAt: '2026-09-15T09:30:00Z',
      },
    ],
    null,
    2,
  );
}
