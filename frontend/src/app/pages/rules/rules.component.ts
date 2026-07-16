import { Component, OnInit, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';

@Component({
  selector: 'app-rules',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './rules.component.html',
  styleUrl: './rules.component.scss',
})
export class RulesComponent implements OnInit {
  wsId = 0;
  rules = signal<any[]>([]);
  error = '';

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.reload();
  }

  reload(): void {
    this.api.listRules(this.wsId).subscribe({
      next: (list) => this.rules.set(list || []),
      error: (err) => (this.error = err?.error?.error || 'Failed to load rules'),
    });
  }

  setStatus(rule: any, status: string): void {
    this.api.updateRule(this.wsId, rule.ruleId, status).subscribe({
      next: () => this.reload(),
      error: (err) => (this.error = err?.error?.error || 'Update failed'),
    });
  }
}
