import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { ApiService } from '../../core/api.service';
import { IconComponent } from '../../shared/icon.component';

@Component({
  selector: 'app-settings',
  standalone: true,
  imports: [FormsModule, IconComponent],
  templateUrl: './settings.component.html',
  styleUrl: './settings.component.scss',
})
export class SettingsComponent implements OnInit {
  wsId = 0;
  acl = signal<any[]>([]);
  message = '';

  constructor(
    private route: ActivatedRoute,
    private api: ApiService,
  ) {}

  ngOnInit(): void {
    this.wsId = Number(this.route.parent?.snapshot.paramMap.get('id'));
    this.reload();
  }

  reload(): void {
    this.api.getAcl(this.wsId).subscribe({ next: (rows) => this.acl.set(rows || []) });
  }

  toggle(row: any, field: 'canView' | 'canEdit'): void {
    const body = {
      screenAclId: row.screenAclId,
      canView: field === 'canView' ? !row.canView : row.canView,
      canEdit: field === 'canEdit' ? !row.canEdit : row.canEdit,
    };
    this.api.updateAcl(this.wsId, body).subscribe({
      next: () => {
        this.message = 'Access updated.';
        this.reload();
      },
    });
  }
}
