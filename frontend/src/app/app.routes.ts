import { Routes } from '@angular/router';
import { authGuard } from './core/auth.guard';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'login' },
  {
    path: 'login',
    loadComponent: () => import('./pages/login/login.component').then((m) => m.LoginComponent),
  },
  {
    path: 'workspaces',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./pages/workspaces/workspaces.component').then((m) => m.WorkspacesComponent),
  },
  {
    path: 'workspaces/:id',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./pages/workspace-shell/workspace-shell.component').then(
        (m) => m.WorkspaceShellComponent,
      ),
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'management/analyser' },
      {
        path: 'management/analyser',
        loadComponent: () =>
          import('./pages/analyser/analyser.component').then((m) => m.AnalyserComponent),
      },
      {
        path: 'management/rules',
        loadComponent: () => import('./pages/rules/rules.component').then((m) => m.RulesComponent),
      },
      {
        path: 'management/etl',
        loadComponent: () => import('./pages/stub/stub.component').then((m) => m.StubComponent),
        data: { title: 'ETL Analyser Agent', message: 'Coming soon — stub for MVP.' },
      },
      {
        path: 'management/informatica',
        loadComponent: () => import('./pages/stub/stub.component').then((m) => m.StubComponent),
        data: {
          title: 'Informatica DQ Conversion Agent',
          message: 'Coming soon — stub for MVP.',
        },
      },
      {
        path: 'operations/dashboard',
        loadComponent: () =>
          import('./pages/dashboard/dashboard.component').then((m) => m.DashboardComponent),
      },
      {
        path: 'operations/pipelines',
        loadComponent: () =>
          import('./pages/pipelines/pipelines.component').then((m) => m.PipelinesComponent),
      },
      {
        path: 'config',
        loadComponent: () =>
          import('./pages/config/config.component').then((m) => m.ConfigComponent),
      },
      {
        path: 'settings',
        loadComponent: () =>
          import('./pages/settings/settings.component').then((m) => m.SettingsComponent),
      },
    ],
  },
  { path: '**', redirectTo: 'login' },
];
