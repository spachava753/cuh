package gmail_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spachava753/cuh/gmail"
)

func composeSummarizeUnreadNewslettersAndArchive(ctx context.Context, newsletterDomains []string, dryRun bool) (map[string]string, error) {
	_ = ctx
	unread := false
	findOut, err := gmail.Find(gmail.FindInput{
		Entity: gmail.EntityMessage,
		Query: gmail.Query{
			Seen:      &unread,
			InMailbox: []string{"INBOX"},
			LabelAny:  []string{"CATEGORY_PROMOTIONS"},
		},
		Page:        gmail.Page{Limit: 100},
		Sort:        gmail.Sort{By: gmail.SortByDate, Order: gmail.SortOrderDesc},
		IncludeMeta: true,
	})
	if err != nil {
		return nil, err
	}

	filteredRefs := refsMatchingSenderDomains(findOut.Refs, findOut.Meta, newsletterDomains)
	if len(filteredRefs) == 0 {
		return map[string]string{}, nil
	}

	getOut, err := gmail.Get(gmail.GetInput{
		Refs: filteredRefs,
		Fields: []gmail.Field{
			gmail.FieldSubject,
			gmail.FieldFrom,
			gmail.FieldDate,
			gmail.FieldSnippet,
			gmail.FieldTextBody,
		},
		Body: gmail.BodyOptions{IncludeText: true, MaxChars: 5000},
	})
	if err != nil {
		return nil, err
	}

	summaries := map[string]string{}
	for _, item := range getOut.Items {
		summaries[item.Ref.ID] = compactSummary(item)
	}

	_, err = gmail.Mutate(gmail.MutateInput{
		Refs: filteredRefs,
		Ops: []gmail.MutationOp{
			{Type: gmail.MutationSetSeen, Value: "true"},
			{Type: gmail.MutationMoveMailbox, Value: "[Gmail]/All Mail"},
		},
		DryRun: dryRun,
	})
	if err != nil {
		return nil, err
	}

	return summaries, nil
}

func composeFindReceiptsOrInvoicesLast30Days(ctx context.Context) ([]gmail.Item, error) {
	_ = ctx
	after := time.Now().AddDate(0, 0, -30)

	invoiceFind, err := gmail.Find(gmail.FindInput{
		Entity: gmail.EntityMessage,
		Query: gmail.Query{
			After:           &after,
			SubjectContains: []string{"invoice"},
			InMailbox:       []string{"[Gmail]/All Mail"},
		},
		Page: gmail.Page{Limit: 200},
	})
	if err != nil {
		return nil, err
	}

	receiptFind, err := gmail.Find(gmail.FindInput{
		Entity: gmail.EntityMessage,
		Query: gmail.Query{
			After:           &after,
			SubjectContains: []string{"receipt"},
			InMailbox:       []string{"[Gmail]/All Mail"},
		},
		Page: gmail.Page{Limit: 200},
	})
	if err != nil {
		return nil, err
	}

	mergedRefs := dedupeRefs(append(invoiceFind.Refs, receiptFind.Refs...))
	if len(mergedRefs) == 0 {
		return []gmail.Item{}, nil
	}

	getOut, err := gmail.Get(gmail.GetInput{
		Refs: mergedRefs,
		Fields: []gmail.Field{
			gmail.FieldSubject,
			gmail.FieldFrom,
			gmail.FieldDate,
			gmail.FieldSnippet,
		},
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(getOut.Items, func(i, j int) bool {
		return getOut.Items[i].Date.After(getOut.Items[j].Date)
	})

	return getOut.Items, nil
}

func composeMarkPromoOlderThan14DaysRead(ctx context.Context, dryRun bool) (gmail.MutateOutput, error) {
	_ = ctx
	unread := false
	before := time.Now().AddDate(0, 0, -14)

	findOut, err := gmail.Find(gmail.FindInput{
		Entity: gmail.EntityMessage,
		Query: gmail.Query{
			Seen:      &unread,
			Before:    &before,
			LabelAny:  []string{"CATEGORY_PROMOTIONS"},
			InMailbox: []string{"INBOX"},
		},
		Page: gmail.Page{Limit: 500},
	})
	if err != nil {
		return gmail.MutateOutput{}, err
	}
	if len(findOut.Refs) == 0 {
		return gmail.MutateOutput{DryRun: dryRun, Results: []gmail.MutationResult{}}, nil
	}

	return gmail.Mutate(gmail.MutateInput{
		Refs:   findOut.Refs,
		Ops:    []gmail.MutationOp{{Type: gmail.MutationSetSeen, Value: "true"}},
		DryRun: dryRun,
	})
}

func composeDeletePhishingLikeUnread(ctx context.Context, trustedDomains []string, dryRun bool) (gmail.MutateOutput, error) {
	_ = ctx
	unread := false
	findOut, err := gmail.Find(gmail.FindInput{
		Entity: gmail.EntityMessage,
		Query: gmail.Query{
			Seen:      &unread,
			InMailbox: []string{"INBOX"},
		},
		Page:        gmail.Page{Limit: 200},
		Sort:        gmail.Sort{By: gmail.SortByDate, Order: gmail.SortOrderDesc},
		IncludeMeta: true,
	})
	if err != nil {
		return gmail.MutateOutput{}, err
	}
	if len(findOut.Refs) == 0 {
		return gmail.MutateOutput{DryRun: dryRun, Results: []gmail.MutationResult{}}, nil
	}

	getOut, err := gmail.Get(gmail.GetInput{
		Refs: findOut.Refs,
		Fields: []gmail.Field{
			gmail.FieldSubject,
			gmail.FieldFrom,
			gmail.FieldSnippet,
			gmail.FieldTextBody,
		},
		Body: gmail.BodyOptions{IncludeText: true, MaxChars: 4000},
	})
	if err != nil {
		return gmail.MutateOutput{}, err
	}

	candidateRefs := make([]gmail.Ref, 0, len(getOut.Items))
	for _, item := range getOut.Items {
		if looksPhishy(item, trustedDomains) {
			candidateRefs = append(candidateRefs, item.Ref)
		}
	}
	if len(candidateRefs) == 0 {
		return gmail.MutateOutput{DryRun: dryRun, Results: []gmail.MutationResult{}}, nil
	}

	return gmail.Mutate(gmail.MutateInput{
		Refs:   candidateRefs,
		Ops:    []gmail.MutationOp{{Type: gmail.MutationTrash}},
		DryRun: dryRun,
	})
}

func composeReplyToLatestMessageInThread(ctx context.Context, threadRef gmail.Ref, textBody string, dryRun bool) (gmail.SendOutput, error) {
	_ = ctx
	if threadRef.Entity != gmail.EntityThread {
		return gmail.SendOutput{}, fmt.Errorf("expected thread ref, got %q", threadRef.Entity)
	}

	getOut, err := gmail.Get(gmail.GetInput{
		Refs: []gmail.Ref{threadRef},
		Fields: []gmail.Field{
			gmail.FieldDate,
			gmail.FieldSubject,
			gmail.FieldFrom,
			gmail.FieldMessageID,
			gmail.FieldThreadID,
		},
	})
	if err != nil {
		return gmail.SendOutput{}, err
	}
	if len(getOut.Items) == 0 {
		return gmail.SendOutput{}, fmt.Errorf("thread %s has no messages", threadRef.ID)
	}

	sort.Slice(getOut.Items, func(i, j int) bool {
		return getOut.Items[i].Date.Before(getOut.Items[j].Date)
	})
	latest := getOut.Items[len(getOut.Items)-1]

	to := []string{}
	for _, addr := range latest.From {
		if strings.TrimSpace(addr.Email) != "" {
			to = append(to, addr.Email)
		}
	}
	if len(to) == 0 {
		return gmail.SendOutput{}, fmt.Errorf("latest message in thread %s has no sender", threadRef.ID)
	}

	subject := latest.Subject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	return gmail.Send(gmail.SendInput{
		Message: gmail.OutgoingMessage{
			To:       to,
			Subject:  subject,
			TextBody: textBody,
			ReplyToRef: &gmail.Ref{
				Entity: gmail.EntityMessage,
				ID:     latest.Ref.ID,
			},
		},
		DryRun: dryRun,
	})
}

func refsMatchingSenderDomains(refs []gmail.Ref, meta []gmail.Meta, domains []string) []gmail.Ref {
	if len(domains) == 0 || len(meta) == 0 {
		return refs
	}
	wanted := map[string]struct{}{}
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		domain = strings.TrimPrefix(domain, "@")
		if domain != "" {
			wanted[domain] = struct{}{}
		}
	}

	out := make([]gmail.Ref, 0, len(refs))
	for i := range refs {
		if i >= len(meta) {
			break
		}
		if senderInDomains(meta[i].From, wanted) {
			out = append(out, refs[i])
		}
	}
	return out
}

func senderInDomains(from []gmail.Address, domains map[string]struct{}) bool {
	if len(domains) == 0 {
		return true
	}
	for _, addr := range from {
		email := strings.TrimSpace(addr.Email)
		at := strings.LastIndex(email, "@")
		if at < 0 || at == len(email)-1 {
			continue
		}
		domain := strings.ToLower(email[at+1:])
		if _, ok := domains[domain]; ok {
			return true
		}
	}
	return false
}

func dedupeRefs(refs []gmail.Ref) []gmail.Ref {
	seen := map[string]struct{}{}
	out := make([]gmail.Ref, 0, len(refs))
	for _, ref := range refs {
		key := string(ref.Entity) + ":" + ref.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func compactSummary(item gmail.Item) string {
	from := ""
	if len(item.From) > 0 {
		from = item.From[0].Email
	}
	text := item.Snippet
	if strings.TrimSpace(text) == "" {
		text = item.TextBody
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 220 {
		text = text[:220]
	}
	return fmt.Sprintf("From=%s | Subject=%s | Summary=%s", from, item.Subject, text)
}

func looksPhishy(item gmail.Item, trustedDomains []string) bool {
	if senderMatchesTrustedDomain(item.From, trustedDomains) {
		return false
	}
	text := strings.ToLower(strings.Join([]string{item.Subject, item.Snippet, item.TextBody}, " "))
	keywords := []string{
		"verify your account",
		"suspended",
		"urgent action required",
		"password expired",
		"wire transfer",
		"gift card",
	}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func senderMatchesTrustedDomain(from []gmail.Address, trustedDomains []string) bool {
	if len(trustedDomains) == 0 {
		return false
	}
	trusted := map[string]struct{}{}
	for _, d := range trustedDomains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d != "" {
			trusted[d] = struct{}{}
		}
	}
	for _, addr := range from {
		email := strings.ToLower(strings.TrimSpace(addr.Email))
		at := strings.LastIndex(email, "@")
		if at < 0 || at == len(email)-1 {
			continue
		}
		domain := email[at+1:]
		if _, ok := trusted[domain]; ok {
			return true
		}
	}
	return false
}
