import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../environments/environment';

@Injectable({ providedIn: 'root' })
export class ApiService {
  private base = environment.apiUrl;

  constructor(private http: HttpClient) {}

  listWorkspaces() {
    return this.http.get<any[]>(`${this.base}/workspaces`);
  }

  createWorkspace(body: { workspaceName: string; customerName: string; description?: string }) {
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

  testConnection(ws: number, connId: number) {
    return this.http.post<any>(`${this.base}/workspaces/${ws}/connections/${connId}/test`, {});
  }

  listRules(ws: number) {
    return this.http.get<any[]>(`${this.base}/workspaces/${ws}/rules`);
  }

  createRule(ws: number, body: any) {
    return this.http.post(`${this.base}/workspaces/${ws}/rules`, body);
  }

  updateRule(ws: number, ruleId: number, status: string) {
    return this.http.patch(`${this.base}/workspaces/${ws}/rules/${ruleId}`, { status });
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
}
