import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';

interface EditModel {
  businessDefinition: string;
  technicalDefinition: string;
  checkSql: string;
}

@Component({
  selector: 'app-rules',
  standalone: true,
  imports: [IconComponent, FormsModule],
  templateUrl: './rules.component.html',
  styleUrl: './rules.component.scss',
})
export class RulesComponent implements OnInit {
  wsId = 0;
  rules = signal<any[]>([]);
  audit = signal<any[]>([]);
  error = '';
  note = '';

  filter = { q: '', table: '', dimension: '', status: '' };
  readonly dimensions = ['ACCURACY', 'COMPLETENESS', 'CONSISTENCY', 'TIMELINESS', 'VALIDITY', 'UNIQUENESS'];
  readonly statuses = ['DRAFT', 'APPROVED', 'ACTIVE', 'RETIRED'];

  tab = signal<'rules' | 'audit'>('rules');
  editingId = signal<number | null>(null);
  editModel: EditModel = { businessDefinition: '', technicalDefinition: '', checkSql: '' };
  selectedAuditRuleId = signal<number | null>(null);

  // Audit records grouped by ruleId (latest first).
  private auditByRule = computed(() => {
    const map = new Map<number, any[]>();
    for (const a of this.audit()) {
      const key = a.ruleId as number;
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(a);
    }
    return map;
  });

  selectedAudit = computed(() => {
    const id = this.selectedAuditRuleId();
    if (id == null) return null;
    const list = this.auditByRule().get(id) || [];
    return list[0] || null;
  });

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.reload();
  }

  reload(): void {
    this.applyFilter();
    this.api.listAudit(this.wsId).subscribe({
      next: (list) => this.audit.set(list || []),
      error: () => this.audit.set([]),
    });
  }

  applyFilter(): void {
    this.api
      .searchRules(this.wsId, {
        q: this.filter.q.trim(),
        table: this.filter.table.trim(),
        dimension: this.filter.dimension,
        status: this.filter.status,
      })
      .subscribe({
        next: (list) => this.rules.set(list || []),
        error: (err) => (this.error = err?.error?.error || 'Failed to load rules'),
      });
  }

  clearFilter(): void {
    this.filter = { q: '', table: '', dimension: '', status: '' };
    this.applyFilter();
  }

  auditFor(ruleId: number): any | null {
    const list = this.auditByRule().get(ruleId) || [];
    return list[0] || null;
  }

  isActive(rule: any): boolean {
    return rule.status === 'ACTIVE';
  }

  canToggle(rule: any): boolean {
    return rule.status === 'APPROVED' || rule.status === 'ACTIVE';
  }

  approve(rule: any): void {
    this.note = '';
    this.api.updateRule(this.wsId, rule.ruleId, 'APPROVED').subscribe({
      next: () => {
        this.note = `Rule #${rule.ruleId} approved (inactive by default).`;
        this.reload();
      },
      error: (err) => (this.error = err?.error?.error || 'Approve failed'),
    });
  }

  reject(rule: any): void {
    this.note = '';
    // Rejected rules are never physicalised — hard delete.
    this.api.deleteRule(this.wsId, rule.ruleId).subscribe({
      next: () => {
        this.note = `Rule #${rule.ruleId} rejected and deleted.`;
        this.reload();
      },
      error: (err) => (this.error = err?.error?.error || 'Reject failed'),
    });
  }

  toggleActive(rule: any): void {
    const next = this.isActive(rule) ? 'APPROVED' : 'ACTIVE';
    this.api.updateRule(this.wsId, rule.ruleId, next).subscribe({
      next: () => {
        this.note = `Rule #${rule.ruleId} ${next === 'ACTIVE' ? 'activated' : 'deactivated'}.`;
        this.reload();
      },
      error: (err) => (this.error = err?.error?.error || 'Toggle failed'),
    });
  }

  startEdit(rule: any): void {
    this.editingId.set(rule.ruleId);
    this.editModel = {
      businessDefinition: rule.businessDefinition || '',
      technicalDefinition: rule.technicalDefinition || '',
      checkSql: rule.checkSql || '',
    };
  }

  cancelEdit(): void {
    this.editingId.set(null);
  }

  saveEdit(rule: any): void {
    this.error = '';
    this.api.editRule(this.wsId, rule.ruleId, this.editModel).subscribe({
      next: () => {
        this.note = `Rule #${rule.ruleId} updated.`;
        this.editingId.set(null);
        this.reload();
      },
      error: (err) => (this.error = err?.error?.error || 'Edit failed'),
    });
  }

  openAudit(rule: any): void {
    this.selectedAuditRuleId.set(rule.ruleId);
    this.tab.set('audit');
  }

  asList(v: unknown): any[] {
    return Array.isArray(v) ? v : [];
  }

  traceLabel(t: any): string {
    if (!t || typeof t !== 'object') return String(t);
    const tool = t.tool || t.name || 'step';
    if (t.error) return `${tool}: error`;
    if (tool === 'mcp.connect') return t.result?.reuse ? 'MCP channel (reused)' : 'MCP channel opened';
    if (tool === 'list_tables') return 'Listed tables';
    if (tool === 'describe_table') return `Described ${t.args?.table ?? 'table'}`;
    if (tool === 'run_select') return 'Queried sample data';
    if (tool === 'impact') return `Impact scored: ${t.rule ?? ''} ${t.percent ?? ''}%`;
    if (tool === 'metadata') return 'Metadata assessed';
    return tool;
  }

  fmtDate(v: string): string {
    if (!v) return '—';
    const d = new Date(v);
    return isNaN(d.getTime()) ? v : d.toLocaleString();
  }
}
