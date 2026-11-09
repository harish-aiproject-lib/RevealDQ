import { Component, OnInit, computed, signal } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { PRODUCTS } from '../../core/products';
import { Workspace, WorkspaceService } from '../../core/workspace.service';
import { IconComponent } from '../../shared/icon.component';
import { SENTIMENTS, pct, relTime, sentimentMeta, titleCase } from './voc-ui';

/**
 * Reveal VoC — Overview dashboard.
 *
 * One screen answering: how much are we hearing, how do customers feel, what
 * are they talking about, and what should we do — plus the state of the
 * automation (sources, last run, schedule).
 */
@Component({
  selector: 'app-voc-home',
  standalone: true,
  imports: [RouterLink, IconComponent],
  templateUrl: './voc-home.component.html',
  styleUrl: './voc-home.component.scss',
})
export class VocHomeComponent implements OnInit {
  readonly product = PRODUCTS.VOC;
  wsId = 0;
  workspace = signal<Workspace | null>(null);
  data = signal<any | null>(null);
  loading = signal(true);
  syncing = signal(false);
  error = '';
  message = '';

  readonly sentiments = SENTIMENTS;
  readonly meta = sentimentMeta;
  readonly relTime = relTime;
  readonly titleCase = titleCase;
  readonly pct = pct;

  stats = computed(() => this.data()?.stats || null);
  analysed = computed(() => this.stats()?.analysed || 0);

  /** Sentiment split as ordered bars. */
  split = computed(() => {
    const s = this.stats();
    if (!s) return [];
    const by = s.bySentiment || {};
    return SENTIMENTS.map((m) => ({ ...m, count: by[m.code] || 0, pct: pct(by[m.code] || 0, s.analysed || 0) }));
  });

  /** Customer voice only (excludes broadcast) — the "happiness" ratio. */
  happiness = computed(() => {
    const by = this.stats()?.bySentiment || {};
    const voice = (by['POSITIVE'] || 0) + (by['NEGATIVE'] || 0) + (by['NEUTRAL'] || 0);
    return voice ? Math.round(((by['POSITIVE'] || 0) / voice) * 100) : null;
  });

  categories = computed(() => {
    const by = this.stats()?.byCategory || {};
    const total = Object.values(by).reduce((a: number, b: any) => a + Number(b), 0) as number;
    return Object.entries(by)
      .filter(([k]) => !!k)
      .map(([k, v]) => ({ code: k, count: Number(v), pct: pct(Number(v), total) }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 8);
  });

  timeline = computed(() => {
    const t: any[] = this.stats()?.timeline || [];
    const max = Math.max(1, ...t.map((d) => Number(d.total)));
    return t.map((d) => ({
      ...d,
      posH: (Number(d.positive) / max) * 100,
      negH: (Number(d.negative) / max) * 100,
      othH: (Number(d.other) / max) * 100,
      label: String(d.date).slice(5),
    }));
  });

  hasCompanies = computed(() => (this.data()?.companies || []).length > 0);

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
    private workspaces: WorkspaceService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    if (this.wsId) this.workspaces.get(this.wsId).subscribe({ next: (ws) => this.workspace.set(ws) });
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.api.vocOverview(this.wsId).subscribe({
      next: (d) => {
        this.data.set(d);
        this.loading.set(false);
      },
      error: (e) => {
        this.error = e?.error?.error || 'Could not load the VoC overview';
        this.loading.set(false);
      },
    });
  }

  sync(): void {
    this.syncing.set(true);
    this.message = '';
    this.error = '';
    this.api.vocSync(this.wsId).subscribe({
      next: (r) => {
        this.message = `${r.provider}: ${r.itemsFetched} fetched · ${r.itemsNew} new · ${r.itemsAnalysed} analysed · ${r.actionsCreated} actions`;
        if (r.errors?.length) this.error = r.errors.join(' · ');
        this.syncing.set(false);
        this.load();
      },
      error: (e) => {
        this.error = e?.error?.error || 'Sync failed';
        this.syncing.set(false);
      },
    });
  }

  companyRatio(c: any): number {
    const voice = Number(c.positive) + Number(c.negative) + Number(c.neutral);
    return voice ? Math.round((Number(c.positive) / voice) * 100) : 0;
  }
}
