import { Injectable, signal } from '@angular/core';

export interface ChatMsg {
  role: 'user' | 'assistant';
  content: string;
}

export interface RecommendedRule {
  ruleName: string;
  dimension: string;
  businessDefinition: string;
  technicalDefinition: string;
  impactedTables: string[];
  impactedColumns: string[];
  checkSql: string;
}

export interface DataPreview {
  title: string;
  sql: string;
  columns: string[];
  rows: Record<string, unknown>[];
}

export interface AnalyserState {
  messages: ChatMsg[];
  recommendedRules: RecommendedRule[];
  dataPreview: DataPreview | null;
  model: string;
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
  ): void {
    const s = this.ensure(wsId);
    s.messages.push({ role: 'assistant', content: answer });
    if (rules?.length) {
      // merge unique by ruleName
      const names = new Set(s.recommendedRules.map((r) => r.ruleName));
      for (const r of rules) {
        if (!names.has(r.ruleName)) s.recommendedRules.unshift(r);
      }
    }
    if (preview) s.dataPreview = preview;
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
    });
  }
}
