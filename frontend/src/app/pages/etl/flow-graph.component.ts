import {
  AfterViewInit,
  Component,
  ElementRef,
  EventEmitter,
  Input,
  NgZone,
  OnChanges,
  OnDestroy,
  Output,
  ViewChild,
} from '@angular/core';
import { mountFlow, type FlowHandle } from './flow-graph';

@Component({
  selector: 'app-flow-graph',
  standalone: true,
  template: `<div #host class="flow-host"></div>`,
  styles: [
    `
      :host {
        display: block;
        width: 100%;
        height: 100%;
      }
      .flow-host {
        width: 100%;
        height: 100%;
      }
    `,
  ],
})
export class FlowGraphComponent implements AfterViewInit, OnChanges, OnDestroy {
  @Input() nodes: any[] = [];
  @Input() edges: any[] = [];
  @Input() selectedId: string | null = null;
  @Output() nodeClick = new EventEmitter<{ id: string; data: any }>();

  @ViewChild('host', { static: true }) host!: ElementRef<HTMLElement>;

  private handle?: FlowHandle;

  constructor(private zone: NgZone) {}

  ngAfterViewInit(): void {
    this.render();
  }

  ngOnChanges(): void {
    if (this.handle) this.render();
  }

  ngOnDestroy(): void {
    this.handle?.destroy();
  }

  private render(): void {
    const props = {
      nodes: this.nodes || [],
      edges: this.edges || [],
      selectedId: this.selectedId,
      onNodeClick: (id: string, data: any) =>
        this.zone.run(() => this.nodeClick.emit({ id, data })),
    };
    this.zone.runOutsideAngular(() => {
      if (!this.handle) {
        this.handle = mountFlow(this.host.nativeElement, props);
      } else {
        this.handle.update(props);
      }
    });
  }
}
