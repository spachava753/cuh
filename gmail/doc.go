// Package gmail provides agent-oriented Gmail primitives designed for composition.
//
// The package intentionally exposes only five core operations:
//
//   - Find: select message/thread references with typed query filters.
//   - Get: hydrate refs into full typed message items.
//   - Mutate: apply explicit state transitions (labels/flags/mailbox/trash).
//   - Send: send a new message or reply.
//   - Labels: discover the available Gmail label catalog.
//
// Recipes such as "unread today", "archive newsletters", or "label sender X"
// are expected to be composed from these primitives as: find -> get -> decide ->
// mutate/send.
//
// # Authentication
//
// Runtime credentials are read from environment variables:
//
//   - GMAIL_ADDRESS
//   - GMAIL_APP_PASSWORD
//
// The package uses Gmail IMAP for read/search/mutation primitives and Gmail SMTP
// for send.
//
// # Safety Model
//
// Mutate and Send both support dry-run modes for validation/planning without
// side effects. Mutating operations are explicit via MutationOp values; no hidden
// state transitions are performed.
//
// Core Composition Pattern
//
//  1. Use Find with a typed Query and pagination.
//  2. Use Get to hydrate only the refs you need for reasoning/summarization.
//  3. Apply Mutate (optionally DryRun first) or Send.
//
// Minimal example (mark unread messages as seen):
//
//	seen := false
//	findOut, err := gmail.Find(gmail.FindInput{
//		Entity: gmail.EntityMessage,
//		Query: gmail.Query{Seen: &seen},
//		Page:  gmail.Page{Limit: 20},
//	})
//	if err != nil { /* handle */ }
//
//	_, err = gmail.Mutate(gmail.MutateInput{
//		Refs: findOut.Refs,
//		Ops:  []gmail.MutationOp{{Type: gmail.MutationSetSeen, Value: "true"}},
//	})
//	if err != nil { /* handle */ }
//
// Reply flow (thread-aware):
//
//  1. Find the target message/thread.
//  2. Send with OutgoingMessage.ReplyToRef.
//  3. Optionally use Get to verify delivery artifacts.
//
// Label flow:
//
//  1. Call Labels to discover canonical label names.
//  2. Use Mutate with MutationAddLabel / MutationRemoveLabel.
//
// # Pagination
//
// FindOutput.NextCursor can be fed back into FindInput.Page.Cursor to continue
// scanning results without rebuilding recipe-specific APIs.
//
// # Advanced Composition Examples
//
// The snippets below are intentionally recipe-level and built only from the five
// primitives. They are compile-validated in this repository.
//
// 1) Summarize unread newsletters and archive them:
//
//	func summarizeUnreadNewslettersAndArchive(newsletterDomains []string, dryRun bool) (map[string]string, error) {
//		unread := false
//		findOut, err := gmail.Find(gmail.FindInput{
//			Entity: gmail.EntityMessage,
//			Query: gmail.Query{
//				Seen:      &unread,
//				InMailbox: []string{"INBOX"},
//				LabelAny:  []string{"CATEGORY_PROMOTIONS"},
//			},
//			Page:        gmail.Page{Limit: 100},
//			Sort:        gmail.Sort{By: gmail.SortByDate, Order: gmail.SortOrderDesc},
//			IncludeMeta: true,
//		})
//		if err != nil {
//			return nil, err
//		}
//
//		filteredRefs := refsMatchingSenderDomains(findOut.Refs, findOut.Meta, newsletterDomains)
//		if len(filteredRefs) == 0 {
//			return map[string]string{}, nil
//		}
//
//		getOut, err := gmail.Get(gmail.GetInput{
//			Refs: filteredRefs,
//			Fields: []gmail.Field{
//				gmail.FieldSubject,
//				gmail.FieldFrom,
//				gmail.FieldDate,
//				gmail.FieldSnippet,
//				gmail.FieldTextBody,
//			},
//			Body: gmail.BodyOptions{IncludeText: true, MaxChars: 5000},
//		})
//		if err != nil {
//			return nil, err
//		}
//
//		summaries := map[string]string{}
//		for _, item := range getOut.Items {
//			summaries[item.Ref.ID] = compactSummary(item)
//		}
//
//		_, err = gmail.Mutate(gmail.MutateInput{
//			Refs: filteredRefs,
//			Ops: []gmail.MutationOp{
//				{Type: gmail.MutationSetSeen, Value: "true"},
//				{Type: gmail.MutationMoveMailbox, Value: "[Gmail]/All Mail"},
//			},
//			DryRun: dryRun,
//		})
//		if err != nil {
//			return nil, err
//		}
//		return summaries, nil
//	}
//
// 2) Find receipts or invoices from the last 30 days:
//
//	func findReceiptsOrInvoicesLast30Days() ([]gmail.Item, error) {
//		after := time.Now().AddDate(0, 0, -30)
//
//		invoiceFind, err := gmail.Find(gmail.FindInput{
//			Entity: gmail.EntityMessage,
//			Query: gmail.Query{
//				After:           &after,
//				SubjectContains: []string{"invoice"},
//				InMailbox:       []string{"[Gmail]/All Mail"},
//			},
//			Page: gmail.Page{Limit: 200},
//		})
//		if err != nil {
//			return nil, err
//		}
//
//		receiptFind, err := gmail.Find(gmail.FindInput{
//			Entity: gmail.EntityMessage,
//			Query: gmail.Query{
//				After:           &after,
//				SubjectContains: []string{"receipt"},
//				InMailbox:       []string{"[Gmail]/All Mail"},
//			},
//			Page: gmail.Page{Limit: 200},
//		})
//		if err != nil {
//			return nil, err
//		}
//
//		mergedRefs := dedupeRefs(append(invoiceFind.Refs, receiptFind.Refs...))
//		if len(mergedRefs) == 0 {
//			return []gmail.Item{}, nil
//		}
//
//		getOut, err := gmail.Get(gmail.GetInput{
//			Refs: mergedRefs,
//			Fields: []gmail.Field{
//				gmail.FieldSubject,
//				gmail.FieldFrom,
//				gmail.FieldDate,
//				gmail.FieldSnippet,
//			},
//		})
//		if err != nil {
//			return nil, err
//		}
//		return getOut.Items, nil
//	}
//
// 3) Mark promo emails older than 14 days as read:
//
//	func markPromoOlderThan14DaysRead(dryRun bool) (gmail.MutateOutput, error) {
//		unread := false
//		before := time.Now().AddDate(0, 0, -14)
//
//		findOut, err := gmail.Find(gmail.FindInput{
//			Entity: gmail.EntityMessage,
//			Query: gmail.Query{
//				Seen:      &unread,
//				Before:    &before,
//				LabelAny:  []string{"CATEGORY_PROMOTIONS"},
//				InMailbox: []string{"INBOX"},
//			},
//			Page: gmail.Page{Limit: 500},
//		})
//		if err != nil {
//			return gmail.MutateOutput{}, err
//		}
//		if len(findOut.Refs) == 0 {
//			return gmail.MutateOutput{DryRun: dryRun, Results: []gmail.MutationResult{}}, nil
//		}
//
//		return gmail.Mutate(gmail.MutateInput{
//			Refs:   findOut.Refs,
//			Ops:    []gmail.MutationOp{{Type: gmail.MutationSetSeen, Value: "true"}},
//			DryRun: dryRun,
//		})
//	}
//
// 4) Delete phishing-like unread messages based on local heuristics:
//
//	func deletePhishingLikeUnread(trustedDomains []string, dryRun bool) (gmail.MutateOutput, error) {
//		unread := false
//		findOut, err := gmail.Find(gmail.FindInput{
//			Entity: gmail.EntityMessage,
//			Query: gmail.Query{
//				Seen:      &unread,
//				InMailbox: []string{"INBOX"},
//			},
//			Page:        gmail.Page{Limit: 200},
//			Sort:        gmail.Sort{By: gmail.SortByDate, Order: gmail.SortOrderDesc},
//			IncludeMeta: true,
//		})
//		if err != nil {
//			return gmail.MutateOutput{}, err
//		}
//
//		getOut, err := gmail.Get(gmail.GetInput{
//			Refs: findOut.Refs,
//			Fields: []gmail.Field{
//				gmail.FieldSubject,
//				gmail.FieldFrom,
//				gmail.FieldSnippet,
//				gmail.FieldTextBody,
//			},
//			Body: gmail.BodyOptions{IncludeText: true, MaxChars: 4000},
//		})
//		if err != nil {
//			return gmail.MutateOutput{}, err
//		}
//
//		candidateRefs := make([]gmail.Ref, 0, len(getOut.Items))
//		for _, item := range getOut.Items {
//			if looksPhishy(item, trustedDomains) {
//				candidateRefs = append(candidateRefs, item.Ref)
//			}
//		}
//		if len(candidateRefs) == 0 {
//			return gmail.MutateOutput{DryRun: dryRun, Results: []gmail.MutationResult{}}, nil
//		}
//
//		return gmail.Mutate(gmail.MutateInput{
//			Refs:   candidateRefs,
//			Ops:    []gmail.MutationOp{{Type: gmail.MutationTrash}},
//			DryRun: dryRun,
//		})
//	}
//
// 5) Reply to the latest message in a thread:
//
//	func replyToLatestMessageInThread(threadRef gmail.Ref, body string, dryRun bool) (gmail.SendOutput, error) {
//		if threadRef.Entity != gmail.EntityThread {
//			return gmail.SendOutput{}, fmt.Errorf("expected thread ref, got %q", threadRef.Entity)
//		}
//
//		getOut, err := gmail.Get(gmail.GetInput{
//			Refs: []gmail.Ref{threadRef},
//			Fields: []gmail.Field{
//				gmail.FieldDate,
//				gmail.FieldSubject,
//				gmail.FieldFrom,
//				gmail.FieldMessageID,
//				gmail.FieldThreadID,
//			},
//		})
//		if err != nil {
//			return gmail.SendOutput{}, err
//		}
//		if len(getOut.Items) == 0 {
//			return gmail.SendOutput{}, fmt.Errorf("thread %s has no messages", threadRef.ID)
//		}
//
//		sort.Slice(getOut.Items, func(i, j int) bool {
//			return getOut.Items[i].Date.Before(getOut.Items[j].Date)
//		})
//		latest := getOut.Items[len(getOut.Items)-1]
//
//		to := []string{}
//		for _, addr := range latest.From {
//			if strings.TrimSpace(addr.Email) != "" {
//				to = append(to, addr.Email)
//			}
//		}
//		if len(to) == 0 {
//			return gmail.SendOutput{}, fmt.Errorf("latest message in thread %s has no sender", threadRef.ID)
//		}
//
//		subject := latest.Subject
//		if !strings.HasPrefix(strings.ToLower(subject), "re:") {
//			subject = "Re: " + subject
//		}
//
//		return gmail.Send(gmail.SendInput{
//			Message: gmail.OutgoingMessage{
//				To:       to,
//				Subject:  subject,
//				TextBody: body,
//				ReplyToRef: &gmail.Ref{
//					Entity: gmail.EntityMessage,
//					ID:     latest.Ref.ID,
//				},
//			},
//			DryRun: dryRun,
//		})
//	}
//
// Helper snippets used by the examples above:
//
//	func refsMatchingSenderDomains(refs []gmail.Ref, meta []gmail.Meta, domains []string) []gmail.Ref {
//		if len(domains) == 0 || len(meta) == 0 {
//			return refs
//		}
//		wanted := map[string]struct{}{}
//		for _, domain := range domains {
//			domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
//			if domain != "" {
//				wanted[domain] = struct{}{}
//			}
//		}
//
//		out := make([]gmail.Ref, 0, len(refs))
//		for i := range refs {
//			if i >= len(meta) {
//				break
//			}
//			for _, addr := range meta[i].From {
//				email := strings.ToLower(strings.TrimSpace(addr.Email))
//				at := strings.LastIndex(email, "@")
//				if at >= 0 && at < len(email)-1 {
//					if _, ok := wanted[email[at+1:]]; ok {
//						out = append(out, refs[i])
//						break
//					}
//				}
//			}
//		}
//		return out
//	}
//
//	func dedupeRefs(refs []gmail.Ref) []gmail.Ref {
//		seen := map[string]struct{}{}
//		out := make([]gmail.Ref, 0, len(refs))
//		for _, ref := range refs {
//			key := string(ref.Entity) + ":" + ref.ID
//			if _, ok := seen[key]; ok {
//				continue
//			}
//			seen[key] = struct{}{}
//			out = append(out, ref)
//		}
//		return out
//	}
//
//	func compactSummary(item gmail.Item) string {
//		from := ""
//		if len(item.From) > 0 {
//			from = item.From[0].Email
//		}
//		text := item.Snippet
//		if strings.TrimSpace(text) == "" {
//			text = item.TextBody
//		}
//		text = strings.Join(strings.Fields(text), " ")
//		if len(text) > 220 {
//			text = text[:220]
//		}
//		return fmt.Sprintf("From=%s | Subject=%s | Summary=%s", from, item.Subject, text)
//	}
//
//	func looksPhishy(item gmail.Item, trustedDomains []string) bool {
//		if senderMatchesTrustedDomain(item.From, trustedDomains) {
//			return false
//		}
//		text := strings.ToLower(strings.Join([]string{item.Subject, item.Snippet, item.TextBody}, " "))
//		for _, kw := range []string{"verify your account", "suspended", "urgent action required", "password expired", "wire transfer", "gift card"} {
//			if strings.Contains(text, kw) {
//				return true
//			}
//		}
//		return false
//	}
//
//	func senderMatchesTrustedDomain(from []gmail.Address, trustedDomains []string) bool {
//		trusted := map[string]struct{}{}
//		for _, d := range trustedDomains {
//			d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
//			if d != "" {
//				trusted[d] = struct{}{}
//			}
//		}
//		for _, addr := range from {
//			email := strings.ToLower(strings.TrimSpace(addr.Email))
//			at := strings.LastIndex(email, "@")
//			if at >= 0 && at < len(email)-1 {
//				if _, ok := trusted[email[at+1:]]; ok {
//					return true
//				}
//			}
//		}
//		return false
//	}
package gmail
