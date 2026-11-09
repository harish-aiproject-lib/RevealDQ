package voc

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// SampleProvider generates realistic, deterministic LinkedIn-style signals so
// the whole VoC pipeline (raw store → agentic analysis → actions → dashboard →
// scheduled refresh) can be exercised without LinkedIn credentials.
//
// Determinism: the base set is seeded by the company handle so re-syncs are
// idempotent (upserted by external id). A small "fresh" batch is seeded by
// the current 6-hour window so a periodic refresh visibly brings in new items.
type SampleProvider struct{}

func (p *SampleProvider) Name() string { return "SAMPLE" }

var samplePeople = []struct{ name, headline string }{
	{"Priya Raman", "Head of Customer Operations · Telco"},
	{"Daniel Okafor", "Product Manager, Payments"},
	{"Mei-Ling Chen", "Data Platform Lead"},
	{"Tom Baxter", "Small business owner"},
	{"Aisha Khan", "Contact Centre Director"},
	{"Lucas Moreau", "CIO · Retail"},
	{"Sofia Alvarez", "Customer Success Manager"},
	{"Rahul Desai", "Finance Transformation Consultant"},
	{"Grace Whitfield", "Marketing Director"},
	{"Kenji Watanabe", "Solutions Architect"},
}

var samplePositive = []string{
	"Huge shout-out to the %s support team — resolved our billing migration issue in under an hour. This is what partnership looks like. #%s #CustomerExperience",
	"Six months on %s and our data quality incidents are down 40%%. The onboarding was smooth and the team actually listens. #%s",
	"Just finished the %s webinar on migration readiness. Practical, no fluff, real examples. Recommend it to anyone planning a cloud move. #%s",
	"Renewed with %s for another 3 years. Reliability has been excellent and the roadmap is exactly where we need it. #%s #Partnership",
	"Impressed by how quickly %s shipped the fix we asked for last quarter. Rare to see a vendor move that fast. #%s",
	"Our finance close is two days faster since we rolled out %s across the group. Happy CFO, happy team. #%s #Finance",
}

var sampleNegative = []string{
	"Third outage this month with %s. No status page update for 2 hours and support keeps asking for the same logs. Seriously reconsidering. #%s #Outage",
	"%s billing has double-charged us again. Two tickets open, zero replies for 5 days. Not the experience we were sold. #%s",
	"Migrated to %s last quarter — the data loss on the CDR feed still isn't explained. Escalated to our account manager, no answer. #%s #DataQuality",
	"The new %s UI update removed the export we rely on every day. Who is testing this with real customers? #%s",
	"Renewal pricing from %s jumped 35%% with no added value. Asking around for alternatives. #%s #Pricing",
	"Waited 47 minutes in the %s support queue today for a password reset. Please fix the basics before adding AI features. #%s #Support",
	"%s API rate limits changed overnight and broke our integration. A heads-up would have been nice. #%s #Developers",
}

var sampleNeutral = []string{
	"Anyone here using %s together with Databricks for revenue assurance? Looking for lessons learned before we start. #%s #Databricks",
	"How does %s handle GDPR deletion requests across replicated warehouses? Genuine question for the community. #%s",
	"Comparing %s and two other vendors for our data-quality programme. What questions should we be asking in the RFP? #%s",
	"Attending the %s customer roundtable in Sydney next week. Ping me if you're there. #%s",
}

var sampleBroadcast = []string{
	"We're excited to announce %s Summit 2026 — three days of customer stories, product deep-dives and hands-on labs. Registration is open. #%s #Summit2026",
	"New on the %s blog: five patterns for revealing data-quality debt before a migration. Link in comments. #%s #DataQuality",
	"%s is hiring! We're growing our customer engineering team across APAC. Join us. #%s #Hiring",
	"Join our live session Thursday: the %s product team walks through the Q3 release and what's next. #%s #Webinar",
	"Proud to be recognised in the 2026 Enterprise Data Platforms report. Thank you to our customers. — Team %s #%s",
}

var sampleComments = []string{
	"Same experience here — the support response times have really slipped this year.",
	"Congrats team! Looking forward to the summit.",
	"We had exactly this issue. Turned out to be a config default; happy to share notes.",
	"Can you share the status page link? Couldn't find it during the incident.",
	"Great to hear. How long did the onboarding take end-to-end?",
	"Following. We're evaluating the same thing.",
	"Our account manager sorted this within a day — worth escalating.",
	"This is a deal-breaker for us too. Any ETA on restoring the export?",
}

func (p *SampleProvider) Fetch(_ context.Context, c Company, since, until time.Time) ([]Item, error) {
	tag := primaryTag(c)
	base := rand.New(rand.NewSource(int64(hashString(strings.ToLower(c.Handle)))))
	now := time.Now().UTC()

	var items []Item
	emit := func(r *rand.Rand, idPrefix string, n int, maxAgeHours int, freshWindow bool) {
		for i := 0; i < n; i++ {
			ageH := r.Intn(maxAgeHours)
			pub := now.Add(-time.Duration(ageH) * time.Hour)
			if freshWindow {
				pub = now.Add(-time.Duration(r.Intn(300)) * time.Minute)
			}
			id := fmt.Sprintf("sample:%s:%s:%03d", strings.ToLower(c.Handle), idPrefix, i)

			var text string
			authorType := "PERSON"
			person := samplePeople[r.Intn(len(samplePeople))]
			roll := r.Intn(100)
			switch {
			case roll < 35:
				text = fmt.Sprintf(pick(r, samplePositive), c.Name, tag)
			case roll < 65:
				text = fmt.Sprintf(pick(r, sampleNegative), c.Name, tag)
			case roll < 80:
				text = fmt.Sprintf(pick(r, sampleNeutral), c.Name, tag)
			default:
				text = fmt.Sprintf(pick(r, sampleBroadcast), c.Name, tag)
				authorType = "ORGANIZATION"
				person.name, person.headline = c.Name, "Official page"
			}
			post := Item{
				ExternalID:     id,
				Kind:           "POST",
				AuthorName:     person.name,
				AuthorHandle:   slug(person.name),
				AuthorHeadline: person.headline,
				AuthorType:     authorType,
				Content:        text + " @" + c.Handle,
				URL:            "https://www.linkedin.com/feed/update/" + id,
				Hashtags:       extractHashtags(text),
				Mentions:       []string{strings.ToLower(c.Handle)},
				LikeCount:      r.Intn(240),
				CommentCount:   0,
				ShareCount:     r.Intn(30),
				PublishedAt:    pub,
				Raw:            map[string]interface{}{"generator": "sample", "seed": idPrefix},
			}
			nComments := r.Intn(3)
			post.CommentCount = nComments
			items = append(items, post)
			for k := 0; k < nComments; k++ {
				cp := samplePeople[r.Intn(len(samplePeople))]
				items = append(items, Item{
					ExternalID:       fmt.Sprintf("%s:c%d", id, k),
					Kind:             "COMMENT",
					ParentExternalID: id,
					AuthorName:       cp.name,
					AuthorHandle:     slug(cp.name),
					AuthorHeadline:   cp.headline,
					AuthorType:       "PERSON",
					Content:          pick(r, sampleComments),
					Hashtags:         []string{},
					Mentions:         []string{strings.ToLower(c.Handle)},
					LikeCount:        r.Intn(40),
					PublishedAt:      pub.Add(time.Duration(30+r.Intn(600)) * time.Minute),
					Raw:              map[string]interface{}{"generator": "sample"},
				})
			}
		}
	}

	// Stable history: ~18 posts over the last 21 days.
	emit(base, "base", 18, 21*24, false)

	// Fresh batch: 2–4 posts keyed to the current 6-hour window so a refresh
	// brings in new signals.
	window := now.Truncate(6 * time.Hour).Format("2006010215")
	fresh := rand.New(rand.NewSource(int64(hashString(strings.ToLower(c.Handle) + window))))
	emit(fresh, "w"+window, 2+fresh.Intn(3), 6, true)

	return filterWindow(items, Window{From: since, To: until}), nil
}

func primaryTag(c Company) string {
	if len(c.Hashtags) > 0 {
		return strings.TrimPrefix(c.Hashtags[0], "#")
	}
	// CamelCase the company name: "Acme Telecom" → "AcmeTelecom".
	var b strings.Builder
	for _, w := range strings.Fields(c.Name) {
		b.WriteString(strings.ToUpper(w[:1]) + strings.ToLower(w[1:]))
	}
	return b.String()
}

func pick(r *rand.Rand, list []string) string { return list[r.Intn(len(list))] }

func slug(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}
