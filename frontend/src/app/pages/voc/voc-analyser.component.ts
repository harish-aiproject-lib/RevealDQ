import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';
import { mdToHtml } from './voc-ui';

interface Turn {
  role: 'user' | 'agent';
  text: string;
  html?: string;
  highlights?: string[];
  engine?: string;
  model?: string;
}

/**
 * Reveal VoC — VoC Analyser Agent.
 *
 * Conversational analyst over the project's stored, classified LinkedIn
 * signals. The API packages stats + a signal sample + actions and the Python
 * agent answers with evidence and next steps.
 */
@Component({
  selector: 'app-voc-analyser',
  standalone: true,
  imports: [FormsModule, IconComponent],
  templateUrl: './voc-analyser.component.html',
  styleUrls: ['./voc-shared.scss', './voc-analyser.component.scss'],
})
export class VocAnalyserComponent implements OnInit {
  wsId = 0;
  companies = signal<any[]>([]);
  turns = signal<Turn[]>([]);
  busy = signal(false);
  error = '';
  question = '';
  companyId = 0;
  model = '';

  readonly models = [
    { id: '', label: 'Default model' },
    { id: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4' },
    { id: 'claude-3-5-haiku-20241022', label: 'Claude Haiku 3.5 (fast)' },
    { id: 'claude-opus-4-20250514', label: 'Claude Opus 4' },
  ];

  readonly starters = [
    'What are customers most unhappy about this month?',
    'Which company has the biggest churn risk and why?',
    'Summarise the positive voices we should amplify.',
    'Which open actions should the team prioritise first?',
    'How is sentiment trending over the last three weeks?',
  ];

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.api.vocCompanies(this.wsId).subscribe({ next: (c) => this.companies.set(c) });
  }

  ask(q?: string): void {
    const question = (q ?? this.question).trim();
    if (!question || this.busy()) return;
    this.error = '';
    this.question = '';
    this.turns.update((t) => [...t, { role: 'user', text: question }]);
    this.busy.set(true);
    this.api.vocAsk(this.wsId, question, this.companyId || undefined, this.model || undefined).subscribe({
      next: (r) => {
        this.turns.update((t) => [
          ...t,
          {
            role: 'agent',
            text: r.answer || '',
            html: mdToHtml(r.answer || ''),
            highlights: r.highlights || [],
            engine: r.engine,
            model: r.modelUsed,
          },
        ]);
        this.busy.set(false);
      },
      error: (e) => {
        this.error = e?.error?.error || 'The VoC agent did not answer';
        this.busy.set(false);
      },
    });
  }

  clear(): void {
    this.turns.set([]);
  }
}
