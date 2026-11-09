import { Routes } from '@angular/router';
import { authGuard } from './core/auth.guard';
import { productGuard } from './core/product.guard';

/**
 * Reality Reveal routing.
 *
 * `/workspaces/:id` hosts a project. Its children are grouped by product module:
 *   - Reveal DQ  → management/*, operations/*
 *   - Reveal VoC → voc/*
 *   - shared     → config, settings
 *
 * `productGuard` keeps each project on its own module: opening the bare project
 * URL lands on the module's default screen, and a DQ route requested inside a
 * VoC project (or vice-versa) is redirected to the module home.
 */
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
    canActivateChild: [productGuard],
    loadComponent: () =>
      import('./pages/workspace-shell/workspace-shell.component').then(
        (m) => m.WorkspaceShellComponent,
      ),
    children: [
      // Module home: productGuard redirects to the product's default route.
      {
        path: '',
        pathMatch: 'full',
        loadComponent: () => import('./pages/stub/stub.component').then((m) => m.StubComponent),
        data: { title: 'Opening project…', message: 'Loading the product module for this project.' },
      },

      // ---- Reveal DQ ----
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
        loadComponent: () => import('./pages/etl/etl.component').then((m) => m.EtlComponent),
      },
      {
        path: 'management/conversion',
        loadComponent: () =>
          import('./pages/conversion/conversion.component').then((m) => m.ConversionComponent),
      },
      { path: 'management/informatica', pathMatch: 'full', redirectTo: 'management/conversion' },
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

      // ---- Reveal VoC ----
      {
        path: 'voc/overview',
        loadComponent: () => import('./pages/voc/voc-home.component').then((m) => m.VocHomeComponent),
      },
      {
        path: 'voc/companies',
        loadComponent: () =>
          import('./pages/voc/voc-companies.component').then((m) => m.VocCompaniesComponent),
      },
      {
        path: 'voc/signals',
        loadComponent: () =>
          import('./pages/voc/voc-signals.component').then((m) => m.VocSignalsComponent),
      },
      {
        path: 'voc/actions',
        loadComponent: () =>
          import('./pages/voc/voc-actions.component').then((m) => m.VocActionsComponent),
      },
      {
        path: 'voc/analyser',
        loadComponent: () =>
          import('./pages/voc/voc-analyser.component').then((m) => m.VocAnalyserComponent),
      },
      {
        path: 'voc/sources',
        loadComponent: () =>
          import('./pages/voc/voc-sources.component').then((m) => m.VocSourcesComponent),
      },

      // ---- Shared across modules ----
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
