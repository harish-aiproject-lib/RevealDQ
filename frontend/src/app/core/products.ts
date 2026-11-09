/**
 * Reality Reveal — product-module registry.
 *
 * The platform hosts multiple "Reveal" products. A project (workspace) is created
 * for exactly one product, and everything module-specific in the UI — navigation,
 * default landing route, accent colour, marketing copy — is driven from here.
 *
 * The backend owns the authoritative catalog (`GET /products`, table
 * `product_modules`) including each module's availability status. This file
 * owns the *presentation* of each module so the API stays UI-agnostic.
 *
 * Adding a product = add one entry here + routes under `pages/<module>/` + a
 * `product_modules` row (backend/internal/db/schema.go).
 */

export type ProductCode = 'DQ' | 'VOC';
export type ProductStatus = 'ACTIVE' | 'PREVIEW' | 'COMING_SOON';

export interface ProductNavItem {
  label: string;
  icon: string;
  route: string; // relative to /workspaces/:id
  /** ACL screen code used by Settings → Security & access. */
  screen?: string;
}

export interface ProductNavSection {
  label: string;
  items: ProductNavItem[];
}

export interface ProductModule {
  code: ProductCode;
  /** Full product name, e.g. "Reveal DQ". */
  name: string;
  /** Short suffix rendered after "Reveal", e.g. "DQ" / "VoC". */
  short: string;
  /** One-line positioning statement. */
  tagline: string;
  /** Longer description for landing / picker cards. */
  description: string;
  icon: string;
  /** CSS accent hue name — maps to `.product-<accent>` utilities in styles.scss. */
  accent: 'cyan' | 'violet';
  /** Where the shell lands when a workspace is opened. */
  defaultRoute: string;
  nav: ProductNavSection[];
  /** Compact-ribbon icons (subset of nav). */
  quickNav: ProductNavItem[];
  /** Headline capabilities for marketing cards. */
  highlights: { icon: string; title: string; text: string }[];
}

/** From the API catalog (`GET /products`). */
export interface ProductCatalogEntry {
  productCode: ProductCode | string;
  productName: string;
  shortName: string;
  tagline: string;
  status: ProductStatus;
  sortOrder: number;
}

const COMMON_TAIL: ProductNavSection[] = [
  {
    label: 'Config',
    items: [{ label: 'Data sources', icon: 'database', route: 'config', screen: 'SCREEN_CONFIG' }],
  },
  {
    label: 'Settings',
    items: [{ label: 'Security & access', icon: 'settings', route: 'settings', screen: 'SCREEN_SETTINGS' }],
  },
];

export const PRODUCTS: Record<ProductCode, ProductModule> = {
  DQ: {
    code: 'DQ',
    name: 'Reveal DQ',
    short: 'DQ',
    tagline: 'The true health of enterprise data — reveal the reality before you migrate.',
    description:
      'Agentic data quality. Converse with your warehouse through MCP, turn findings into governed DQ rules, and run them as executable pipelines.',
    icon: 'shield',
    accent: 'cyan',
    defaultRoute: 'management/analyser',
    nav: [
      {
        label: 'Management',
        items: [
          { label: 'DQ Analyser Agent', icon: 'agent', route: 'management/analyser', screen: 'SCREEN_ANALYSER' },
          { label: 'DQ Rules Engine', icon: 'shield', route: 'management/rules', screen: 'SCREEN_RULES' },
          { label: 'ETL Analyser', icon: 'flow', route: 'management/etl', screen: 'SCREEN_ETL' },
          {
            label: 'ETL Code Conversion Agent',
            icon: 'convert',
            route: 'management/conversion',
            screen: 'SCREEN_INFORMATICA',
          },
        ],
      },
      {
        label: 'Operations',
        items: [
          { label: 'Dashboard', icon: 'gauge', route: 'operations/dashboard', screen: 'SCREEN_DASHBOARD' },
          { label: 'DQ Pipelines', icon: 'pipeline', route: 'operations/pipelines', screen: 'SCREEN_PIPELINES' },
        ],
      },
      ...COMMON_TAIL,
    ],
    quickNav: [
      { label: 'DQ Analyser Agent', icon: 'agent', route: 'management/analyser' },
      { label: 'DQ Rules Engine', icon: 'shield', route: 'management/rules' },
      { label: 'Dashboard', icon: 'gauge', route: 'operations/dashboard' },
      { label: 'DQ Pipelines', icon: 'pipeline', route: 'operations/pipelines' },
      { label: 'Data sources', icon: 'database', route: 'config' },
      { label: 'Security & access', icon: 'settings', route: 'settings' },
    ],
    highlights: [
      {
        icon: 'agent',
        title: 'Conversational analysis',
        text: 'Ask the DQ Analyser Agent about your data in plain language. It queries schema and rows through MCP and answers with evidence.',
      },
      {
        icon: 'shield',
        title: 'Governed rule lifecycle',
        text: 'Every recommendation ships with business and technical definitions, impacted columns and a SQL check — draft, approve, activate.',
      },
      {
        icon: 'chart',
        title: 'Executable pipelines',
        text: 'Stored-procedure jobs validate the warehouse on demand and stream pass / fail statistics to the operations dashboard.',
      },
    ],
  },

  VOC: {
    code: 'VOC',
    name: 'Reveal VoC',
    short: 'VoC',
    tagline: 'Voice of Customer — reveal what customers are really telling you.',
    description:
      'Listen to what customers say about your companies on LinkedIn. Automated extraction stores every post and comment as raw structured data; agents classify sentiment, surface themes and create the actions that matter.',
    icon: 'mic',
    accent: 'violet',
    defaultRoute: 'voc/overview',
    nav: [
      {
        label: 'Voice of Customer',
        items: [
          { label: 'Overview', icon: 'mic', route: 'voc/overview', screen: 'SCREEN_VOC_HOME' },
          { label: 'Companies', icon: 'building', route: 'voc/companies', screen: 'SCREEN_VOC_HOME' },
          { label: 'Signals', icon: 'inbox', route: 'voc/signals', screen: 'SCREEN_VOC_HOME' },
          { label: 'Actions', icon: 'flag', route: 'voc/actions', screen: 'SCREEN_VOC_HOME' },
          { label: 'VoC Analyser Agent', icon: 'agent', route: 'voc/analyser', screen: 'SCREEN_VOC_HOME' },
        ],
      },
      {
        label: 'Automation',
        items: [{ label: 'Sources & sync', icon: 'refresh', route: 'voc/sources', screen: 'SCREEN_VOC_HOME' }],
      },
      ...COMMON_TAIL,
    ],
    quickNav: [
      { label: 'Overview', icon: 'mic', route: 'voc/overview' },
      { label: 'Companies', icon: 'building', route: 'voc/companies' },
      { label: 'Signals', icon: 'inbox', route: 'voc/signals' },
      { label: 'Actions', icon: 'flag', route: 'voc/actions' },
      { label: 'VoC Analyser Agent', icon: 'agent', route: 'voc/analyser' },
      { label: 'Sources & sync', icon: 'refresh', route: 'voc/sources' },
      { label: 'Security & access', icon: 'settings', route: 'settings' },
    ],
    highlights: [
      {
        icon: 'link',
        title: 'LinkedIn listening, automated',
        text: 'Add the companies you care about. Posts that tag the official handle or use its hashtags — and the comments under them — are extracted on a schedule and stored raw in VOC_DB.',
      },
      {
        icon: 'spark',
        title: 'Agentic sentiment & themes',
        text: 'Agents classify every signal as happy, negative, neutral or broadcast, tag the category and theme, and trace each insight back to the verbatim.',
      },
      {
        icon: 'flag',
        title: 'From signal to action',
        text: 'Negative and urgent voices are grouped into de-duplicated, prioritised actions your team can own — refreshed every cycle.',
      },
    ],
  },
};

export const PRODUCT_LIST: ProductModule[] = [PRODUCTS.DQ, PRODUCTS.VOC];

export const DEFAULT_PRODUCT: ProductCode = 'DQ';

export function productFor(code: string | null | undefined): ProductModule {
  const key = String(code || DEFAULT_PRODUCT).toUpperCase() as ProductCode;
  return PRODUCTS[key] || PRODUCTS[DEFAULT_PRODUCT];
}

/** True when a shell child route (e.g. "management/rules") belongs to the product. */
export function routeBelongsTo(product: ProductModule, childPath: string): boolean {
  const path = childPath.replace(/^\/+/, '');
  return product.nav.some((s) => s.items.some((i) => path === i.route || path.startsWith(i.route + '/')));
}
