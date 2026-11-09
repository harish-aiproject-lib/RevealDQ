/**
 * Shared presentation helpers for the Reveal VoC pages.
 */

export type Sentiment = 'POSITIVE' | 'NEGATIVE' | 'NEUTRAL' | 'BROADCAST' | '';

export const SENTIMENTS: { code: Sentiment; label: string; icon: string; css: string }[] = [
  { code: 'POSITIVE', label: 'Happy', icon: 'smile', css: 'pos' },
  { code: 'NEGATIVE', label: 'Negative', icon: 'frown', css: 'neg' },
  { code: 'NEUTRAL', label: 'Neutral', icon: 'wave', css: 'neu' },
  { code: 'BROADCAST', label: 'Broadcast', icon: 'megaphone', css: 'bro' },
];

export const CATEGORIES = [
  'SUPPORT', 'RELIABILITY', 'PRODUCT', 'BILLING', 'PRICING', 'DATA_QUALITY', 'ONBOARDING',
  'PEOPLE', 'HIRING', 'EVENT', 'ANNOUNCEMENT', 'QUESTION', 'OTHER',
];

export function sentimentMeta(code: string | null | undefined) {
  return SENTIMENTS.find((s) => s.code === (code || '').toUpperCase()) || {
    code: '' as Sentiment, label: 'Pending', icon: 'clock', css: 'pending',
  };
}

export function titleCase(s: string | null | undefined): string {
  return (s || '')
    .toLowerCase()
    .split('_')
    .filter(Boolean)
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join(' ');
}

export function relTime(iso: string | null | undefined): string {
  if (!iso) return '—';
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return '—';
  const diff = Date.now() - t;
  const m = Math.round(diff / 60000);
  if (m < 1) return 'just now';
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h}h ago`;
  const d = Math.round(h / 24);
  if (d < 30) return `${d}d ago`;
  return new Date(iso).toLocaleDateString();
}

export function pct(n: number, total: number): number {
  return total ? Math.round((n / total) * 100) : 0;
}

/** Minimal, safe markdown → HTML (Angular sanitises the bound result). */
export function mdToHtml(md: string): string {
  if (!md) return '';
  const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  const inline = (s: string) =>
    esc(s)
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/_([^_]+)_/g, '<em>$1</em>');
  const out: string[] = [];
  let list: 'ul' | 'ol' | null = null;
  const close = () => {
    if (list) {
      out.push(`</${list}>`);
      list = null;
    }
  };
  for (const raw of md.split('\n')) {
    const line = raw.trimEnd();
    if (!line.trim()) {
      close();
      continue;
    }
    let m: RegExpMatchArray | null;
    if ((m = line.match(/^#{1,3}\s+(.*)$/))) {
      close();
      out.push(`<h4>${inline(m[1])}</h4>`);
    } else if ((m = line.match(/^\s*[-*]\s+(.*)$/))) {
      if (list !== 'ul') {
        close();
        list = 'ul';
        out.push('<ul>');
      }
      out.push(`<li>${inline(m[1])}</li>`);
    } else if ((m = line.match(/^\s*\d+[.)]\s+(.*)$/))) {
      if (list !== 'ol') {
        close();
        list = 'ol';
        out.push('<ol>');
      }
      out.push(`<li>${inline(m[1])}</li>`);
    } else {
      close();
      out.push(`<p>${inline(line)}</p>`);
    }
  }
  close();
  return out.join('');
}

// ---------------------------------------------------------------------------
// Scrape window (date range) helpers
// ---------------------------------------------------------------------------

export interface DateRange {
  from: string; // YYYY-MM-DD, inclusive
  to: string; // YYYY-MM-DD, inclusive
}

/** Local calendar date as YYYY-MM-DD. */
export function ymd(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Inclusive range covering the last `days` days ending today (default 5). */
export function lastDays(days = 5): DateRange {
  const to = new Date();
  const from = new Date(to);
  from.setDate(to.getDate() - (days - 1));
  return { from: ymd(from), to: ymd(to) };
}

export const RANGE_PRESETS: { label: string; days: number }[] = [
  { label: 'Last 5 days', days: 5 },
  { label: 'Last 7 days', days: 7 },
  { label: 'Last 14 days', days: 14 },
  { label: 'Last 30 days', days: 30 },
];

/** Number of whole days in an inclusive range (0 when invalid). */
export function rangeDays(r: DateRange): number {
  const a = Date.parse(r.from);
  const b = Date.parse(r.to);
  if (isNaN(a) || isNaN(b) || b < a) return 0;
  return Math.round((b - a) / 86400000) + 1;
}

/** Human summary of a sync result, including scraper notes. */
export function syncSummary(r: any): string {
  const parts = [`${r.provider || 'sync'}: ${r.itemsFetched} fetched · ${r.itemsNew} new · ${r.itemsAnalysed} analysed · ${r.actionsCreated} actions`];
  if (r.windowFrom && r.windowTo) parts.push(`window ${r.windowFrom.slice(0, 10)} → ${r.windowTo.slice(0, 10)}`);
  return parts.join(' · ');
}
