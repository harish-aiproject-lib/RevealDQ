import { Component, OnInit, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';

@Component({
  selector: 'app-pipelines',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './pipelines.component.html',
  styleUrl: './pipelines.component.scss',
})
export class PipelinesComponent implements OnInit {
  wsId = 0;
  pipelines = signal<any[]>([]);
  runs = signal<any[]>([]);
  message = '';
  error = '';
  runningId: number | null = null;

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.reload();
  }

  reload(): void {
    this.api.listPipelines(this.wsId).subscribe({ next: (p) => this.pipelines.set(p || []) });
    this.api.listRuns(this.wsId).subscribe({ next: (r) => this.runs.set(r || []) });
  }

  run(p: any): void {
    this.runningId = p.pipelineId;
    this.message = '';
    this.error = '';
    this.api.runPipeline(this.wsId, p.pipelineId).subscribe({
      next: (res) => {
        this.runningId = null;
        this.message = `${p.pipelineName}: ${res.status} — checked ${res.rowsChecked}, failed ${res.rowsFailed}`;
        this.reload();
      },
      error: (err) => {
        this.runningId = null;
        this.error = err?.error?.error || 'Run failed';
      },
    });
  }
}
