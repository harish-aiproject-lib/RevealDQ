"""Reveal VoC — agentic analytics over LinkedIn signals.

The Go API extracts and stores raw posts/comments (automation). This module is
the reasoning layer it calls:

  analyse_items(...)   → sentiment (POSITIVE / NEGATIVE / NEUTRAL / BROADCAST),
                         category, themes, one-line summary, urgency,
                         suggested action — per signal.
  propose_actions(...) → de-duplicated, prioritised actions across a company's
                         analysed signals.
  answer_question(...) → conversational analyst over the stored picture.

Every function has a Claude path (when ANTHROPIC_API_KEY is set) and a
deterministic heuristic fallback so the pipeline never blocks on the LLM.
"""

from __future__ import annotations

import json
import os
import re
from collections import Counter, defaultdict
from typing import Any, Optional

ANTHROPIC_API_KEY = os.getenv("ANTHROPIC_API_KEY", "")

SENTIMENTS = ("POSITIVE", "NEGATIVE", "NEUTRAL", "BROADCAST")
CATEGORIES = (
    "SUPPORT",       # service desk, response times, tickets
    "RELIABILITY",   # outages, bugs, performance
    "PRODUCT",       # features, UX, roadmap
    "BILLING",       # invoices, charges, renewal pricing
    "PRICING",       # value for money, price rises
    "DATA_QUALITY",  # data loss, accuracy, migration data issues
    "ONBOARDING",    # implementation, migration experience
    "PEOPLE",        # account managers, staff, culture
    "HIRING",        # recruitment / careers broadcast
    "EVENT",         # webinars, summits, roundtables
    "ANNOUNCEMENT",  # releases, awards, PR
    "QUESTION",      # community asking for help / comparisons
    "OTHER",
)
URGENCIES = ("LOW", "MEDIUM", "HIGH")

# ---------------------------------------------------------------------------
# Heuristic engine (fallback / offline)
# ---------------------------------------------------------------------------

_POS = {
    "shout-out": 2, "shoutout": 2, "thank": 2, "thanks": 2, "excellent": 3, "great": 2, "love": 3,
    "impressed": 3, "smooth": 2, "recommend": 3, "happy": 2, "fantastic": 3, "amazing": 3,
    "resolved": 2, "reliable": 2, "reliability has been excellent": 4, "renewed": 2, "faster": 2,
    "down 40": 3, "partnership": 1, "listens": 2, "well done": 3, "congrats": 2, "kudos": 3,
    "seamless": 3, "outstanding": 3, "helpful": 2, "quickly": 1, "rare to see": 2, "brilliant": 3,
}
_NEG = {
    "outage": 4, "outages": 4, "down": 1, "broke": 3, "broken": 3, "double-charged": 4, "double charged": 4,
    "zero replies": 4, "no replies": 3, "no answer": 3, "no reply": 3, "not the experience": 3,
    "data loss": 4, "lost": 2, "removed": 2, "who is testing": 3, "jumped 35": 3, "no added value": 3,
    "alternatives": 2, "reconsidering": 4, "waited": 2, "queue": 1, "please fix": 3, "frustrat": 4,
    "disappoint": 4, "terrible": 4, "worst": 4, "unacceptable": 4, "escalated": 2, "still isn't": 2,
    "slipped": 2, "deal-breaker": 4, "deal breaker": 4, "seriously": 1, "rate limits changed": 3,
    "heads-up would have been nice": 3, "cancel": 3, "refund": 3, "bug": 2, "crash": 3, "slow": 2,
    "expensive": 2, "overpriced": 3, "ignored": 3, "useless": 4, "issue": 1, "problem": 1,
}
_BROADCAST = (
    "excited to announce", "we're hiring", "we are hiring", "join us", "registration is open",
    "new on the", "blog", "join our live", "webinar", "proud to be recognised", "proud to be recognized",
    "walks through", "link in comments", "summit", "we're growing", "we are growing", "release",
    "now available", "introducing", "save the date",
)
_QUESTION = ("anyone here", "how does", "what questions", "genuine question", "looking for lessons",
             "ping me", "?")

_CATEGORY_KW = {
    "SUPPORT": ("support", "ticket", "queue", "response time", "replies", "help desk", "helpdesk", "password reset"),
    "RELIABILITY": ("outage", "status page", "down ", "downtime", "broke", "crash", "rate limit", "incident", "performance", "slow"),
    "BILLING": ("billing", "invoice", "charged", "double-charged", "refund"),
    "PRICING": ("pricing", "price", "renewal pricing", "35%", "expensive", "cost", "value"),
    "DATA_QUALITY": ("data loss", "data quality", "cdr", "accuracy", "duplicates", "missing data", "feed"),
    "ONBOARDING": ("onboarding", "migrat", "implementation", "rolled out", "roll-out", "rollout", "go-live"),
    "PRODUCT": ("ui update", "export", "feature", "roadmap", "api", "integration", "release", "ux", "dashboard"),
    "PEOPLE": ("account manager", "team", "staff", "engineer", "consultant"),
    "HIRING": ("hiring", "careers", "join us", "we're growing", "recruit"),
    "EVENT": ("webinar", "summit", "roundtable", "live session", "conference", "meetup"),
    "ANNOUNCEMENT": ("announce", "recognised", "recognized", "award", "report", "blog", "introducing", "launch"),
    "QUESTION": ("anyone here", "how does", "what questions", "genuine question", "comparing", "looking for"),
}


def _score_text(text: str, table: dict[str, int]) -> int:
    t = text.lower()
    return sum(w for k, w in table.items() if k in t)


def _themes(text: str, company: str) -> list[str]:
    t = text.lower()
    found = []
    for cat, kws in _CATEGORY_KW.items():
        for kw in kws:
            if kw.strip() in t:
                found.append(kw.strip().replace(" ", "-"))
                break
    tags = re.findall(r"#(\w+)", text)
    found.extend(tag.lower() for tag in tags if tag.lower() != company.lower().replace(" ", ""))
    seen, out = set(), []
    for f in found:
        if f not in seen:
            seen.add(f)
            out.append(f)
    return out[:5]


def _category(text: str, sentiment: str) -> str:
    t = text.lower()
    best, best_hits = "OTHER", 0
    for cat, kws in _CATEGORY_KW.items():
        hits = sum(1 for kw in kws if kw.strip() in t)
        if hits > best_hits:
            best, best_hits = cat, hits
    if sentiment == "BROADCAST" and best not in ("HIRING", "EVENT", "ANNOUNCEMENT", "PRODUCT"):
        return "ANNOUNCEMENT"
    if sentiment == "NEUTRAL" and best == "OTHER" and any(q in t for q in _QUESTION):
        return "QUESTION"
    return best


def heuristic_analyse(item: dict[str, Any], company: str) -> dict[str, Any]:
    text = (item.get("content") or "").strip()
    t = text.lower()
    author_type = (item.get("authorType") or "PERSON").upper()

    pos = _score_text(text, _POS)
    neg = _score_text(text, _NEG)
    broadcast_hits = sum(1 for k in _BROADCAST if k in t)

    if author_type == "ORGANIZATION" or (broadcast_hits >= 2 and neg == 0):
        sentiment, score = "BROADCAST", 0.0
    elif neg >= pos + 2:
        sentiment = "NEGATIVE"
        score = -min(1.0, 0.35 + 0.1 * neg)
    elif pos >= neg + 2:
        sentiment = "POSITIVE"
        score = min(1.0, 0.35 + 0.1 * pos)
    elif neg > pos:
        sentiment, score = "NEGATIVE", -0.3
    elif pos > neg:
        sentiment, score = "POSITIVE", 0.3
    else:
        sentiment, score = "NEUTRAL", 0.0

    category = _category(text, sentiment)
    likes = int(item.get("likeCount") or 0)
    comments = int(item.get("commentCount") or 0)
    reach = likes + 3 * comments

    if sentiment == "NEGATIVE":
        urgency = "HIGH" if (neg >= 6 or reach >= 120 or "reconsidering" in t or "alternatives" in t or "deal-breaker" in t) else ("MEDIUM" if neg >= 3 or reach >= 40 else "LOW")
    elif sentiment == "NEUTRAL" and category == "QUESTION":
        urgency = "MEDIUM" if reach >= 40 else "LOW"
    else:
        urgency = "LOW"

    summary = _summarise(text, sentiment, category, company)
    suggested = _suggest(sentiment, category, item, company)
    return {
        "externalId": item.get("externalId"),
        "sentiment": sentiment,
        "score": round(score, 3),
        "category": category,
        "themes": _themes(text, company),
        "summary": summary,
        "urgency": urgency,
        "suggestedAction": suggested,
    }


def _summarise(text: str, sentiment: str, category: str, company: str) -> str:
    first = re.split(r"(?<=[.!?])\s+", text.strip())[0] if text.strip() else ""
    first = re.sub(r"\s*#\w+", "", first).strip()
    first = first[:160] + ("…" if len(first) > 160 else "")
    label = {"POSITIVE": "Praise", "NEGATIVE": "Complaint", "NEUTRAL": "Question/observation", "BROADCAST": "Company broadcast"}[sentiment]
    return f"{label} · {category.replace('_', ' ').title()}: {first}" if first else f"{label} · {category.title()}"


def _suggest(sentiment: str, category: str, item: dict[str, Any], company: str) -> str:
    kind = (item.get("kind") or "POST").upper()
    if sentiment == "NEGATIVE":
        return {
            "SUPPORT": "Route to support lead; respond publicly within 4h and open a ticket review.",
            "RELIABILITY": "Reply with incident reference and status-page link; feed into reliability post-mortem.",
            "BILLING": "Escalate to billing operations for account reconciliation and direct outreach.",
            "PRICING": "Account manager to schedule a value review; capture in renewal-risk register.",
            "DATA_QUALITY": "Open a data-integrity investigation and follow up with the customer on findings.",
            "ONBOARDING": "Customer success to review implementation plan with the customer.",
            "PRODUCT": "Log as product feedback; product manager to reply with roadmap position.",
            "PEOPLE": "Account leadership to reach out directly.",
        }.get(category, "Acknowledge publicly and take the conversation to a direct channel.")
    if sentiment == "POSITIVE":
        return "Thank the author publicly; consider requesting a case study or reference quote."
    if sentiment == "NEUTRAL" and category == "QUESTION":
        return "Have a solutions expert answer in-thread; link relevant documentation."
    if sentiment == "BROADCAST":
        return "No action — monitor engagement on this broadcast." if kind == "POST" else "Monitor."
    return "Monitor."


# ---------------------------------------------------------------------------
# Claude engine
# ---------------------------------------------------------------------------

ANALYSE_SYSTEM = """You are the Reveal VoC analyst — a Voice-of-Customer expert reading LinkedIn posts and
comments that mention or tag a company. For every item you classify:

sentiment: one of POSITIVE (customer is happy / praising), NEGATIVE (complaint, frustration, churn risk),
           NEUTRAL (question, comparison, observation with no clear valence),
           BROADCAST (the company or a partner is broadcasting information: announcements, hiring,
           events, marketing — not a customer voice).
score:     -1.0 … 1.0 (negative → positive; BROADCAST and NEUTRAL near 0).
category:  one of %s
themes:    2–5 short lowercase tags (e.g. "response-time", "double-billing").
summary:   one sentence, ≤ 160 chars, business language.
urgency:   LOW | MEDIUM | HIGH — HIGH when churn is threatened, an outage is public, or the post has high reach.
suggestedAction: one concrete sentence for the company's team.

Return ONLY a JSON object: {"results":[{"externalId":..., "sentiment":..., "score":..., "category":...,
"themes":[...], "summary":..., "urgency":..., "suggestedAction":...}, ...]}. Include every externalId given.
""" % ", ".join(CATEGORIES)

ACTIONS_SYSTEM = """You are the Reveal VoC action planner. You receive analysed LinkedIn signals about a company
(mostly NEGATIVE plus a few POSITIVE). Group them into a short list (max 6) of concrete, de-duplicated actions
the company's customer, product or operations team should take. Merge signals with the same root cause.

Each action: {"title": ≤ 90 chars imperative, "description": 1–3 sentences citing what customers said,
"category": one of %s, "priority": LOW|MEDIUM|HIGH, "dedupeKey": stable slug like "support-response-times",
"evidence": [externalIds], "signalCount": n}.
Include one "amplify" action for strong positive sentiment if it exists (priority LOW).
Return ONLY JSON: {"actions":[...]}.
""" % ", ".join(CATEGORIES)

ASK_SYSTEM = """You are the Reveal VoC analyser agent — a senior Voice-of-Customer analyst. You are given aggregate
statistics, a sample of analysed LinkedIn signals and the current action list for a project. Answer the user's
question precisely, cite numbers from the data, quote short fragments where useful, and end with up to three
recommended next steps. Use plain business English; markdown allowed (headings, bullets). If the data cannot
answer the question, say so and suggest what to collect.
Return ONLY JSON: {"answer": "<markdown>", "highlights": ["...", "..."]}.
"""


def _client():
    import anthropic  # local import keeps the module importable without the SDK

    return anthropic.Anthropic(api_key=ANTHROPIC_API_KEY)


def _parse_json(text: str) -> dict:
    text = text.strip()
    fence = re.search(r"```(?:json)?\s*([\s\S]*?)```", text)
    if fence:
        text = fence.group(1).strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        s, e = text.find("{"), text.rfind("}")
        if s >= 0 and e > s:
            return json.loads(text[s : e + 1])
        raise


def _normalise(res: dict[str, Any], fallback: dict[str, Any]) -> dict[str, Any]:
    out = dict(fallback)
    sent = str(res.get("sentiment", "")).upper()
    if sent in SENTIMENTS:
        out["sentiment"] = sent
    try:
        out["score"] = max(-1.0, min(1.0, float(res.get("score", out["score"]))))
    except (TypeError, ValueError):
        pass
    cat = str(res.get("category", "")).upper().replace(" ", "_")
    out["category"] = cat if cat in CATEGORIES else out["category"]
    themes = res.get("themes")
    if isinstance(themes, list):
        out["themes"] = [str(t).lower()[:40] for t in themes][:6]
    if res.get("summary"):
        out["summary"] = str(res["summary"])[:600]
    urg = str(res.get("urgency", "")).upper()
    if urg in URGENCIES:
        out["urgency"] = urg
    if res.get("suggestedAction"):
        out["suggestedAction"] = str(res["suggestedAction"])[:400]
    return out


def analyse_items(company: dict[str, Any], items: list[dict[str, Any]], model: str) -> dict[str, Any]:
    """Classify a batch. Returns {"results": [...], "engine": "claude"|"heuristic", "modelUsed": str}."""
    name = company.get("name") or company.get("handle") or "the company"
    heuristics = {it.get("externalId"): heuristic_analyse(it, name) for it in items}

    if not ANTHROPIC_API_KEY or not items:
        return {"results": list(heuristics.values()), "engine": "heuristic", "modelUsed": ""}

    compact = [
        {
            "externalId": it.get("externalId"),
            "kind": it.get("kind"),
            "authorType": it.get("authorType"),
            "authorHeadline": (it.get("authorHeadline") or "")[:80],
            "content": (it.get("content") or "")[:1200],
            "likeCount": it.get("likeCount", 0),
            "commentCount": it.get("commentCount", 0),
            "publishedAt": it.get("publishedAt"),
        }
        for it in items
    ]
    user = (
        f"Company: {name} (LinkedIn handle @{company.get('handle', '')}; hashtags {company.get('hashtags', [])}).\n"
        f"Classify these {len(compact)} LinkedIn signals:\n{json.dumps(compact, ensure_ascii=False)}"
    )
    try:
        resp = _client().messages.create(
            model=model,
            max_tokens=6000,
            system=ANALYSE_SYSTEM,
            messages=[{"role": "user", "content": user}],
        )
        text = "".join(b.text for b in resp.content if hasattr(b, "text"))
        parsed = _parse_json(text)
        results = []
        for r in parsed.get("results", []):
            eid = r.get("externalId")
            if eid in heuristics:
                results.append(_normalise(r, heuristics[eid]))
        covered = {r["externalId"] for r in results}
        for eid, h in heuristics.items():
            if eid not in covered:
                results.append(h)
        return {"results": results, "engine": "claude", "modelUsed": model}
    except Exception as e:  # noqa: BLE001 — never block the pipeline on the LLM
        out = list(heuristics.values())
        return {"results": out, "engine": "heuristic", "modelUsed": "", "warning": f"claude failed: {e}"[:300]}


# ---------------------------------------------------------------------------
# Actions
# ---------------------------------------------------------------------------

_ACTION_TITLES = {
    "SUPPORT": "Fix support responsiveness raised on LinkedIn",
    "RELIABILITY": "Address public reliability / outage complaints",
    "BILLING": "Resolve billing errors reported by customers",
    "PRICING": "Review renewal pricing concerns before churn",
    "DATA_QUALITY": "Investigate customer-reported data-quality issues",
    "ONBOARDING": "Improve onboarding / migration experience",
    "PRODUCT": "Respond to product regressions & feature complaints",
    "PEOPLE": "Account leadership outreach to unhappy customers",
    "QUESTION": "Answer open community questions in-thread",
    "OTHER": "Follow up on uncategorised negative feedback",
}


def heuristic_actions(company: dict[str, Any], signals: list[dict[str, Any]], positive_total: int) -> list[dict[str, Any]]:
    name = company.get("name") or "the company"
    groups: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for s in signals:
        if (s.get("sentiment") or "").upper() == "NEGATIVE":
            groups[(s.get("category") or "OTHER").upper()].append(s)

    actions = []
    for cat, sigs in sorted(groups.items(), key=lambda kv: -len(kv[1])):
        high = sum(1 for s in sigs if (s.get("urgency") or "").upper() == "HIGH")
        prio = "HIGH" if high >= 1 or len(sigs) >= 4 else ("MEDIUM" if len(sigs) >= 2 else "LOW")
        quotes = [re.sub(r"\s*#\w+", "", (s.get("content") or ""))[:140].strip() for s in sigs[:2]]
        desc = f"{len(sigs)} negative LinkedIn signal(s) about {cat.replace('_', ' ').lower()} for {name}."
        if quotes:
            desc += " Customers said: " + " | ".join(f"“{q}…”" for q in quotes if q)
        actions.append({
            "title": _ACTION_TITLES.get(cat, _ACTION_TITLES["OTHER"]),
            "description": desc,
            "category": cat,
            "priority": prio,
            "dedupeKey": f"neg-{cat.lower()}",
            "evidence": [s.get("externalId") for s in sigs if s.get("externalId")],
            "signalCount": len(sigs),
        })
        if len(actions) >= 6:
            break

    if positive_total >= 3:
        pos = [s for s in signals if (s.get("sentiment") or "").upper() == "POSITIVE"]
        actions.append({
            "title": "Amplify positive customer stories",
            "description": f"{positive_total} positive LinkedIn signals for {name}. Thank the authors and ask the strongest advocates for a case study or reference quote.",
            "category": "ANNOUNCEMENT",
            "priority": "LOW",
            "dedupeKey": "amplify-positive",
            "evidence": [s.get("externalId") for s in pos[:10] if s.get("externalId")],
            "signalCount": positive_total,
        })
    return actions


def propose_actions(company: dict[str, Any], signals: list[dict[str, Any]], positive_total: int, negative_total: int, model: str) -> dict[str, Any]:
    fallback = heuristic_actions(company, signals, positive_total)
    if not ANTHROPIC_API_KEY or not signals:
        return {"actions": fallback, "engine": "heuristic"}
    user = (
        f"Company: {company.get('name')} (@{company.get('handle', '')}). "
        f"Totals: {negative_total} negative, {positive_total} positive signals.\n"
        f"Signals:\n{json.dumps(signals, ensure_ascii=False)[:60000]}"
    )
    try:
        resp = _client().messages.create(
            model=model, max_tokens=3000, system=ACTIONS_SYSTEM,
            messages=[{"role": "user", "content": user}],
        )
        text = "".join(b.text for b in resp.content if hasattr(b, "text"))
        parsed = _parse_json(text)
        actions = []
        for a in parsed.get("actions", [])[:8]:
            if not a.get("title"):
                continue
            cat = str(a.get("category", "OTHER")).upper().replace(" ", "_")
            prio = str(a.get("priority", "MEDIUM")).upper()
            actions.append({
                "title": str(a["title"])[:300],
                "description": str(a.get("description", ""))[:2000],
                "category": cat if cat in CATEGORIES else "OTHER",
                "priority": prio if prio in URGENCIES else "MEDIUM",
                "dedupeKey": re.sub(r"[^a-z0-9-]", "-", str(a.get("dedupeKey") or a["title"]).lower())[:120],
                "evidence": [str(e) for e in a.get("evidence", []) if e][:50],
                "signalCount": int(a.get("signalCount") or len(a.get("evidence", []) or [])),
            })
        return {"actions": actions or fallback, "engine": "claude" if actions else "heuristic", "modelUsed": model}
    except Exception as e:  # noqa: BLE001
        return {"actions": fallback, "engine": "heuristic", "warning": f"claude failed: {e}"[:300]}


# ---------------------------------------------------------------------------
# Q&A
# ---------------------------------------------------------------------------

def heuristic_answer(question: str, stats: dict[str, Any], signals: list[dict[str, Any]], actions: list[dict[str, Any]]) -> dict[str, Any]:
    by_s = stats.get("bySentiment") or {}
    total = stats.get("totalPosts", 0)
    analysed = stats.get("analysed", 0) or 1
    neg = by_s.get("NEGATIVE", 0)
    pos = by_s.get("POSITIVE", 0)
    bro = by_s.get("BROADCAST", 0)
    neu = by_s.get("NEUTRAL", 0)
    cats = Counter()
    for s in signals:
        if (s.get("sentiment") or "").upper() == "NEGATIVE":
            cats[(s.get("category") or "OTHER")] += 1
    top_neg = ", ".join(f"{c.replace('_', ' ').lower()} ({n})" for c, n in cats.most_common(3)) or "none yet"
    open_actions = [a for a in actions if (a.get("status") or "OPEN") in ("OPEN", "IN_PROGRESS")]
    q = question.lower()

    lines = [f"### Voice of Customer snapshot", ""]
    lines.append(f"- **{total}** LinkedIn signals collected, **{stats.get('analysed', 0)}** analysed.")
    lines.append(f"- Sentiment: **{pos}** positive ({pos * 100 // analysed}%), **{neg}** negative ({neg * 100 // analysed}%), {neu} neutral, {bro} broadcast.")
    lines.append(f"- Top negative themes: {top_neg}.")
    lines.append(f"- Open actions: **{len(open_actions)}**" + (f" — highest priority: “{open_actions[0].get('title')}”." if open_actions else "."))
    if any(k in q for k in ("negative", "complain", "unhappy", "risk", "churn")):
        lines += ["", "**Most recent negative signals**"]
        for s in [s for s in signals if (s.get("sentiment") or "").upper() == "NEGATIVE"][:5]:
            lines.append(f"- {s.get('publishedAt', '')} · {s.get('company', '')} · {s.get('summary') or s.get('content', '')[:120]}")
    elif any(k in q for k in ("positive", "happy", "praise", "advocate")):
        lines += ["", "**Recent positive signals**"]
        for s in [s for s in signals if (s.get("sentiment") or "").upper() == "POSITIVE"][:5]:
            lines.append(f"- {s.get('publishedAt', '')} · {s.get('company', '')} · {s.get('summary') or s.get('content', '')[:120]}")
    lines += ["", "**Recommended next steps**"]
    for a in open_actions[:3]:
        lines.append(f"- {a.get('title')} ({a.get('priority')})")
    if not open_actions:
        lines.append("- Run a sync to collect signals, then review the Actions page.")
    lines += ["", "_Claude is not configured (ANTHROPIC_API_KEY); this is a rule-based summary._"]
    return {"answer": "\n".join(lines), "highlights": [f"{neg} negative", f"{pos} positive", f"{len(open_actions)} open actions"], "engine": "heuristic"}


def answer_question(question: str, stats: dict[str, Any], signals: list[dict[str, Any]], actions: list[dict[str, Any]], model: str) -> dict[str, Any]:
    if not ANTHROPIC_API_KEY:
        return heuristic_answer(question, stats, signals, actions)
    slim_stats = {k: stats.get(k) for k in ("totalPosts", "analysed", "companies", "openActions", "highUrgency", "bySentiment", "byCategory", "byCompany", "topThemes", "topHashtags", "timeline")}
    slim_actions = [{k: a.get(k) for k in ("title", "priority", "status", "category", "signalCount", "companyName")} for a in actions[:20]]
    user = (
        f"Question: {question}\n\nStats: {json.dumps(slim_stats, default=str)}\n\n"
        f"Signals (sample of {len(signals)}): {json.dumps(signals, ensure_ascii=False, default=str)[:60000]}\n\n"
        f"Actions: {json.dumps(slim_actions, default=str)}"
    )
    try:
        resp = _client().messages.create(
            model=model, max_tokens=2500, system=ASK_SYSTEM,
            messages=[{"role": "user", "content": user}],
        )
        text = "".join(b.text for b in resp.content if hasattr(b, "text"))
        parsed = _parse_json(text)
        return {"answer": parsed.get("answer") or text, "highlights": parsed.get("highlights", []), "engine": "claude", "modelUsed": model}
    except Exception as e:  # noqa: BLE001
        out = heuristic_answer(question, stats, signals, actions)
        out["warning"] = f"claude failed: {e}"[:300]
        return out
