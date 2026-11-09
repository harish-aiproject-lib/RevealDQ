import { Component, OnInit, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';
import { FlowGraphComponent } from './flow-graph.component';

interface LlmOption {
  id: string;
  label: string;
}

interface EtlMetric {
  label: string;
  value: string;
  hint: string;
  tone: string;
}

interface EtlStep {
  index: number;
  name: string;
  type: string;
  detail: string;
  kind: string;
}

interface EtlGraph {
  nodes: any[];
  edges: any[];
}

interface EtlAnalysis {
  summaryMarkdown: string;
  metrics: EtlMetric[];
  steps: EtlStep[];
  graph: EtlGraph;
  fileType: string;
  modelUsed: string;
}

interface SelectedNode {
  id: string;
  label: string;
  ntype: string;
  kind: string;
  upstream: string[];
  downstream: string[];
}

interface ChatMsg {
  role: 'user' | 'assistant';
  content: string;
  html: string;
}

@Component({
  selector: 'app-etl',
  standalone: true,
  imports: [FormsModule, IconComponent, FlowGraphComponent],
  templateUrl: './etl.component.html',
  styleUrl: './etl.component.scss',
})
export class EtlComponent implements OnInit {
  wsId = 0;

  readonly fileTypes = [
    { id: 'sql', label: 'SQL script (.sql)', icon: 'database' },
    { id: 'informatica_xml', label: 'Informatica ETL (.xml)', icon: 'convert' },
  ];
  fileType = signal<'sql' | 'informatica_xml'>('sql');

  readonly models: LlmOption[] = [
    { id: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4 (default)' },
    { id: 'claude-3-5-haiku-20241022', label: 'Claude Haiku 3.5 (cheaper)' },
  ];
  selectedModel = 'claude-sonnet-4-20250514';

  fileName = signal('');
  fileContent = '';

  analysis = signal<EtlAnalysis | null>(null);
  messages = signal<ChatMsg[]>([]);
  question = '';
  loading = signal(false);
  error = signal('');

  selectedNode = signal<SelectedNode | null>(null);
  nodeExplanationHtml = signal('');
  nodeExplaining = signal(false);
  nodeExplainSource = signal('');

  summaryHtml = computed(() => this.mdToHtml(this.analysis()?.summaryMarkdown || ''));
  graphNodes = computed(() => this.analysis()?.graph?.nodes || []);
  graphEdges = computed(() => this.analysis()?.graph?.edges || []);

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
  }

  setFileType(id: 'sql' | 'informatica_xml'): void {
    this.fileType.set(id);
  }

  onFilePicked(event: Event): void {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    this.error.set('');
    const lower = file.name.toLowerCase();
    if (lower.endsWith('.xml')) this.fileType.set('informatica_xml');
    else if (lower.endsWith('.sql')) this.fileType.set('sql');
    const reader = new FileReader();
    reader.onload = () => {
      this.fileContent = String(reader.result || '');
      this.fileName.set(file.name);
      this.analysis.set(null);
      this.messages.set([]);
    };
    reader.onerror = () => this.error.set('Could not read the file.');
    reader.readAsText(file);
  }

  analyze(): void {
    if (!this.fileContent.trim()) {
      this.error.set('Upload a .sql or Informatica .xml file first.');
      return;
    }
    this.error.set('');
    this.loading.set(true);
    this.messages.set([]);
    this.selectedNode.set(null);
    this.nodeExplanationHtml.set('');
    this.api
      .etlAnalyze(this.wsId, {
        fileName: this.fileName(),
        fileType: this.fileType(),
        content: this.fileContent,
        model: this.selectedModel,
      })
      .subscribe({
        next: (res) => {
          this.loading.set(false);
          this.analysis.set({
            summaryMarkdown: res.summaryMarkdown || '',
            metrics: res.metrics || [],
            steps: res.steps || [],
            graph: res.graph || { nodes: [], edges: [] },
            fileType: res.fileType || this.fileType(),
            modelUsed: res.modelUsed || this.selectedModel,
          });
          this.pushAssistant(res.answer || 'Analysis complete.');
        },
        error: (err) => {
          this.loading.set(false);
          this.error.set(err?.error?.error || 'ETL analysis failed.');
        },
      });
  }

  ask(): void {
    const text = this.question.trim();
    if (!text || this.loading() || !this.analysis()) return;
    this.error.set('');
    this.pushUser(text);
    this.question = '';
    this.loading.set(true);

    const history = this.messages()
      .slice(0, -1)
      .map((m) => ({ role: m.role, content: m.content }));

    this.api
      .etlAnalyze(this.wsId, {
        fileName: this.fileName(),
        fileType: this.fileType(),
        content: this.fileContent,
        message: text,
        history,
        model: this.selectedModel,
      })
      .subscribe({
        next: (res) => {
          this.loading.set(false);
          this.pushAssistant(res.answer || 'No answer returned.');
        },
        error: (err) => {
          this.loading.set(false);
          this.error.set(err?.error?.error || 'Follow-up failed.');
        },
      });
  }

  onNodeClick(evt: { id: string; data: any }): void {
    const d = evt.data || {};
    const node: SelectedNode = {
      id: evt.id,
      label: d.label || evt.id,
      ntype: d.ntype || '',
      kind: d.kind || '',
      upstream: d.upstream || [],
      downstream: d.downstream || [],
    };
    this.selectedNode.set(node);
    // Show the heuristic explanation immediately, then enrich via the reasoning agent.
    this.nodeExplanationHtml.set(this.mdToHtml(d.explanation || ''));
    this.nodeExplainSource.set('');
    this.nodeExplaining.set(true);
    this.api
      .etlExplainNode(this.wsId, {
        fileName: this.fileName(),
        fileType: this.fileType(),
        content: this.fileContent,
        node,
        model: this.selectedModel,
      })
      .subscribe({
        next: (res) => {
          this.nodeExplaining.set(false);
          if (res?.explanation) this.nodeExplanationHtml.set(this.mdToHtml(res.explanation));
          this.nodeExplainSource.set(res?.source || '');
        },
        error: () => this.nodeExplaining.set(false),
      });
  }

  closeNode(): void {
    this.selectedNode.set(null);
    this.nodeExplanationHtml.set('');
  }

  private pushUser(content: string): void {
    this.messages.update((m) => [...m, { role: 'user', content, html: this.escape(content) }]);
  }

  private pushAssistant(content: string): void {
    this.messages.update((m) => [...m, { role: 'assistant', content, html: this.mdToHtml(content) }]);
  }

  toneClass(tone: string): string {
    return ['good', 'warn', 'bad'].includes(tone) ? tone : 'info';
  }

  kindClass(kind: string): string {
    return ['source', 'target'].includes(kind) ? kind : 'transform';
  }

  fileTypeLabel(): string {
    const a = this.analysis();
    return a?.fileType === 'informatica_xml' ? 'Informatica ETL' : 'SQL script';
  }

  // --- minimal, safe markdown → HTML (Angular sanitizes the bound result) ---
  private escape(s: string): string {
    return s
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
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
