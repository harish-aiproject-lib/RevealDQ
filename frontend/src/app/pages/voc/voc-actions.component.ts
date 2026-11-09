import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';
import { CATEGORIES, relTime, titleCase } from './voc-ui';

type Status = 'OPEN' | 'IN_PROGRESS' | 'DONE' | 'DISMISSED';

/**
 * Reveal VoC — Actions.
 *
 * Actions are created by the agent from negative / urgent signals on every
 * sync (de-duplicated by root cause) or added manually. A simple board tracks
 * them from open to done.
 */
@Component({
  selector: 'app-voc-actions',
  standalone: true,
  imports: [FormsModule, RouterLink, IconComponent],
  templateUrl: './voc-actions.component.html',
  styleUrls: ['./voc-shared.scss', './voc-actions.component.scss'],
})
export class VocActionsComponent implements OnInit {
  wsId = 0;
  actions = signal<any[]>([]);
  companies = signal<any[]>([]);
  loading = signal(false);
  error = '';
  message = '';
  showForm = false;

  readonly columns: { status: Status; label: string }[] = [
    { status: 'OPEN', label: 'Open' },
    { status: 'IN_PROGRESS', label: 'In progress' },
    { status: 'DONE', label: 'Done' },
    { status: 'DISMISSED', label: 'Dismissed' },
  ];
  readonly categories = CATEGORIES;
  readonly titleCase = titleCase;
  readonly relTime = relTime;

  form = { companyId: null as number | null, title: '', description: '', category: 'OTHER', priority: 'MEDIUM' };

  byStatus = computed(() => {
    const m: Record<string, any[]> = { OPEN: [], IN_PROGRESS: [], DONE: [], DISMISSED: [] };
    for (const a of this.actions()) (m[a.status] ||= []).push(a);
    return m;
  });

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.load();
    this.api.vocCompanies(this.wsId).subscribe({ next: (c) => this.companies.set(c) });
  }

  load(): void {
    this.loading.set(true);
    this.api.vocActions(this.wsId).subscribe({
      next: (list) => {
        this.actions.set(list);
        this.loading.set(false);
      },
      error: (e) => {
        this.error = e?.error?.error || 'Could not load actions';
        this.loading.set(false);
      },
    });
  }

  move(a: any, status: Status): void {
    this.api.vocUpdateAction(this.wsId, a.actionId, { status }).subscribe({
      next: () => this.load(),
      error: (e) => (this.error = e?.error?.error || 'Update failed'),
    });
  }

  setPriority(a: any, priority: string): void {
    this.api.vocUpdateAction(this.wsId, a.actionId, { priority }).subscribe({ next: () => this.load() });
  }

  setOwner(a: any, owner: string): void {
    if (owner === (a.owner || '')) return;
    this.api.vocUpdateAction(this.wsId, a.actionId, { owner }).subscribe({ next: () => this.load() });
  }

  create(): void {
    if (!this.form.title.trim()) {
      this.error = 'Title is required';
      return;
    }
    this.error = '';
    this.api.vocCreateAction(this.wsId, { ...this.form, title: this.form.title.trim() }).subscribe({
      next: () => {
        this.message = 'Action created.';
        this.form = { companyId: null, title: '', description: '', category: 'OTHER', priority: 'MEDIUM' };
        this.showForm = false;
        this.load();
      },
      error: (e) => (this.error = e?.error?.error || 'Could not create action'),
    });
  }

  next(status: Status): Status | null {
    return status === 'OPEN' ? 'IN_PROGRESS' : status === 'IN_PROGRESS' ? 'DONE' : null;
  }
}
