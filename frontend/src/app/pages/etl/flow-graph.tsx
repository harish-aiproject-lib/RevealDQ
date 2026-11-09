import { createRoot, type Root } from 'react-dom/client';
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
  Handle,
  Position,
  type Node,
  type Edge,
  type NodeProps,
} from '@xyflow/react';

type Kind = 'source' | 'transform' | 'target';

const KIND_COLORS: Record<Kind, { bg: string; border: string; ink: string }> = {
  source: { bg: '#ecfeff', border: '#06b6d4', ink: '#0e7490' },
  transform: { bg: '#fff7ed', border: '#f97316', ink: '#c2410c' },
  target: { bg: '#ecfdf5', border: '#10b981', ink: '#047857' },
};

function EtlNode({ data, selected }: NodeProps) {
  const d = data as any;
  const kind: Kind = (['source', 'transform', 'target'].includes(d.kind) ? d.kind : 'transform') as Kind;
  const c = KIND_COLORS[kind];
  return (
    <div
      style={{
        minWidth: 160,
        maxWidth: 220,
        borderRadius: 12,
        border: `2px solid ${c.border}`,
        background: c.bg,
        padding: '8px 10px',
        boxShadow: selected ? `0 0 0 3px ${c.border}55` : '0 1px 3px rgba(0,0,0,.12)',
        fontFamily: 'inherit',
        cursor: 'pointer',
      }}
    >
      <Handle type="target" position={Position.Left} style={{ background: c.border, border: 'none' }} />
      <div
        style={{
          fontSize: 9,
          fontWeight: 700,
          letterSpacing: '.06em',
          textTransform: 'uppercase',
          color: c.ink,
        }}
      >
        {kind}
      </div>
      <div style={{ fontWeight: 700, fontSize: 13, color: '#101828', wordBreak: 'break-word' }}>
        {d.label}
      </div>
      <div style={{ fontSize: 11, color: '#475467' }}>{d.ntype}</div>
      <Handle type="source" position={Position.Right} style={{ background: c.border, border: 'none' }} />
    </div>
  );
}

const nodeTypes = { etl: EtlNode };

export interface FlowProps {
  nodes: Node[];
  edges: Edge[];
  selectedId?: string | null;
  onNodeClick?: (id: string, data: any) => void;
}

function Flow({ nodes, edges, selectedId, onNodeClick }: FlowProps) {
  const decorated = nodes.map((n) => ({ ...n, selected: n.id === selectedId }));
  const styledEdges = edges.map((e) => ({
    ...e,
    style: { stroke: '#94a3b8', strokeWidth: 1.5 },
    markerEnd: { type: 'arrowclosed' as any, color: '#94a3b8' },
  }));
  return (
    <div style={{ width: '100%', height: '100%' }}>
      <ReactFlowProvider>
        <ReactFlow
          nodes={decorated}
          edges={styledEdges}
          nodeTypes={nodeTypes}
          fitView
          minZoom={0.15}
          maxZoom={2}
          nodesConnectable={false}
          onNodeClick={(_e, node) => onNodeClick?.(node.id, node.data)}
        >
          <Background gap={18} color="#e2e8f0" />
          <Controls showInteractive={false} />
          <MiniMap
            pannable
            zoomable
            nodeColor={(n) => {
              const k = (n.data as any)?.kind as Kind;
              return (KIND_COLORS[k] || KIND_COLORS.transform).border;
            }}
          />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}

export interface FlowHandle {
  update(props: FlowProps): void;
  destroy(): void;
}

export function mountFlow(el: HTMLElement, props: FlowProps): FlowHandle {
  const root: Root = createRoot(el);
  const render = (p: FlowProps) => root.render(<Flow {...p} />);
  render(props);
  return {
    update: (p) => render(p),
    destroy: () => root.unmount(),
  };
}
