import { Injectable, signal } from '@angular/core';

export interface ChatMsg {
  role: 'user' | 'assistant';
  content: string;
}

export interface RuleImpact {
  impactedCount: number;
  totalCount: number;
  impactedPercent: number;
}

export interface RuleProvenance {
  prompt: string;
  conversation: ChatMsg[];
  toolTraces: unknown[];
  modelUsed: string | null;
  dataSource: string | null;
  impact?: RuleImpact | null;
  at: string;
}

export interface RecommendedRule {
  ruleName: string;
  dimension: string;
  businessDefinition: string;
  technicalDefinition: string;
  impactedTables: string[];
  impactedColumns: string[];
  checkSql: string;
  impact?: RuleImpact | null;
  provenance?: RuleProvenance;
}

export interface DataPreview {
  title: string;
  sql: string;
  columns: string[];
  rows: Record<string, unknown>[];
  impact?: RuleImpact | null;
}

export interface AnalyserState {
  messages: ChatMsg[];
  recommendedRules: RecommendedRule[];
  dataPreview: DataPreview | null;
  model: string;
  dataSource: string | null;
}

const DEFAULT_MODEL = 'claude-sonnet-4-20250514';

@Injectable({ providedIn: 'root' })
export class ConversationService {
  private byWorkspace = new Map<number, AnalyserState>();
  readonly active = signal<AnalyserState | null>(null);
  private activeWs = 0;

  ensure(wsId: number): AnalyserState {
    if (!this.byWorkspace.has(wsId)) {
      this.byWorkspace.set(wsId, {
        messages: [],
        recommendedRules: [],
        dataPreview: null,
        model: DEFAULT_MODEL,
        dataSource: null,
      });
    }
    this.activeWs = wsId;
    const state = this.byWorkspace.get(wsId)!;
    this.active.set({ ...state, messages: [...state.messages] });
    return state;
  }

  get(wsId: number): AnalyserState {
    return this.ensure(wsId);
  }

  setModel(wsId: number, model: string): void {
    const s = this.ensure(wsId);
    s.model = model;
    this.bump(wsId);
  }

  appendMessage(wsId: number, msg: ChatMsg): void {
    const s = this.ensure(wsId);
    s.messages.push(msg);
    this.bump(wsId);
  }

  applyResponse(
    wsId: number,
    answer: string,
    rules: RecommendedRule[],
    preview: DataPreview | null,
    dataSource: string | null = null,
    modelUsed: string | null = null,
    toolTraces: unknown[] = [],
  ): void {
    const s = this.ensure(wsId);
    // Capture the triggering prompt BEFORE appending the assistant reply.
    const lastUser = [...s.messages].reverse().find((m) => m.role === 'user');
    s.messages.push({ role: 'assistant', content: answer });

    if (rules?.length) {
      // Provenance snapshot: the prompt + full interaction that produced these rules.
      const provenance: RuleProvenance = {
        prompt: lastUser?.content || '',
        conversation: s.messages.map((m) => ({ role: m.role, content: m.content })),
        toolTraces: toolTraces || [],
        modelUsed,
        dataSource,
        at: new Date().toISOString(),
      };
      // Merge by ruleName, refreshing impact stats on rules seen again.
      const byName = new Map(s.recommendedRules.map((r) => [r.ruleName, r]));
      for (const r of rules) {
        const existing = byName.get(r.ruleName);
        if (existing) {
          existing.impact = r.impact ?? existing.impact;
        } else {
          r.provenance = { ...provenance, impact: r.impact ?? null };
          s.recommendedRules.unshift(r);
          byName.set(r.ruleName, r);
        }
      }
    }
    if (preview) s.dataPreview = preview;
    if (dataSource) s.dataSource = dataSource;
    this.bump(wsId);
  }

  history(wsId: number): { role: string; content: string }[] {
    return this.ensure(wsId).messages.map((m) => ({ role: m.role, content: m.content }));
  }

  private bump(wsId: number): void {
    if (wsId !== this.activeWs) return;
    const s = this.ensure(wsId);
    this.active.set({
      ...s,
      messages: [...s.messages],
      recommendedRules: [...s.recommendedRules],
      dataPreview: s.dataPreview ? { ...s.dataPreview, rows: [...s.dataPreview.rows] } : null,
      dataSource: s.dataSource,
    });
  }
}
