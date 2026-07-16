import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { ConversationService } from '../../core/conversation.service';

interface LlmOption {
  id: string;
  label: string;
}

@Component({
  selector: 'app-analyser',
  standalone: true,
  imports: [FormsModule],
  templateUrl: './analyser.component.html',
  styleUrl: './analyser.component.scss',
})
export class AnalyserComponent implements OnInit {
  wsId = 0;
  message = 'What DQ issues exist across CDR to rating to invoice?';
  loading = false;
  error = '';
  saveNote = '';

  mcpConnected = signal(false);
  mcpDetails = signal<{ host: string; port: number; database: string; username: string; name: string } | null>(
    null,
  );

  readonly models: LlmOption[] = [
    { id: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4 (default)' },
    { id: 'claude-3-5-haiku-20241022', label: 'Claude Haiku 3.5 (cheaper)' },
  ];

  selectedModel = 'claude-sonnet-4-20250514';

  state = computed(() => this.conversation.active());

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
    private conversation: ConversationService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    const s = this.conversation.ensure(this.wsId);
    this.selectedModel = s.model;
    this.loadMcp();
  }

  loadMcp(): void {
    this.api.listConnections(this.wsId).subscribe({
      next: (list) => {
        const c = (list || [])[0];
        if (!c) {
          this.mcpConnected.set(false);
          this.mcpDetails.set(null);
          return;
        }
        this.mcpDetails.set({
          host: c.host,
          port: c.port,
          database: c.databaseName,
          username: c.username,
          name: c.connectionName,
        });
        this.api.testConnection(this.wsId, c.connectionId).subscribe({
          next: (res) => this.mcpConnected.set(!!res?.ok),
          error: () => this.mcpConnected.set(false),
        });
      },
      error: () => this.mcpConnected.set(false),
    });
  }

  onModelChange(): void {
    this.conversation.setModel(this.wsId, this.selectedModel);
  }

  send(): void {
    const text = this.message.trim();
    if (!text || this.loading) return;
    this.error = '';
    this.saveNote = '';
    this.conversation.appendMessage(this.wsId, { role: 'user', content: text });
    this.message = '';
    this.loading = true;

    const history = this.conversation.history(this.wsId).slice(0, -1); // exclude just-added user msg duplicate if agent uses it as message
    this.api.chat(this.wsId, text, history, this.selectedModel).subscribe({
      next: (res) => {
        this.loading = false;
        this.conversation.applyResponse(
          this.wsId,
          res.answer || 'No answer returned.',
          res.recommendedRules || [],
          res.dataPreview || null,
        );
      },
      error: (err) => {
        this.loading = false;
        this.error = err?.error?.error || 'Analyser request failed';
      },
    });
  }

  saveRule(rule: any): void {
    this.api
      .createRule(this.wsId, {
        ruleName: rule.ruleName,
        dimension: rule.dimension,
        businessDefinition: rule.businessDefinition,
        technicalDefinition: rule.technicalDefinition,
        impactedTables: rule.impactedTables || [],
        impactedColumns: rule.impactedColumns || [],
        checkSql: rule.checkSql,
        status: 'DRAFT',
      })
      .subscribe({
        next: () => (this.saveNote = `Saved draft rule: ${rule.ruleName}`),
        error: (err) => (this.error = err?.error?.error || 'Save rule failed'),
      });
  }

  cellValue(row: Record<string, unknown>, col: string): string {
    const v = row[col];
    if (v === null || v === undefined) return '—';
    return String(v);
  }
}
