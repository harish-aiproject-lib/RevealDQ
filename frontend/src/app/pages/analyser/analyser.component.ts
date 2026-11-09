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
    { id: 'claude-opus-4-20250514', label: 'Claude Opus 4 (highest quality)' },
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
        // MCP analyser is MySQL-only — prefer an active MySQL source, not Databricks/etc.
        const rows = list || [];
        const c =
          rows.find((x: any) => String(x.dbType || '').toUpperCase() === 'MYSQL' && x.isActive) ||
          rows.find((x: any) => String(x.dbType || '').toUpperCase() === 'MYSQL') ||
          null;
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
          res.dataSource || null,
          res.modelUsed || null,
          res.toolTraces || [],
        );
      },
      error: (err) => {
        this.loading = false;
        this.error = err?.error?.error || 'Analyser request failed';
      },
    });
  }

  saveRule(rule: any): void {
    const prov = rule.provenance || {};
    // Traceability record persisted with the rule for the future audit screen.
    const audit = {
      prompt: prov.prompt || '',
      conversation: prov.conversation || [],
      toolTraces: prov.toolTraces || [],
      modelUsed: prov.modelUsed || this.selectedModel,
      dataSource: prov.dataSource || this.state()?.dataSource || null,
      impact: rule.impact || null,
    };
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
        audit,
      })
      .subscribe({
        next: (res: any) => {
          if (res?.duplicate) {
            this.saveNote = res.message || `Rule already exists — ID #${res.ruleId} (not saved again).`;
          } else {
            this.saveNote = `Saved draft rule #${res?.ruleId} (with audit trail): ${rule.ruleName}`;
          }
        },
        error: (err) => (this.error = err?.error?.error || 'Save rule failed'),
      });
  }

  cellValue(row: Record<string, unknown>, col: string): string {
    const v = row[col];
    if (v === null || v === undefined) return '—';
    return String(v);
  }

  impactClass(percent: number | null | undefined): string {
    const p = percent ?? 0;
    if (p >= 25) return 'high';
    if (p >= 5) return 'mid';
    return 'low';
  }
}
