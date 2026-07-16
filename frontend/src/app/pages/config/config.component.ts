import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';

@Component({
  selector: 'app-config',
  standalone: true,
  imports: [FormsModule, IconComponent],
  templateUrl: './config.component.html',
  styleUrl: './config.component.scss',
})
export class ConfigComponent implements OnInit {
  wsId = 0;
  connections = signal<any[]>([]);
  form = {
    connectionName: 'Local RevealDQ MySQL',
    host: '127.0.0.1',
    port: 3306,
    databaseName: 'RevealDQ',
    username: 'root',
    password: '',
  };
  message = '';
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
    this.api.listConnections(this.wsId).subscribe({
      next: (list) => this.connections.set(list || []),
    });
  }

  save(): void {
    this.error = '';
    this.api.createConnection(this.wsId, this.form).subscribe({
      next: () => {
        this.message = 'Connection saved.';
        this.reload();
      },
      error: (err) => (this.error = err?.error?.error || 'Save failed'),
    });
  }

  test(connId: number): void {
    this.api.testConnection(this.wsId, connId).subscribe({
      next: (res) => {
        this.message = res.ok
          ? `Connection OK — ${res.tableCount} tables in schema.`
          : `Connection failed: ${res.error}`;
      },
      error: (err) => (this.error = err?.error?.error || 'Test failed'),
    });
  }
}
