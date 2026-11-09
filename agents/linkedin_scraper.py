"""
Reveal VoC — LinkedIn public-page scraper (Playwright, no credentials).

This is deterministic AUTOMATION, not an agent: no LLM, no reasoning. It
drives a headless Chromium session against pages LinkedIn serves to
logged-out visitors and returns normalised items in the same shape the Go
`voc.Item` contract expects.

What is reachable without a LinkedIn session (verified Sep 2026):

  1. Company page            https://www.linkedin.com/company/{handle}
     ~10 most recent posts/reposts by the organisation. Each <article>
     carries data-activity-urn; the activity id encodes the publish time
     (top 41 bits = epoch millis) so dating is exact.

  2. Public post page        https://www.linkedin.com/feed/update/urn:li:activity:{id}
     Includes a schema.org SocialMediaPosting JSON-LD block: body text,
     author, datePublished, like/comment counts and the visible comments
     (text, author, datePublished).

  3. Third-party posts that mention / hashtag the company are discovered via
     a web search engine (Brave) restricted to linkedin.com/posts. The
     activity id sits in every post URL, so results are date-filtered before
     the page is ever loaded. This leg is best-effort: search indexes lag
     LinkedIn by days to weeks and rate-limit quickly, so for short windows
     most in-range signals come from legs 1 and 2.

What is NOT reachable logged-out: /company/{handle}/posts (auth wall),
LinkedIn search, hashtag feeds, full comment threads beyond the first page.

Rate limiting: LinkedIn starts redirecting to /authwall after a burst of page
loads from one IP. The scraper throttles itself, caps pages per run and
stops cleanly when it hits the wall, reporting how far it got.
"""

from __future__ import annotations

import hashlib
import json
import random
import re
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional, Set, Tuple
from urllib.parse import quote_plus

USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
)
LINKEDIN = "https://www.linkedin.com"
ACTIVITY_RE = re.compile(r"activity[-:](\d{17,20})")
URN_RE = re.compile(r"urn:li:activity:(\d{17,20})")
HASHTAG_RE = re.compile(r"#(\w+)")
LDJSON_RE = re.compile(r'<script type="application/ld\+json">(.*?)</script>', re.S)

# Guardrails — keep runs polite and bounded.
MAX_POST_PAGES_HARD = 60
MIN_DELAY_S, MAX_DELAY_S = 1.2, 2.8
PAGE_TIMEOUT_MS = 40_000


# ---------------------------------------------------------------------------
# small helpers
# ---------------------------------------------------------------------------

def activity_time(activity_id: str) -> datetime:
    """LinkedIn activity ids are Snowflake-like: top 41 bits are epoch millis."""
    return datetime.fromtimestamp((int(activity_id) >> 22) / 1000.0, tz=timezone.utc)


def parse_iso(s: str) -> Optional[datetime]:
    if not s:
        return None
    try:
        d = datetime.fromisoformat(s.replace("Z", "+00:00"))
        return d if d.tzinfo else d.replace(tzinfo=timezone.utc)
    except ValueError:
        return None


def iso(d: Optional[datetime]) -> str:
    return d.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ") if d else ""


def handle_from_url(u: str) -> str:
    m = re.search(r"linkedin\.com/(?:in|company|showcase|school)/([^/?#]+)", u or "")
    return m.group(1).lower() if m else ""


def author_type_from_url(u: str) -> str:
    return "ORGANIZATION" if re.search(r"linkedin\.com/(company|showcase|school)/", u or "") else "PERSON"


def hashtags(text: str) -> List[str]:
    return sorted({t.lower() for t in HASHTAG_RE.findall(text or "")})


def mentions_of(text: str, company: str, handle: str) -> List[str]:
    out: Set[str] = set()
    low = (text or "").lower()
    if company and company.lower() in low:
        out.add(handle or company.lower())
    if handle and handle.lower() in low:
        out.add(handle.lower())
    return sorted(out)


def clean(s: str) -> str:
    return re.sub(r"[ \t]+", " ", (s or "")).strip()


# ---------------------------------------------------------------------------
# scrape session
# ---------------------------------------------------------------------------

@dataclass
class ScrapeStats:
    company_posts_seen: int = 0
    company_posts_in_range: int = 0
    discovered_urls: int = 0
    discovered_in_range: int = 0
    post_pages_loaded: int = 0
    comments_in_range: int = 0
    authwalls: int = 0
    search_queries: int = 0
    errors: int = 0


@dataclass
class ScrapeResult:
    items: List[Dict[str, Any]] = field(default_factory=list)
    notes: List[str] = field(default_factory=list)
    stats: ScrapeStats = field(default_factory=ScrapeStats)

    def to_dict(self) -> Dict[str, Any]:
        return {"items": self.items, "notes": self.notes, "stats": self.stats.__dict__}


class LinkedInPublicScraper:
    def __init__(self, page, date_from: datetime, date_to: datetime, max_posts: int, stats: ScrapeStats, notes: List[str]):
        self.page = page
        self.date_from = date_from
        self.date_to = date_to
        self.max_posts = max(1, min(max_posts, MAX_POST_PAGES_HARD))
        self.stats = stats
        self.notes = notes
        self.walled = False

    # -- navigation ---------------------------------------------------------

    def _sleep(self):
        time.sleep(random.uniform(MIN_DELAY_S, MAX_DELAY_S))

    def goto(self, url: str) -> bool:
        """Load a LinkedIn URL. Returns False on auth wall / login redirect."""
        if self.walled:
            return False
        try:
            self.page.goto(url, wait_until="domcontentloaded", timeout=PAGE_TIMEOUT_MS)
            self.page.wait_for_timeout(1500)
        except Exception as e:  # noqa: BLE001
            self.stats.errors += 1
            self.notes.append(f"navigation failed for {url[:80]}: {str(e)[:120]}")
            return False
        final = self.page.url
        if "/authwall" in final or "/login" in final or "/signup" in final or "/uas/" in final:
            self.stats.authwalls += 1
            self.walled = True
            self.notes.append("LinkedIn presented its sign-in wall — stopped early to avoid rate limiting. Re-run later or narrow the date range.")
            return False
        return True

    def in_range(self, d: Optional[datetime]) -> bool:
        return d is not None and self.date_from <= d <= self.date_to

    # -- company feed -------------------------------------------------------

    def company_feed(self, handle: str, company: str) -> List[Dict[str, Any]]:
        """Recent posts/reposts on the public company page."""
        if not handle:
            return []
        if not self.goto(f"{LINKEDIN}/company/{handle}"):
            return []
        cards = self.page.locator("article[data-activity-urn]")
        n = cards.count()
        self.stats.company_posts_seen = n
        if n == 0:
            self.notes.append(f"company page for '{handle}' loaded but exposed no public posts")
            return []
        rows = cards.evaluate_all(
            """els => els.map(e => {
                const q = (s) => { const x = e.querySelector(s); return x ? x.innerText.trim() : ''; };
                const actor = e.querySelector("a[data-tracking-control-name*='feed-actor-name']");
                return {
                  urn: e.getAttribute('data-activity-urn') || '',
                  attributed: e.getAttribute('data-attributed-urn') || '',
                  text: q('p.attributed-text-segment-list__content'),
                  header: q('.main-feed-activity-card__header'),
                  actorName: actor ? actor.innerText.trim() : '',
                  actorUrl: actor ? actor.getAttribute('href') || '' : '',
                  reactions: q("[data-test-id='social-actions__reaction-count']"),
                  comments: q("[data-test-id='social-actions__comments']"),
                  when: q('time'),
                };
            })"""
        )
        out = []
        for r in rows:
            m = URN_RE.search(r.get("urn", ""))
            if not m:
                continue
            aid = m.group(1)
            published = activity_time(aid)
            kind = "REPOST" if "reposted" in (r.get("header") or "").lower() else "POST"
            actor_url = r.get("actorUrl") or ""
            item = {
                "externalId": f"urn:li:activity:{aid}",
                "kind": kind,
                "authorName": clean(r.get("actorName")) or company,
                "authorHandle": handle_from_url(actor_url) or handle,
                "authorType": author_type_from_url(actor_url) if actor_url else "ORGANIZATION",
                "content": clean(r.get("text")),
                "url": f"{LINKEDIN}/feed/update/urn:li:activity:{aid}",
                "hashtags": hashtags(r.get("text")),
                "mentions": mentions_of(r.get("text"), company, handle),
                "likeCount": _int(r.get("reactions")),
                "commentCount": _int(r.get("comments")),
                "shareCount": 0,
                "publishedAt": iso(published),
                "raw": {"source": "company_page", "activityId": aid, "attributedUrn": r.get("attributed"), "relative": r.get("when"), "header": r.get("header")},
            }
            out.append(item)
        return out

    # -- discovery ----------------------------------------------------------

    # Web-index discovery is best-effort: Brave serves one or two queries per
    # session before an interstitial, ignores freshness parameters, and its
    # index skews old; Bing does not surface post URLs. Keep it cheap.
    MAX_SEARCH_QUERIES = 3

    def discover(self, company: str, handle: str, tags: List[str]) -> List[Tuple[str, str]]:
        """Return [(activity_id, url)] of third-party posts naming the company, within range."""
        queries: List[str] = []
        if company:
            queries.append(f'site:linkedin.com/posts "{company}"')
        for t in tags:
            t = t.lstrip("#")
            if t and t.lower() != (company or "").lower().replace(" ", ""):
                queries.append(f'site:linkedin.com/posts "#{t}"')
        if handle and handle.lower() != (company or "").lower().replace(" ", ""):
            queries.append(f'site:linkedin.com/posts "{handle}"')
        queries = queries[: self.MAX_SEARCH_QUERIES]

        found: Dict[str, str] = {}
        blocked = False
        for q in queries:
            urls = self._search(q)
            if urls is None:
                blocked = True
                break
            for u in urls:
                m = ACTIVITY_RE.search(u)
                if m:
                    found.setdefault(m.group(1), u.split("?")[0])
            time.sleep(random.uniform(3.0, 5.0))  # be gentle with the search engine
        self.stats.discovered_urls = len(found)
        in_range = [(aid, u) for aid, u in found.items() if self.in_range(activity_time(aid))]
        in_range.sort(key=lambda x: int(x[0]), reverse=True)
        self.stats.discovered_in_range = len(in_range)
        if not queries:
            self.notes.append("no company name or hashtags to search for — only the company page was scanned")
        elif blocked and not found:
            self.notes.append("web-index discovery of third-party posts was rate-limited this run; comments on the company's own posts were still collected")
        elif found and not in_range:
            self.notes.append(f"web index knows {len(found)} third-party posts naming the company but none inside the window (indexes lag by days/weeks)")
        return in_range

    def _search(self, q: str) -> Optional[List[str]]:
        """One Brave Search query → LinkedIn post URLs. None = blocked/failed."""
        self.stats.search_queries += 1
        try:
            url = f"https://search.brave.com/search?q={quote_plus(q)}&source=web"
            self.page.goto(url, wait_until="domcontentloaded", timeout=PAGE_TIMEOUT_MS)
            self.page.wait_for_timeout(2500)
            hrefs = self.page.locator("a[href*='linkedin.com/posts/']").evaluate_all("els => els.map(e => e.href)")
            urls = [h for h in hrefs if h and "linkedin.com/posts/" in h]
            if not urls:
                # A results page echoes the query in its title; the interstitial does not.
                title = self.page.title() or ""
                if q.split('"')[0].strip()[:12] not in title and "brave" in title.lower():
                    return None
            return urls
        except Exception as e:  # noqa: BLE001
            self.stats.errors += 1
            self.notes.append(f"search failed: {str(e)[:100]}")
            return None

    # -- post pages ---------------------------------------------------------

    def post_page(self, activity_id: str, company: str, handle: str, hint_url: str = "") -> Tuple[Optional[Dict[str, Any]], List[Dict[str, Any]]]:
        """Load one public post and return (post_item, comment_items) — all normalised, undated ones dropped."""
        url = f"{LINKEDIN}/feed/update/urn:li:activity:{activity_id}"
        if not self.goto(url):
            return None, []
        self.stats.post_pages_loaded += 1
        html = self.page.content()
        ld = _ld_posting(html)
        self._sleep()
        if not ld:
            self.notes.append(f"post {activity_id}: page had no structured data; skipped")
            return None, []

        author = ld.get("author") or ld.get("creator") or {}
        if isinstance(author, list):
            author = author[0] if author else {}
        if not isinstance(author, dict):
            author = {"name": str(author)}
        author_url = author.get("url") or ""
        author_type = "ORGANIZATION" if author.get("@type") == "Organization" else author_type_from_url(author_url)
        text = _ld_text(ld)
        published = parse_iso(ld.get("datePublished") or ld.get("uploadDate")) or activity_time(activity_id)
        likes, comments_n = 0, int(ld.get("commentCount") or 0)
        for st in ld.get("interactionStatistic") or []:
            if isinstance(st, dict):
                t = (st.get("interactionType") or "").lower()
                if "like" in t:
                    likes = int(st.get("userInteractionCount") or 0)
                elif "comment" in t:
                    comments_n = int(st.get("userInteractionCount") or comments_n)

        post = {
            "externalId": f"urn:li:activity:{activity_id}",
            "kind": "POST",
            "authorName": clean(author.get("name")) or "Unknown",
            "authorHandle": handle_from_url(author_url),
            "authorType": author_type,
            "content": text,
            "url": (ld.get("@id") or hint_url or url).split("?")[0],
            "hashtags": hashtags(text),
            "mentions": mentions_of(text, company, handle),
            "likeCount": likes,
            "commentCount": comments_n,
            "shareCount": 0,
            "publishedAt": iso(published),
            "raw": {"source": "post_page", "activityId": activity_id, "mediaType": ld.get("@type"), "ldjson": _slim_ld(ld)},
        }

        comments: List[Dict[str, Any]] = []
        for c in ld.get("comment") or []:
            if not isinstance(c, dict):
                continue
            ctext = clean(c.get("text") or "")
            cauthor = c.get("author") or {}
            if not isinstance(cauthor, dict):
                cauthor = {"name": str(cauthor)}
            cdate = parse_iso(c.get("datePublished"))
            if not ctext or not self.in_range(cdate):
                continue
            cid = hashlib.sha1(f"{activity_id}|{cauthor.get('name','')}|{ctext}".encode("utf-8")).hexdigest()[:20]
            clikes = 0
            cst = c.get("interactionStatistic")
            if isinstance(cst, dict):
                clikes = int(cst.get("userInteractionCount") or 0)
            comments.append({
                "externalId": f"urn:li:comment:{activity_id}:{cid}",
                "kind": "COMMENT",
                "parentExternalId": post["externalId"],
                "authorName": clean(cauthor.get("name")) or "LinkedIn member",
                "authorHandle": handle_from_url(cauthor.get("url") or ""),
                "authorType": "ORGANIZATION" if cauthor.get("@type") == "Organization" else "PERSON",
                "content": ctext,
                "url": post["url"],
                "hashtags": hashtags(ctext),
                "mentions": sorted(set(mentions_of(ctext, company, handle)) | ({handle} if handle else set())),
                "likeCount": clikes,
                "commentCount": 0,
                "shareCount": 0,
                "publishedAt": iso(cdate),
                "raw": {"source": "post_page", "activityId": activity_id},
            })
        self.stats.comments_in_range += len(comments)
        return post, comments


def _int(s: Any) -> int:
    digits = re.sub(r"[^\d]", "", str(s or ""))
    return int(digits) if digits else 0


def _ld_posting(html: str) -> Optional[Dict[str, Any]]:
    """
    The post's schema.org block. Text posts are SocialMediaPosting; video and
    image posts are VideoObject / ImageObject with the same comment array.
    Anything carrying a datePublished or a comment list is accepted.
    """
    for block in LDJSON_RE.findall(html):
        try:
            d = json.loads(block)
        except json.JSONDecodeError:
            continue
        cands = d if isinstance(d, list) else [d]
        for c in cands:
            if not isinstance(c, dict):
                continue
            if c.get("@type") in ("SocialMediaPosting", "DiscussionForumPosting", "Article", "VideoObject", "ImageObject") \
                    or "comment" in c or "datePublished" in c:
                return c
    return None


def _ld_text(ld: Dict[str, Any]) -> str:
    for k in ("articleBody", "text", "description", "headline", "name", "caption"):
        v = ld.get(k)
        if isinstance(v, str) and v.strip():
            return clean(v)
    return ""


def _slim_ld(ld: Dict[str, Any]) -> Dict[str, Any]:
    """Keep the raw structured data but drop bulky/duplicated fields."""
    return {k: v for k, v in ld.items() if k not in ("comment", "articleBody", "text", "hasPart", "@context")}


# ---------------------------------------------------------------------------
# entry point
# ---------------------------------------------------------------------------

def scrape(
    handle: str,
    company: str,
    hashtags_: Optional[List[str]] = None,
    date_from: Optional[datetime] = None,
    date_to: Optional[datetime] = None,
    max_posts: int = 25,
    include_company_posts: bool = True,
    include_discovery: bool = True,
) -> Dict[str, Any]:
    """
    Scrape public LinkedIn signals for one company inside [date_from, date_to].

    Returns {"items": [...voc.Item...], "notes": [...], "stats": {...}}.
    Synchronous (blocks on the browser); callers on an event loop should run
    it in a worker thread.
    """
    from playwright.sync_api import sync_playwright  # imported lazily: heavy

    now = datetime.now(timezone.utc)
    date_to = (date_to or now).astimezone(timezone.utc)
    date_from = (date_from or (date_to - timedelta(days=5))).astimezone(timezone.utc)
    if date_from > date_to:
        date_from, date_to = date_to, date_from
    handle = (handle or "").strip().strip("/").lower()
    company = (company or "").strip()
    tags = [t.strip().lstrip("#") for t in (hashtags_ or []) if t and t.strip()]

    result = ScrapeResult()
    st, notes = result.stats, result.notes
    items: Dict[str, Dict[str, Any]] = {}

    with sync_playwright() as pw:
        browser = pw.chromium.launch(headless=True)
        ctx = browser.new_context(
            user_agent=USER_AGENT,
            locale="en-US",
            viewport={"width": 1280, "height": 900},
            extra_http_headers={"Accept-Language": "en-US,en;q=0.9"},
        )
        ctx.add_init_script("Object.defineProperty(navigator,'webdriver',{get:()=>undefined});")
        page = ctx.new_page()
        s = LinkedInPublicScraper(page, date_from, date_to, max_posts, st, notes)
        try:
            # 1) Company's own feed (broadcast signals + the posts customers comment on).
            feed_posts: List[Dict[str, Any]] = []
            if include_company_posts and handle:
                feed_posts = s.company_feed(handle, company)
                for p in feed_posts:
                    if s.in_range(parse_iso(p["publishedAt"])):
                        items[p["externalId"]] = p
                        st.company_posts_in_range += 1

            # 2) Third-party posts naming the company (already date-filtered by activity id).
            discovered = s.discover(company, handle, tags) if include_discovery else []

            # 3) Post pages: comments live here. Budget = max_posts pages.
            #    Priority: in-range third-party posts, then company posts with comments
            #    (including slightly older ones — their comments may fall in range).
            queue: List[Tuple[str, str]] = list(discovered)
            lookback = date_from - timedelta(days=30)
            for p in sorted(feed_posts, key=lambda x: x["publishedAt"], reverse=True):
                pub = parse_iso(p["publishedAt"])
                if p["commentCount"] > 0 and pub and lookback <= pub <= date_to:
                    queue.append((p["raw"]["activityId"], p["url"]))
            seen: Set[str] = set()
            budget = s.max_posts
            for aid, url in queue:
                if aid in seen or budget <= 0 or s.walled:
                    continue
                seen.add(aid)
                budget -= 1
                post, comments = s.post_page(aid, company, handle, url)
                if post is None:
                    continue
                pub = parse_iso(post["publishedAt"])
                if s.in_range(pub):
                    items[post["externalId"]] = _merge(items.get(post["externalId"]), post)
                elif comments:
                    # parent kept for context of in-range comments
                    post["raw"]["outOfRangeParent"] = True
                    items[post["externalId"]] = _merge(items.get(post["externalId"]), post)
                for c in comments:
                    items[c["externalId"]] = c
            if budget <= 0 and len(queue) > len(seen):
                notes.append(f"page budget of {s.max_posts} reached; {len(queue) - len(seen)} candidate posts not opened")
        finally:
            browser.close()

    result.items = sorted(items.values(), key=lambda x: x["publishedAt"], reverse=True)
    notes.insert(0, (
        f"window {date_from.date()} → {date_to.date()}: company posts {st.company_posts_in_range}/{st.company_posts_seen} in range, "
        f"discovered {st.discovered_in_range}/{st.discovered_urls} third-party posts, opened {st.post_pages_loaded} pages, "
        f"{st.comments_in_range} comments in range"
    ))
    return result.to_dict()


def _merge(old: Optional[Dict[str, Any]], new: Dict[str, Any]) -> Dict[str, Any]:
    """Prefer the richer post-page record but keep feed-only fields (e.g. REPOST kind)."""
    if not old:
        return new
    merged = dict(old)
    for k, v in new.items():
        if v not in (None, "", 0, [], {}):
            merged[k] = v
    if old.get("kind") == "REPOST":
        merged["kind"] = "REPOST"
    merged["raw"] = {**(old.get("raw") or {}), **(new.get("raw") or {})}
    return merged


if __name__ == "__main__":  # manual smoke test: python linkedin_scraper.py microsoft "Microsoft" 5
    import sys

    h = sys.argv[1] if len(sys.argv) > 1 else "microsoft"
    c = sys.argv[2] if len(sys.argv) > 2 else h.title()
    d = int(sys.argv[3]) if len(sys.argv) > 3 else 5
    out = scrape(h, c, [], datetime.now(timezone.utc) - timedelta(days=d), None, max_posts=6)
    print(json.dumps(out["stats"], indent=1))
    print("\n".join(out["notes"]))
    for it in out["items"][:12]:
        print(f"- {it['kind']:7} {it['publishedAt']} {it['authorName'][:24]:24} {it['content'][:90]!r}")
    print("total items:", len(out["items"]))
