import { DatePipe } from '@angular/common';
import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';

type StepId = 'analyze' | 'convert' | 'validate' | 'checkin' | 'deploy' | 'job';
type StepState = 'idle' | 'running' | 'done' | 'error';

interface StepDef {
  id: StepId;
  label: string;
  icon: string;
  agent: string;
  hint: string;
  sideEffect: boolean;
}

interface Metric {
  label: string;
  value: string;
  hint: string;
  tone: string;
}

interface Analysis {
  source: { sourceType: string; sourceLabel: string; kind: string };
  summaryMarkdown: string;
  planMarkdown: string;
  metrics: Metric[];
  steps: any[];
}

interface Conversion {
  code: string;
  language: string;
  notes: string[];
  validation: { ok: boolean; issues: string[]; checkedWith: string };
  engine: string;
  modelUsed: string;
}

@Component({
  selector: 'app-conversion',
  standalone: true,
  imports: [FormsModule, IconComponent, DatePipe],
  templateUrl: './conversion.component.html',
  styleUrl: './conversion.component.scss',
})
export class ConversionComponent implements OnInit {
  wsId = 0;

  readonly models = [
    { id: 'claude-3-5-haiku-20241022', label: 'Claude Haiku 3.5 (fast)' },
    { id: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4 (balanced)' },
    { id: 'claude-opus-4-20250514', label: 'Claude Opus 4 (highest quality)' },
    { id: 'lakebridge', label: 'Lakebridge agent (Databricks — coming soon)' },
  ];
  selectedModel = 'claude-sonnet-4-20250514';

  readonly targets = [
    { id: 'databricks_sql', label: 'Databricks SQL' },
    { id: 'pyspark', label: 'PySpark' },
  ];
  target = signal('databricks_sql');

  readonly steps: StepDef[] = [
    { id: 'analyze', label: 'Analyse', icon: 'agent', agent: 'Metadata Extraction', hint: 'Parse & inventory the source', sideEffect: false },
    { id: 'convert', label: 'Convert', icon: 'convert', agent: 'Conversion (LLM)', hint: 'Transpile to Databricks', sideEffect: false },
    { id: 'validate', label: 'Validate', icon: 'shield', agent: 'Validation', hint: 'Syntax & review gate', sideEffect: false },
    { id: 'checkin', label: 'Check-in', icon: 'flow', agent: 'Git', hint: 'Commit artifact to repo', sideEffect: true },
    { id: 'deploy', label: 'Deploy', icon: 'database', agent: 'Databricks', hint: 'Import notebook', sideEffect: true },
    { id: 'job', label: 'Run job', icon: 'pipeline', agent: 'Databricks Jobs', hint: 'Create & trigger job', sideEffect: true },
  ];

  tab = signal<'workflow' | 'audit'>('workflow');

  fileName = signal('');
  fileContent = '';

  analysis = signal<Analysis | null>(null);
  conversion = signal<Conversion | null>(null);

  gitPath = signal('');
  notebookPath = signal('');
  jobId = signal('');
  runId = signal('');

  statuses = signal<Record<StepId, StepState>>({
    analyze: 'idle', convert: 'idle', validate: 'idle', checkin: 'idle', deploy: 'idle', job: 'idle',
  });
  stepMsg = signal<Record<string, string>>({});

  error = signal('');
  busy = signal(false);
  savingAudit = signal(false);

  audit = signal<any[]>([]);
  selectedAudit = signal<any | null>(null);

  planHtml = computed(() => this.mdToHtml(this.analysis()?.planMarkdown || ''));
  summaryHtml = computed(() => this.mdToHtml(this.analysis()?.summaryMarkdown || ''));

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.loadAudit();
  }

  // ---- file handling ----
  onFilePicked(event: Event): void {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      this.fileContent = String(reader.result || '');
      this.fileName.set(file.name);
      this.resetPipeline();
    };
    reader.onerror = () => this.error.set('Could not read the file.');
    reader.readAsText(file);
  }

  private resetPipeline(): void {
    this.error.set('');
    this.analysis.set(null);
    this.conversion.set(null);
    this.gitPath.set('');
    this.notebookPath.set('');
    this.jobId.set('');
    this.runId.set('');
    this.statuses.set({ analyze: 'idle', convert: 'idle', validate: 'idle', checkin: 'idle', deploy: 'idle', job: 'idle' });
    this.stepMsg.set({});
  }

  private setStatus(id: StepId, s: StepState, msg = ''): void {
    this.statuses.update((m) => ({ ...m, [id]: s }));
    if (msg) this.stepMsg.update((m) => ({ ...m, [id]: msg }));
  }

  status(id: StepId): StepState {
    return this.statuses()[id];
  }

  msg(id: StepId): string {
    return this.stepMsg()[id] || '';
  }

  canRun(id: StepId): boolean {
    if (this.busy()) return false;
    switch (id) {
      case 'analyze':
        return !!this.fileContent.trim();
      case 'convert':
        return !!this.analysis();
      case 'validate':
        return !!this.conversion();
      case 'checkin':
        return !!this.conversion();
      case 'deploy':
        return !!this.conversion();
      case 'job':
        return !!this.notebookPath();
    }
  }

  // ---- pipeline steps ----
  runAnalyze(): Promise<boolean> {
    return new Promise((resolve) => {
      if (!this.fileContent.trim()) {
        this.error.set('Upload a .sql, .xml or .dtsx file first.');
        return resolve(false);
      }
      this.error.set('');
      this.busy.set(true);
      this.setStatus('analyze', 'running');
      this.api.conversionAnalyze(this.wsId, { fileName: this.fileName(), content: this.fileContent }).subscribe({
        next: (res) => {
          this.analysis.set({
            source: res.source || {},
            summaryMarkdown: res.summaryMarkdown || '',
            planMarkdown: res.planMarkdown || '',
            metrics: res.metrics || [],
            steps: res.steps || [],
          });
          this.setStatus('analyze', 'done', res.source?.sourceLabel || '');
          this.busy.set(false);
          resolve(true);
        },
        error: (err) => {
          this.setStatus('analyze', 'error', err?.error?.error || 'failed');
          this.error.set(err?.error?.error || 'Analysis failed.');
          this.busy.set(false);
          resolve(false);
        },
      });
    });
  }

  runConvert(): Promise<boolean> {
    return new Promise((resolve) => {
      const a = this.analysis();
      if (!a) return resolve(false);
      this.error.set('');
      this.busy.set(true);
      this.setStatus('convert', 'running');
      this.api
        .conversionRun(this.wsId, {
          fileName: this.fileName(),
          content: this.fileContent,
          sourceType: a.source.sourceType,
          target: this.target(),
          model: this.selectedModel,
        })
        .subscribe({
          next: (res) => {
            this.conversion.set({
              code: res.code || '',
              language: res.language || 'sql',
              notes: res.notes || [],
              validation: res.validation || { ok: true, issues: [], checkedWith: '' },
              engine: res.engine || 'heuristic',
              modelUsed: res.modelUsed || '',
            });
            // reset downstream steps since code changed
            this.setStatus('validate', 'idle');
            this.setStatus('checkin', 'idle');
            this.setStatus('deploy', 'idle');
            this.setStatus('job', 'idle');
            this.notebookPath.set('');
            this.gitPath.set('');
            this.jobId.set('');
            this.setStatus('convert', 'done', this.engineLabel(res.engine));
            this.busy.set(false);
            resolve(true);
          },
          error: (err) => {
            this.setStatus('convert', 'error', err?.error?.error || 'failed');
            this.error.set(err?.error?.error || 'Conversion failed.');
            this.busy.set(false);
            resolve(false);
          },
        });
    });
  }

  // Validation is computed server-side during convert; this is the human review gate.
  runValidate(): Promise<boolean> {
    return new Promise((resolve) => {
      const c = this.conversion();
      if (!c) return resolve(false);
      if (c.validation.ok) {
        this.setStatus('validate', 'done', 'Syntax OK · ' + (c.validation.checkedWith || ''));
        resolve(true);
      } else {
        this.setStatus('validate', 'error', `${c.validation.issues.length} issue(s)`);
        this.error.set('Validation found issues — review before deploying.');
        resolve(false);
      }
    });
  }

  runCheckin(): Promise<boolean> {
    return new Promise((resolve) => {
      const c = this.conversion();
      if (!c) return resolve(false);
      this.busy.set(true);
      this.setStatus('checkin', 'running');
      this.api
        .conversionCheckin(this.wsId, { fileName: this.fileName(), code: c.code, language: c.language })
        .subscribe({
          next: (res) => {
            this.gitPath.set(res.path || '');
            this.setStatus('checkin', res.committed ? 'done' : 'error',
              res.committed ? res.path : (res.output || 'not committed'));
            this.busy.set(false);
            resolve(!!res.committed);
          },
          error: (err) => {
            this.setStatus('checkin', 'error', err?.error?.error || 'failed');
            this.busy.set(false);
            resolve(false);
          },
        });
    });
  }

  runDeploy(): Promise<boolean> {
    return new Promise((resolve) => {
      const c = this.conversion();
      if (!c) return resolve(false);
      this.busy.set(true);
      this.setStatus('deploy', 'running');
      this.api
        .conversionDeploy(this.wsId, { fileName: this.fileName(), code: c.code, language: c.language })
        .subscribe({
          next: (res) => {
            if (res.deployed) {
              this.notebookPath.set(res.notebookPath || '');
              this.setStatus('deploy', 'done', res.notebookPath || '');
              this.busy.set(false);
              resolve(true);
            } else {
              this.setStatus('deploy', 'error', res.error || 'not deployed');
              this.error.set('Databricks deploy: ' + (res.error || 'failed'));
              this.busy.set(false);
              resolve(false);
            }
          },
          error: (err) => {
            this.setStatus('deploy', 'error', err?.error?.error || 'failed');
            this.busy.set(false);
            resolve(false);
          },
        });
    });
  }

  runJob(run = true): Promise<boolean> {
    return new Promise((resolve) => {
      if (!this.notebookPath()) return resolve(false);
      this.busy.set(true);
      this.setStatus('job', 'running');
      this.api
        .conversionJob(this.wsId, { name: this.fileName(), notebookPath: this.notebookPath(), run })
        .subscribe({
          next: (res) => {
            if (res.created) {
              this.jobId.set(res.jobId || '');
              this.runId.set(res.runId || '');
              const note = res.run ? `job ${res.jobId} · run ${res.runId}` : `job ${res.jobId} created`;
              this.setStatus('job', 'done', res.run === false && res.runError ? `job ${res.jobId} · run failed` : note);
              this.busy.set(false);
              resolve(true);
            } else {
              this.setStatus('job', 'error', res.error || 'not created');
              this.error.set('Databricks job: ' + (res.error || 'failed'));
              this.busy.set(false);
              resolve(false);
            }
          },
          error: (err) => {
            this.setStatus('job', 'error', err?.error?.error || 'failed');
            this.busy.set(false);
            resolve(false);
          },
        });
    });
  }

  async runEndToEnd(): Promise<void> {
    const confirmed = window.confirm(
      'Run the full pipeline end-to-end for this file?\n\n' +
        'This will Analyse → Convert → Validate, then COMMIT to Git, DEPLOY a notebook to Databricks, ' +
        'and CREATE & RUN a Databricks job. These last three have real side effects.',
    );
    if (!confirmed) return;
    if (!(await this.runAnalyze())) return;
    if (!(await this.runConvert())) return;
    if (!(await this.runValidate())) return;
    await this.runCheckin();
    if (await this.runDeploy()) {
      await this.runJob(true);
    }
    this.saveAudit();
  }

  // ---- audit ----
  private currentStatusLabel(): string {
    if (this.jobId()) return 'JOB_TRIGGERED';
    if (this.notebookPath()) return 'DEPLOYED';
    if (this.gitPath()) return 'CHECKED_IN';
    if (this.conversion()) return 'CONVERTED';
    return 'ANALYZED';
  }

  saveAudit(): void {
    const a = this.analysis();
    const c = this.conversion();
    if (!a || !c) {
      this.error.set('Convert the file before saving to the audit trail.');
      return;
    }
    this.savingAudit.set(true);
    this.api
      .saveConversionAudit(this.wsId, {
        fileName: this.fileName(),
        sourceType: a.source.sourceType,
        sourceLabel: a.source.sourceLabel,
        target: this.target(),
        modelUsed: c.modelUsed || this.selectedModel,
        engine: c.engine,
        planMarkdown: a.planMarkdown,
        sourceCode: this.fileContent,
        convertedCode: c.code,
        language: c.language,
        validation: c.validation,
        status: this.currentStatusLabel(),
        gitPath: this.gitPath(),
        notebookPath: this.notebookPath(),
        jobId: this.jobId(),
        runId: this.runId(),
      })
      .subscribe({
        next: () => {
          this.savingAudit.set(false);
          this.loadAudit();
        },
        error: (err) => {
          this.savingAudit.set(false);
          this.error.set(err?.error?.error || 'Could not save audit record.');
        },
      });
  }

  loadAudit(): void {
    this.api.listConversionAudit(this.wsId).subscribe({
      next: (rows) => this.audit.set(rows || []),
      error: () => {},
    });
  }

  selectAudit(row: any): void {
    this.selectedAudit.set(this.selectedAudit()?.conversionId === row.conversionId ? null : row);
  }

  // ---- downloads ----
  downloadPlan(): void {
    const a = this.analysis();
    if (!a) return;
    this.download(this.baseName() + '_conversion_plan.md', a.planMarkdown);
  }

  downloadCode(): void {
    const c = this.conversion();
    if (!c) return;
    const ext = c.language === 'python' ? '.py' : '.sql';
    this.download(this.baseName() + '_converted' + ext, c.code);
  }

  private download(name: string, text: string): void {
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    a.click();
    URL.revokeObjectURL(url);
  }

  private baseName(): string {
    const n = this.fileName() || 'artifact';
    return n.replace(/\.[^.]+$/, '') || 'artifact';
  }

  // ---- display helpers ----
  engineLabel(engine: string): string {
    if (engine === 'llm') return 'Converted by LLM';
    if (engine === 'lakebridge') return 'Lakebridge (placeholder)';
    return 'Heuristic transpile';
  }

  toneClass(tone: string): string {
    return ['good', 'warn', 'bad'].includes(tone) ? tone : 'info';
  }

  stepIconState(id: StepId): string {
    if (this.status(id) === 'done') return 'check';
    return this.steps.find((x) => x.id === id)?.icon || 'agent';
  }

  // ---- minimal markdown -> HTML (Angular sanitizes bound output) ----
  private escape(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  private inline(s: string): string {
    return this.escape(s)
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
  }

  private mdToHtml(md: string): string {
    if (!md) return '';
    const lines = md.split('\n');
    const out: string[] = [];
    let listType: 'ul' | 'ol' | null = null;
    const closeList = () => {
      if (listType) {
        out.push(`</${listType}>`);
        listType = null;
      }
    };
    for (const raw of lines) {
      const line = raw.trimEnd();
      if (!line.trim()) {
        closeList();
        continue;
      }
      let m: RegExpMatchArray | null;
      if ((m = line.match(/^###\s+(.*)$/))) {
        closeList();
        out.push(`<h4>${this.inline(m[1])}</h4>`);
      } else if ((m = line.match(/^##\s+(.*)$/))) {
        closeList();
        out.push(`<h3>${this.inline(m[1])}</h3>`);
      } else if ((m = line.match(/^#\s+(.*)$/))) {
        closeList();
        out.push(`<h3>${this.inline(m[1])}</h3>`);
      } else if ((m = line.match(/^\s*[-*]\s+(.*)$/))) {
        if (listType !== 'ul') {
          closeList();
          listType = 'ul';
          out.push('<ul>');
        }
        out.push(`<li>${this.inline(m[1])}</li>`);
      } else if ((m = line.match(/^\s*\d+\.\s+(.*)$/))) {
        if (listType !== 'ol') {
          closeList();
          listType = 'ol';
          out.push('<ol>');
        }
        out.push(`<li>${this.inline(m[1])}</li>`);
      } else {
        closeList();
        out.push(`<p>${this.inline(line)}</p>`);
      }
    }
    closeList();
    return out.join('');
  }
}
