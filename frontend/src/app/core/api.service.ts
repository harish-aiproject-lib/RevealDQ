import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../environments/environment';

@Injectable({ providedIn: 'root' })
export class ApiService {
  private base = environment.apiUrl;

  constructor(private http: HttpClient) {}

  /** Reality Reveal product-module catalog (Reveal DQ, Reveal VoC, ...). */
  listProducts() {
    return this.http.get<any[]>(`${this.base}/products`);
  }

  listWorkspaces() {
    return this.http.get<any[]>(`${this.base}/workspaces`);
  }

  createWorkspace(body: {
    workspaceName: string;
    customerName: string;
    description?: string;
    productCode: string;
  }) {
    return this.http.post<any>(`${this.base}/workspaces`, body);
  }

  getWorkspace(id: number) {
    return this.http.get<any>(`${this.base}/workspaces/${id}`);
  }

  listConnections(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/connections`);
  }

  createConnection(ws: number, body: any) {
    return this.http.post(`${this.base}/workspaces/${ws}/connections`, body);
  }

  deleteConnection(ws: number, connId: number) {
    return this.http.delete<any>(`${this.base}/workspaces/${ws}/connections/${connId}`);
  }

  testConnection(ws: number, connId: number) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/connections/${connId}/test`, {});
  }

  listRules(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/rules`);
  }

  searchRules(
    ws: number,
    q: { q?: string; table?: string; dimension?: string; status?: string },
  ) {
    const p = new URLSearchParams();
    if (q.q) p.set('q', q.q);
    if (q.table) p.set('table', q.table);
    if (q.dimension) p.set('dimension', q.dimension);
    if (q.status) p.set('status', q.status);
    const qs = p.toString();
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/rules/search${qs ? '?' + qs : ''}`);
  }

  createRule(ws: number, body: any) {
    return this.http.post(`${this.base}/workspaces/${ws}/rules`, body);
  }

  updateRule(ws: number, ruleId: number, status: string) {
    return this.http.patch(`${this.base}/workspaces/${ws}/rules/${ruleId}`, { status });
  }

  editRule(
    ws: number,
    ruleId: number,
    body: { businessDefinition: string; technicalDefinition: string; checkSql: string },
  ) {
    return this.http.put<any>(`${this.base}/workspaces/${ws}/rules/${ruleId}`, body);
  }

  deleteRule(ws: number, ruleId: number) {
    return this.http.delete<any>(`${this.base}/workspaces/${ws}/rules/${ruleId}`);
  }

  listAudit(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/audit`);
  }

  listPipelines(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/pipelines`);
  }

  runPipeline(ws: number, pipelineId: number) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/pipelines/${pipelineId}/run`, {});
  }

  listRuns(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/runs`);
  }

  dashboard(ws: number) {
    return this.http.get<any>(`${this.base}/workspaces/${ws}/dashboard`);
  }

  getAcl(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/acl`);
  }

  updateAcl(ws: number, body: any) {
    return this.http.put(`${this.base}/workspaces/${ws}/acl`, body);
  }

  chat(ws: number, message: string, history: any[] = [], model?: string) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/analyser/chat`, {
      message,
      history,
      model,
    });
  }

  etlAnalyze(
    ws: number,
    body: {
      fileName: string;
      fileType: string;
      content: string;
      message?: string;
      history?: any[];
      model?: string;
    },
  ) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/etl/analyze`, body);
  }

  etlExplainNode(
    ws: number,
    body: {
      fileName: string;
      fileType: string;
      content: string;
      node: { id: string; label: string; ntype: string; kind: string; upstream: string[]; downstream: string[] };
      model?: string;
    },
  ) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/etl/explain-node`, body);
  }

  conversionAnalyze(ws: number, body: { fileName: string; content: string }) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/conversion/analyze`, body);
  }

  conversionRun(
    ws: number,
    body: { fileName: string; content: string; sourceType: string; target: string; model: string },
  ) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/conversion/run`, body);
  }

  conversionCheckin(ws: number, body: { fileName: string; code: string; language: string }) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/conversion/checkin`, body);
  }

  conversionDeploy(ws: number, body: { fileName: string; code: string; language: string }) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/conversion/deploy`, body);
  }

  conversionJob(ws: number, body: { name: string; notebookPath: string; run: boolean }) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/conversion/job`, body);
  }

  saveConversionAudit(ws: number, body: any) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/conversion/audit`, body);
  }

  listConversionAudit(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/conversion/audit`);
  }

  // ---------------------------------------------------------------------
  // Reveal VoC — Voice of Customer (LinkedIn listening)
  // ---------------------------------------------------------------------

  private voc(ws: number, path: string) {
    return `${this.base}/workspaces/${ws}/voc/${path}`;
  }

  vocOverview(ws: number) {
    return this.http.get<any>(this.voc(ws, 'overview'));
  }

  vocStats(ws: number) {
    return this.http.get<any>(this.voc(ws, 'stats'));
  }

  vocCompanies(ws: number) {
    return this.http.get<any[]>(this.voc(ws, 'companies'));
  }

  vocCreateCompany(
    ws: number,
    body: { companyName: string; linkedinHandle: string; hashtags: string[]; syncNow?: boolean },
  ) {
    return this.http.post<any>(this.voc(ws, 'companies'), body);
  }

  vocUpdateCompany(ws: number, companyId: number, body: { isActive?: boolean }) {
    return this.http.patch<any>(this.voc(ws, `companies/${companyId}`), body);
  }

  vocDeleteCompany(ws: number, companyId: number) {
    return this.http.delete<any>(this.voc(ws, `companies/${companyId}`));
  }

  vocSources(ws: number) {
    return this.http.get<any>(this.voc(ws, 'sources'));
  }

  vocCreateSource(
    ws: number,
    body: { provider: string; label?: string; accountEmail?: string; config?: Record<string, any> },
  ) {
    return this.http.post<any>(this.voc(ws, 'sources'), body);
  }

  vocDeleteSource(ws: number, sourceId: number) {
    return this.http.delete<any>(this.voc(ws, `sources/${sourceId}`));
  }

  vocOAuthStart(ws: number, sourceId: number) {
    return this.http.post<{ url: string }>(this.voc(ws, `sources/${sourceId}/oauth/start`), {});
  }

  vocImport(ws: number, sourceId: number, companyId: number, payload: any) {
    return this.http.post<any>(this.voc(ws, `sources/${sourceId}/import?companyId=${companyId}`), payload);
  }

  /**
   * Trigger extraction + analysis. `from`/`to` are inclusive YYYY-MM-DD bounds
   * for the scrape window; omit both for an incremental sync (5-day window on
   * the first run).
   */
  vocSync(ws: number, companyId?: number, range?: { from?: string; to?: string }) {
    return this.http.post<any>(this.voc(ws, 'sync'), {
      companyId: companyId || 0,
      from: range?.from || '',
      to: range?.to || '',
    });
  }

  vocReanalyse(ws: number, companyId?: number) {
    return this.http.post<any>(this.voc(ws, 'reanalyse'), { companyId: companyId || 0 });
  }

  vocRuns(ws: number, limit = 30) {
    return this.http.get<any[]>(this.voc(ws, `runs?limit=${limit}`));
  }

  vocSchedule(ws: number) {
    return this.http.get<any>(this.voc(ws, 'schedule'));
  }

  vocPutSchedule(ws: number, body: { enabled: boolean; cadenceMinutes: number }) {
    return this.http.put<any>(this.voc(ws, 'schedule'), body);
  }

  vocPosts(
    ws: number,
    f: { companyId?: number; sentiment?: string; category?: string; kind?: string; q?: string; limit?: number; offset?: number } = {},
  ) {
    const params = Object.entries(f)
      .filter(([, v]) => v !== undefined && v !== null && v !== '' && v !== 0)
      .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
      .join('&');
    return this.http.get<{ items: any[]; total: number }>(this.voc(ws, `posts${params ? '?' + params : ''}`));
  }

  vocActions(ws: number, status?: string) {
    return this.http.get<any[]>(this.voc(ws, `actions${status ? '?status=' + status : ''}`));
  }

  vocCreateAction(
    ws: number,
    body: { companyId?: number | null; title: string; description?: string; category?: string; priority?: string },
  ) {
    return this.http.post<any>(this.voc(ws, 'actions'), body);
  }

  vocUpdateAction(ws: number, actionId: number, body: { status?: string; owner?: string; priority?: string }) {
    return this.http.patch<any>(this.voc(ws, `actions/${actionId}`), body);
  }

  vocAsk(ws: number, question: string, companyId?: number, model?: string) {
    return this.http.post<any>(this.voc(ws, 'ask'), { question, companyId: companyId || 0, model });
  }
}
