import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';

type DriverId = 'MYSQL' | 'SQLSERVER' | 'DATABRICKS';

interface Driver {
  id: DriverId;
  label: string;
  icon: string;
  note: string;
}

interface ConnStatus {
  state: string; // connected | failed | placeholder | needs_credentials | testing | unknown
  message: string;
}

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
  message = '';
  error = '';

  readonly drivers: Driver[] = [
    { id: 'MYSQL', label: 'MySQL', icon: 'database', note: 'RevealDQ warehouse — already configured.' },
    { id: 'SQLSERVER', label: 'SQL Server', icon: 'database', note: 'Placeholder — connector coming soon.' },
    { id: 'DATABRICKS', label: 'Databricks', icon: 'spark', note: 'SQL Warehouse via REST (PAT or OAuth service principal).' },
  ];
  selectedDriver = signal<DriverId>('MYSQL');

  mysql = {
    connectionName: 'Local RevealDQ MySQL',
    host: '127.0.0.1',
    port: 3306,
    databaseName: 'RevealDQ',
    username: 'root',
    password: '',
  };

  sqlserver = {
    connectionName: 'SQL Server (placeholder)',
    host: '',
    port: 1433,
    databaseName: '',
    username: '',
    password: '',
  };

  databricks = {
    connectionName: 'Databricks SQL Warehouse',
    host: 'dbc-1797d33a-6229.cloud.databricks.com',
    httpPath: '/sql/1.0/warehouses/f6641f80942d3e26',
    warehouseId: 'f6641f80942d3e26',
    workspaceId: '7474655352692357',
    oauthUrl: 'https://dbc-1797d33a-6229.cloud.databricks.com/oidc',
    catalog: 'default',
    schema: '',
    authType: 'token' as 'token' | 'oauth',
    token: '',
    clientId: '',
    clientSecret: '',
  };

  private statuses = signal<Record<number, ConnStatus>>({});

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

  selectDriver(id: DriverId): void {
    this.selectedDriver.set(id);
    this.message = '';
    this.error = '';
  }

  save(): void {
    this.error = '';
    this.message = '';
    const driver = this.selectedDriver();
    let body: any;

    if (driver === 'MYSQL') {
      body = { dbType: 'MYSQL', ...this.mysql };
    } else if (driver === 'SQLSERVER') {
      body = { dbType: 'SQLSERVER', ...this.sqlserver };
    } else {
      const d = this.databricks;
      const useOauth = d.authType === 'oauth';
      body = {
        dbType: 'DATABRICKS',
        connectionName: d.connectionName,
        host: d.host,
        port: 443,
        databaseName: d.catalog || 'default',
        username: useOauth ? d.clientId : 'token',
        password: useOauth ? d.clientSecret : d.token,
        httpPath: d.httpPath,
        extra: {
          warehouseId: d.warehouseId,
          authType: d.authType,
          clientId: d.clientId,
          workspaceId: d.workspaceId,
          oauthUrl: d.oauthUrl,
          catalog: d.catalog,
          schema: d.schema,
        },
      };
    }

    this.api.createConnection(this.wsId, body).subscribe({
      next: () => {
        this.message = `${this.driverLabel(driver)} connection saved.`;
        this.reload();
      },
      error: (err) => (this.error = err?.error?.error || 'Save failed'),
    });
  }

  test(connId: number): void {
    this.setStatus(connId, { state: 'testing', message: 'Testing…' });
    this.api.testConnection(this.wsId, connId).subscribe({
      next: (res) => {
        const state = res?.status || (res?.ok ? 'connected' : 'failed');
        this.setStatus(connId, { state, message: res?.message || res?.error || '' });
      },
      error: (err) =>
        this.setStatus(connId, { state: 'failed', message: err?.error?.error || 'Test failed' }),
    });
  }

  reconnect(connId: number): void {
    // Reconnect re-runs the live handshake; the agent/warehouse re-authenticates.
    this.test(connId);
  }

  remove(connId: number): void {
    this.api.deleteConnection(this.wsId, connId).subscribe({
      next: () => {
        this.message = 'Connection removed.';
        this.reload();
      },
      error: (err) => (this.error = err?.error?.error || 'Delete failed'),
    });
  }

  private setStatus(connId: number, s: ConnStatus): void {
    this.statuses.update((m) => ({ ...m, [connId]: s }));
  }

  statusFor(connId: number): ConnStatus {
    return this.statuses()[connId] || { state: 'unknown', message: 'Not tested yet.' };
  }

  statusLabel(state: string): string {
    return (
      {
        connected: 'Connected',
        failed: 'Failed',
        placeholder: 'Placeholder',
        needs_credentials: 'Needs credentials',
        testing: 'Testing…',
        unknown: 'Not tested',
      }[state] || state
    );
  }

  statusClass(state: string): string {
    if (state === 'connected') return 'ok';
    if (state === 'failed') return 'bad';
    if (state === 'placeholder' || state === 'needs_credentials') return 'warn';
    return 'muted';
  }

  driverNote(): string {
    return this.drivers.find((d) => d.id === this.selectedDriver())?.note || '';
  }

  driverLabel(dbType: string): string {
    return this.drivers.find((d) => d.id === (dbType as DriverId))?.label || dbType;
  }

  driverIcon(dbType: string): string {
    return this.drivers.find((d) => d.id === (dbType as DriverId))?.icon || 'database';
  }

  endpoint(c: any): string {
    if ((c.dbType || '').toUpperCase() === 'DATABRICKS') {
      return `${c.host}${c.httpPath || ''}`;
    }
    return `${c.host}:${c.port}`;
  }
}
