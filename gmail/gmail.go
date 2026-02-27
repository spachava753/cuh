package gmail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/responses"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

const (
	gmailIMAPAddress = "imap.gmail.com:993"
	gmailSMTPHost    = "smtp.gmail.com"
	gmailSMTPAddress = "smtp.gmail.com:465"
	gmailAllMail     = "[Gmail]/All Mail"
	gmailTrash       = "[Gmail]/Trash"

	envGmailAddress     = "GMAIL_ADDRESS"
	envGmailAppPassword = "GMAIL_APP_PASSWORD"
)

const (
	fetchGmailThreadID  imap.FetchItem = "X-GM-THRID"
	fetchGmailMessageID imap.FetchItem = "X-GM-MSGID"
	fetchGmailLabels    imap.FetchItem = "X-GM-LABELS"
)

// Entity identifies the Gmail object type a primitive operates on.
//
// Entity controls both selection semantics in Find and hydration/mutation scope
// when a Ref is passed to Get or Mutate.
type Entity string

const (
	// EntityMessage scopes an operation to individual messages.
	EntityMessage Entity = "message"
	// EntityThread scopes an operation to thread references.
	EntityThread Entity = "thread"
)

// Ref is a stable handle used across primitives.
//
// Refs are produced by Find and consumed by Get and Mutate. For EntityMessage,
// ID is the Gmail message id (X-GM-MSGID). For EntityThread, ID is X-GM-THRID.
type Ref struct {
	Entity Entity
	ID     string
}

// MatchPolicy controls whether query clauses are ANDed or ORed.
//
// It is evaluated over populated top-level Query clauses.
type MatchPolicy string

const (
	// MatchAll requires every populated query clause to match.
	MatchAll MatchPolicy = "all"
	// MatchAny requires at least one populated query clause to match.
	MatchAny MatchPolicy = "any"
)

// Query captures typed search criteria.
//
// Example:
//
//	unread := false
//	q := gmail.Query{
//		Seen:            &unread,
//		From:            []string{"alerts@example.com"},
//		SubjectContains: []string{"invoice"},
//		InMailbox:       []string{"INBOX"},
//		Match:           gmail.MatchAll,
//	}
type Query struct {
	Text            string
	From            []string
	To              []string
	SubjectContains []string

	LabelAll  []string
	LabelAny  []string
	LabelNone []string

	Seen          *bool
	Starred       *bool
	Important     *bool
	HasAttachment *bool

	After  *time.Time
	Before *time.Time

	InMailbox []string
	ThreadID  string

	Match MatchPolicy
}

// Page controls paginated find results.
//
// Feed FindOutput.NextCursor back into Cursor to continue scanning.
type Page struct {
	Limit  int
	Cursor string
}

// SortField controls find ordering.
type SortField string

const (
	// SortByDate orders by message date.
	SortByDate SortField = "date"
	// SortByRelevance currently maps to server/date ordering suitable for ranking.
	SortByRelevance SortField = "relevance"
)

// SortOrder controls ascending or descending order.
type SortOrder string

const (
	// SortOrderAsc orders results oldest-first.
	SortOrderAsc SortOrder = "asc"
	// SortOrderDesc orders results newest-first.
	SortOrderDesc SortOrder = "desc"
)

// Sort controls find ordering behavior.
//
// Zero values default to SortByDate and SortOrderDesc.
type Sort struct {
	By    SortField
	Order SortOrder
}

// Address is a normalized email address.
type Address struct {
	Name  string
	Email string
}

// Meta is optional lightweight metadata returned from Find.
//
// Meta aligns index-by-index with FindOutput.Refs when IncludeMeta is true.
type Meta struct {
	Ref      Ref
	Subject  string
	From     []Address
	To       []Address
	Date     time.Time
	Labels   []string
	Snippet  string
	ThreadID string
}

// FindInput is the selection primitive input.
//
// Example:
//
//	findOut, err := gmail.Find(gmail.FindInput{
//		Entity: gmail.EntityMessage,
//		Query:  q,
//		Page:   gmail.Page{Limit: 25},
//		Sort:   gmail.Sort{By: gmail.SortByDate, Order: gmail.SortOrderDesc},
//	})
type FindInput struct {
	Entity      Entity
	Query       Query
	Page        Page
	Sort        Sort
	IncludeMeta bool
}

// FindOutput is the selection primitive output.
//
// NextCursor is empty when there are no more results in the current scan.
type FindOutput struct {
	Refs       []Ref
	Meta       []Meta
	NextCursor string
}

// Field controls which logical fields are desired from Get.
//
// Fields act as declarative intent for agents and callers; Get remains safe to
// return a richer item than requested when needed for protocol constraints.
type Field string

const (
	// FieldSubject requests the message subject.
	FieldSubject Field = "subject"
	// FieldFrom requests parsed sender addresses.
	FieldFrom Field = "from"
	// FieldTo requests parsed recipient addresses.
	FieldTo Field = "to"
	// FieldDate requests the message timestamp.
	FieldDate Field = "date"
	// FieldLabels requests Gmail labels for the message.
	FieldLabels Field = "labels"
	// FieldSnippet requests a short text summary.
	FieldSnippet Field = "snippet"
	// FieldTextBody requests plain-text message content.
	FieldTextBody Field = "text_body"
	// FieldHTMLBody requests HTML message content.
	FieldHTMLBody Field = "html_body"
	// FieldThreadID requests the Gmail thread ID.
	FieldThreadID Field = "thread_id"
	// FieldMessageID requests the RFC Message-ID header value.
	FieldMessageID Field = "message_id"
	// FieldInReplyTo requests the RFC In-Reply-To header value.
	FieldInReplyTo Field = "in_reply_to"
)

// BodyOptions controls body extraction behavior for Get.
//
// IncludeText enables MIME body parsing; MaxChars bounds TextBody/HTMLBody
// content size per item when greater than zero.
type BodyOptions struct {
	IncludeText bool
	MaxChars    int
}

// GetInput hydrates refs into typed items.
//
// Example:
//
//	getOut, err := gmail.Get(gmail.GetInput{
//		Refs:   findOut.Refs,
//		Fields: []gmail.Field{gmail.FieldSubject, gmail.FieldFrom, gmail.FieldSnippet},
//		Body:   gmail.BodyOptions{IncludeText: true, MaxChars: 4000},
//	})
type GetInput struct {
	Refs   []Ref
	Fields []Field
	Body   BodyOptions
}

// Item is the hydrated message model.
//
// Item is message-granular even when hydrated from a thread Ref.
type Item struct {
	Ref
	Subject   string
	From      []Address
	To        []Address
	Date      time.Time
	Labels    []string
	Snippet   string
	TextBody  string
	HTMLBody  string
	ThreadID  string
	MessageID string
	InReplyTo string
}

// GetOutput contains hydrated objects.
type GetOutput struct {
	Items []Item
}

// MutationType is the allowed state transition operation.
//
// Mutation types are explicit and composable; recipes should be assembled by
// sequencing Mutate calls or supplying multiple MutationOp values.
type MutationType string

const (
	// MutationAddLabel adds a label name in MutationOp.Value to each targeted ref.
	MutationAddLabel MutationType = "add_label"
	// MutationRemoveLabel removes a label name in MutationOp.Value.
	MutationRemoveLabel MutationType = "remove_label"
	// MutationSetSeen sets or clears the seen flag. Value defaults to true, or accepts "true"/"false".
	MutationSetSeen MutationType = "set_seen"
	// MutationSetStarred sets or clears the starred flag. Value defaults to true, or accepts "true"/"false".
	MutationSetStarred MutationType = "set_starred"
	// MutationMoveMailbox moves target refs to the mailbox in MutationOp.Value.
	MutationMoveMailbox MutationType = "move_mailbox"
	// MutationTrash moves target refs to Gmail Trash.
	MutationTrash MutationType = "trash"
	// MutationUntrash moves target refs out of Trash and back to All Mail.
	MutationUntrash MutationType = "untrash"
)

// MutationOp describes one state transition request.
//
// Value semantics depend on Type:
//   - add/remove label: label name
//   - set_seen/set_starred: optional bool string ("true" or "false")
//   - move_mailbox: destination mailbox name
type MutationOp struct {
	Type  MutationType
	Value string
}

// MutateInput applies operations to refs.
//
// Example:
//
//	_, err := gmail.Mutate(gmail.MutateInput{
//		Refs: findOut.Refs,
//		Ops: []gmail.MutationOp{
//			{Type: gmail.MutationAddLabel, Value: "Clients/Acme"},
//			{Type: gmail.MutationSetSeen, Value: "true"},
//		},
//		DryRun: true,
//	})
type MutateInput struct {
	Refs           []Ref
	Ops            []MutationOp
	DryRun         bool
	IdempotencyKey string
}

// MutationResult captures per-ref mutation status.
//
// Succeeded is false when one or more ops failed for that ref; Error contains
// a human-readable cause.
type MutationResult struct {
	Ref       Ref
	Applied   []MutationOp
	Succeeded bool
	Error     string
}

// MutateOutput captures mutation execution results.
//
// Results is always per-ref so agents can reason about partial success.
type MutateOutput struct {
	DryRun  bool
	Results []MutationResult
}

// OutgoingMessage is the transmission model for Send.
//
// Provide ReplyToRef for reply mode. If InReplyToMessageID/References are not
// set, Send derives threading headers from the reply target.
type OutgoingMessage struct {
	To       []string
	Cc       []string
	Bcc      []string
	Subject  string
	TextBody string
	HTMLBody string

	ReplyToRef *Ref

	InReplyToMessageID string
	References         []string
}

// SendInput is the send primitive input.
//
// DryRun validates recipients/body and returns a generated MessageID without
// transmitting mail.
type SendInput struct {
	Message OutgoingMessage
	DryRun  bool
}

// SendOutput is the send primitive output.
//
// ThreadID is populated when Send is used in reply mode.
type SendOutput struct {
	MessageID string
	ThreadID  string
}

// Label describes an available mailbox/label.
//
// Use Label.Name as the canonical value for MutationAddLabel/MutationRemoveLabel
// and Query label filters.
type Label struct {
	Name       string
	Delimiter  string
	Attributes []string
}

// LabelsOutput is the label catalog response.
//
// Labels are sorted by name for deterministic agent planning.
type LabelsOutput struct {
	Labels []Label
}

type fetchedMessage struct {
	UID            uint32
	GmailMessageID string
	ThreadID       string
	Envelope       *imap.Envelope
	Flags          []string
	Labels         []string
	InternalDate   time.Time
	BodyStructure  *imap.BodyStructure
	TextBody       string
	HTMLBody       string
}

// Find selects Gmail refs that match a typed query.
//
// Find is the primitive for selection only. It returns lightweight refs and
// optional aligned Meta records. Hydrate details with Get.
//
// Example:
//
//	unread := false
//	out, err := gmail.Find(gmail.FindInput{
//		Entity: gmail.EntityMessage,
//		Query: gmail.Query{
//			Seen:      &unread,
//			InMailbox: []string{"INBOX"},
//		},
//		Page:        gmail.Page{Limit: 50},
//		IncludeMeta: true,
//	})
func Find(input FindInput) (FindOutput, error) {
	entity, err := normalizeEntity(input.Entity)
	if err != nil {
		return FindOutput{}, err
	}

	limit := input.Page.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset, err := parseCursor(input.Page.Cursor)
	if err != nil {
		return FindOutput{}, err
	}

	address, appPassword, err := loadCredentials()
	if err != nil {
		return FindOutput{}, err
	}

	imapClient, err := connectIMAP(address, appPassword)
	if err != nil {
		return FindOutput{}, err
	}
	defer imapClient.Logout()

	mailboxes := normalizeMailboxes(input.Query.InMailbox)
	records := make([]fetchedMessage, 0, 256)
	for _, mailbox := range mailboxes {
		if _, err := imapClient.Select(mailbox, true); err != nil {
			return FindOutput{}, fmt.Errorf("gmail: selecting mailbox %q failed: %w", mailbox, err)
		}

		uids, err := findCandidateUIDs(imapClient, input.Query)
		if err != nil {
			return FindOutput{}, err
		}
		if len(uids) == 0 {
			continue
		}

		msgs, err := fetchMessagesByUID(imapClient, uids, false, input.Query.HasAttachment != nil)
		if err != nil {
			return FindOutput{}, err
		}
		for _, msg := range msgs {
			if messageMatchesQuery(msg, input.Query) {
				records = append(records, msg)
			}
		}
	}

	records = dedupeByEntity(records, entity)
	sortFetched(records, input.Sort)

	if offset > len(records) {
		offset = len(records)
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	window := records[offset:end]

	out := FindOutput{Refs: make([]Ref, 0, len(window))}
	if input.IncludeMeta {
		out.Meta = make([]Meta, 0, len(window))
	}
	for _, rec := range window {
		ref := buildRefFromFetched(rec, entity)
		out.Refs = append(out.Refs, ref)
		if input.IncludeMeta {
			out.Meta = append(out.Meta, Meta{
				Ref:      ref,
				Subject:  envelopeSubject(rec.Envelope),
				From:     convertAddresses(envelopeFrom(rec.Envelope)),
				To:       convertAddresses(envelopeTo(rec.Envelope)),
				Date:     envelopeDate(rec.Envelope, rec.InternalDate),
				Labels:   append([]string(nil), rec.Labels...),
				Snippet:  makeSnippet(rec.TextBody, rec.HTMLBody, envelopeSubject(rec.Envelope)),
				ThreadID: rec.ThreadID,
			})
		}
	}
	if end < len(records) {
		out.NextCursor = strconv.Itoa(end)
	}

	return out, nil
}

// Get hydrates refs into rich message items.
//
// Get accepts message refs and thread refs. Thread refs return one Item per
// message in that thread, sorted chronologically.
//
// Example:
//
//	getOut, err := gmail.Get(gmail.GetInput{
//		Refs:   out.Refs,
//		Fields: []gmail.Field{gmail.FieldSubject, gmail.FieldFrom, gmail.FieldTextBody},
//		Body:   gmail.BodyOptions{IncludeText: true, MaxChars: 6000},
//	})
func Get(input GetInput) (GetOutput, error) {
	if len(input.Refs) == 0 {
		return GetOutput{Items: []Item{}}, nil
	}

	fetchBody := shouldFetchBody(input)
	maxChars := input.Body.MaxChars

	address, appPassword, err := loadCredentials()
	if err != nil {
		return GetOutput{}, err
	}

	imapClient, err := connectIMAP(address, appPassword)
	if err != nil {
		return GetOutput{}, err
	}
	defer imapClient.Logout()

	if _, err := imapClient.Select(gmailAllMail, true); err != nil {
		return GetOutput{}, fmt.Errorf("gmail: selecting mailbox %q failed: %w", gmailAllMail, err)
	}

	items := make([]Item, 0, len(input.Refs))
	for _, ref := range input.Refs {
		normalizedRef, err := normalizeRef(ref)
		if err != nil {
			return GetOutput{}, err
		}

		uids, err := resolveRefUIDs(imapClient, normalizedRef)
		if err != nil {
			return GetOutput{}, err
		}
		if len(uids) == 0 {
			continue
		}

		msgs, err := fetchMessagesByUID(imapClient, uids, fetchBody, false)
		if err != nil {
			return GetOutput{}, err
		}
		sort.Slice(msgs, func(i, j int) bool {
			di := envelopeDate(msgs[i].Envelope, msgs[i].InternalDate)
			dj := envelopeDate(msgs[j].Envelope, msgs[j].InternalDate)
			if di.Equal(dj) {
				return msgs[i].UID < msgs[j].UID
			}
			return di.Before(dj)
		})

		for _, msg := range msgs {
			item := Item{
				Ref:       Ref{Entity: EntityMessage, ID: msg.GmailMessageID},
				Subject:   envelopeSubject(msg.Envelope),
				From:      convertAddresses(envelopeFrom(msg.Envelope)),
				To:        convertAddresses(envelopeTo(msg.Envelope)),
				Date:      envelopeDate(msg.Envelope, msg.InternalDate),
				Labels:    append([]string(nil), msg.Labels...),
				TextBody:  truncateString(msg.TextBody, maxChars),
				HTMLBody:  truncateString(msg.HTMLBody, maxChars),
				ThreadID:  msg.ThreadID,
				MessageID: envelopeMessageID(msg.Envelope),
				InReplyTo: envelopeInReplyTo(msg.Envelope),
			}
			item.Snippet = truncateString(makeSnippet(item.TextBody, item.HTMLBody, item.Subject), maxChars)
			items = append(items, item)
		}
	}

	return GetOutput{Items: items}, nil
}

// Mutate applies explicit state transition operations to refs.
//
// Mutate is the only primitive for mailbox/label/flag state changes. Use
// DryRun to validate intent before changing mailbox state.
//
// Example:
//
//	mutOut, err := gmail.Mutate(gmail.MutateInput{
//		Refs: out.Refs,
//		Ops: []gmail.MutationOp{
//			{Type: gmail.MutationAddLabel, Value: "Processed"},
//			{Type: gmail.MutationSetSeen, Value: "true"},
//		},
//	})
func Mutate(input MutateInput) (MutateOutput, error) {
	if len(input.Refs) == 0 {
		return MutateOutput{DryRun: input.DryRun, Results: []MutationResult{}}, nil
	}
	if len(input.Ops) == 0 {
		return MutateOutput{}, errors.New("gmail: ops are required")
	}

	out := MutateOutput{
		DryRun:  input.DryRun,
		Results: make([]MutationResult, 0, len(input.Refs)),
	}
	if input.DryRun {
		for _, ref := range input.Refs {
			normalizedRef, err := normalizeRef(ref)
			if err != nil {
				return MutateOutput{}, err
			}
			out.Results = append(out.Results, MutationResult{
				Ref:       normalizedRef,
				Applied:   append([]MutationOp(nil), input.Ops...),
				Succeeded: true,
			})
		}
		return out, nil
	}

	address, appPassword, err := loadCredentials()
	if err != nil {
		return MutateOutput{}, err
	}

	imapClient, err := connectIMAP(address, appPassword)
	if err != nil {
		return MutateOutput{}, err
	}
	defer imapClient.Logout()

	if _, err := imapClient.Select(gmailAllMail, false); err != nil {
		return MutateOutput{}, fmt.Errorf("gmail: selecting mailbox %q failed: %w", gmailAllMail, err)
	}

	for _, ref := range input.Refs {
		normalizedRef, err := normalizeRef(ref)
		if err != nil {
			return MutateOutput{}, err
		}
		result := MutationResult{Ref: normalizedRef}

		uids, err := resolveRefUIDs(imapClient, normalizedRef)
		if err != nil {
			result.Error = err.Error()
			out.Results = append(out.Results, result)
			continue
		}
		if len(uids) == 0 {
			result.Error = fmt.Sprintf("gmail: ref %s not found", normalizedRef.ID)
			out.Results = append(out.Results, result)
			continue
		}

		seqSet := new(imap.SeqSet)
		seqSet.AddNum(uids...)

		succeeded := true
		for _, op := range input.Ops {
			if err := applyMutationOp(imapClient, seqSet, op); err != nil {
				result.Error = err.Error()
				succeeded = false
				break
			}
			result.Applied = append(result.Applied, op)
		}
		result.Succeeded = succeeded
		out.Results = append(out.Results, result)

		if _, err := imapClient.Select(gmailAllMail, false); err != nil {
			return MutateOutput{}, fmt.Errorf("gmail: re-selecting mailbox %q failed: %w", gmailAllMail, err)
		}
	}

	return out, nil
}

// Send transmits a new or reply email.
//
// Set OutgoingMessage.ReplyToRef to send a reply with derived threading
// headers. For a new message, omit ReplyToRef.
//
// Example:
//
//	sendOut, err := gmail.Send(gmail.SendInput{
//		Message: gmail.OutgoingMessage{
//			To:       []string{"me@example.com"},
//			Subject:  "Status",
//			TextBody: "Done.",
//		},
//	})
func Send(input SendInput) (SendOutput, error) {
	msg := input.Message
	if err := validateOutgoingMessage(msg); err != nil {
		return SendOutput{}, err
	}

	from, appPassword, err := loadCredentials()
	if err != nil {
		return SendOutput{}, err
	}

	threadID := ""
	if msg.ReplyToRef != nil {
		item, err := lookupReplyTarget(*msg.ReplyToRef)
		if err != nil {
			return SendOutput{}, err
		}
		threadID = item.ThreadID
		if strings.TrimSpace(msg.InReplyToMessageID) == "" {
			msg.InReplyToMessageID = item.MessageID
		}
		if len(msg.References) == 0 && item.MessageID != "" {
			msg.References = []string{item.MessageID}
		}
	}

	messageID := generateMessageID(from)
	if input.DryRun {
		return SendOutput{MessageID: messageID, ThreadID: threadID}, nil
	}

	rawMessage, err := buildOutgoingMessage(from, msg, messageID)
	if err != nil {
		return SendOutput{}, err
	}

	recipients := uniqueRecipients(msg.To, msg.Cc, msg.Bcc)
	smtpClient, err := connectSMTP(from, appPassword)
	if err != nil {
		return SendOutput{}, err
	}
	defer smtpClient.Close()

	if err := smtpClient.Mail(from, nil); err != nil {
		return SendOutput{}, fmt.Errorf("gmail: MAIL FROM failed: %w", err)
	}
	for _, rcpt := range recipients {
		if err := smtpClient.Rcpt(rcpt, nil); err != nil {
			return SendOutput{}, fmt.Errorf("gmail: RCPT TO %q failed: %w", rcpt, err)
		}
	}

	writer, err := smtpClient.Data()
	if err != nil {
		return SendOutput{}, fmt.Errorf("gmail: DATA failed: %w", err)
	}
	if _, err := writer.Write(rawMessage); err != nil {
		return SendOutput{}, fmt.Errorf("gmail: writing message failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return SendOutput{}, fmt.Errorf("gmail: finalizing message failed: %w", err)
	}
	if err := smtpClient.Quit(); err != nil {
		return SendOutput{}, fmt.Errorf("gmail: QUIT failed: %w", err)
	}

	return SendOutput{MessageID: messageID, ThreadID: threadID}, nil
}

// Labels returns the available Gmail labels/mailboxes catalog.
//
// Call Labels before writing label-based recipes so agents can use canonical
// label names instead of guessing.
//
// Example:
//
//	labelsOut, err := gmail.Labels()
func Labels() (LabelsOutput, error) {
	address, appPassword, err := loadCredentials()
	if err != nil {
		return LabelsOutput{}, err
	}

	imapClient, err := connectIMAP(address, appPassword)
	if err != nil {
		return LabelsOutput{}, err
	}
	defer imapClient.Logout()

	ch := make(chan *imap.MailboxInfo, 128)
	done := make(chan error, 1)
	go func() {
		done <- imapClient.List("", "*", ch)
	}()

	labels := make([]Label, 0, 64)
	for mailbox := range ch {
		entry := Label{
			Name:       mailbox.Name,
			Delimiter:  mailbox.Delimiter,
			Attributes: append([]string(nil), mailbox.Attributes...),
		}
		sort.Strings(entry.Attributes)
		labels = append(labels, entry)
	}
	if err := <-done; err != nil {
		return LabelsOutput{}, fmt.Errorf("gmail: listing labels failed: %w", err)
	}

	sort.Slice(labels, func(i, j int) bool {
		return labels[i].Name < labels[j].Name
	})
	return LabelsOutput{Labels: labels}, nil
}

func normalizeEntity(entity Entity) (Entity, error) {
	if entity == "" {
		return EntityMessage, nil
	}
	switch entity {
	case EntityMessage, EntityThread:
		return entity, nil
	default:
		return "", fmt.Errorf("gmail: unsupported entity %q", entity)
	}
}

func normalizeRef(ref Ref) (Ref, error) {
	entity, err := normalizeEntity(ref.Entity)
	if err != nil {
		return Ref{}, err
	}
	id := strings.TrimSpace(ref.ID)
	if id == "" {
		return Ref{}, errors.New("gmail: ref id is required")
	}
	return Ref{Entity: entity, ID: id}, nil
}

func parseCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("gmail: invalid cursor %q", cursor)
	}
	return offset, nil
}

func normalizeMailboxes(mailboxes []string) []string {
	if len(mailboxes) == 0 {
		return []string{gmailAllMail}
	}
	out := make([]string, 0, len(mailboxes))
	seen := map[string]struct{}{}
	for _, mailbox := range mailboxes {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			continue
		}
		if _, ok := seen[mailbox]; ok {
			continue
		}
		seen[mailbox] = struct{}{}
		out = append(out, mailbox)
	}
	if len(out) == 0 {
		return []string{gmailAllMail}
	}
	return out
}

func findCandidateUIDs(imapClient *client.Client, query Query) ([]uint32, error) {
	baseCriteria := buildSearchCriteria(query)
	baseUIDs, err := imapClient.UidSearch(baseCriteria)
	if err != nil {
		return nil, fmt.Errorf("gmail: searching messages failed: %w", err)
	}

	if strings.TrimSpace(query.ThreadID) == "" {
		return baseUIDs, nil
	}

	threadUIDs, err := searchUIDByXGM(imapClient, "X-GM-THRID", strings.TrimSpace(query.ThreadID))
	if err != nil {
		return nil, err
	}
	if len(baseUIDs) == 0 || len(threadUIDs) == 0 {
		return []uint32{}, nil
	}
	return intersectUIDs(baseUIDs, threadUIDs), nil
}

func buildSearchCriteria(query Query) *imap.SearchCriteria {
	criteria := imap.NewSearchCriteria()
	if query.Match == MatchAny {
		// OR policy cannot be represented precisely with IMAP criteria; broad fetch then filter.
		return criteria
	}

	if query.After != nil {
		criteria.Since = *query.After
	}
	if query.Before != nil {
		criteria.Before = *query.Before
	}
	for _, text := range stringsForSearch(query.Text) {
		criteria.Text = append(criteria.Text, text)
	}
	for _, from := range stringsForSearch(query.From...) {
		criteria.Header.Add("FROM", from)
	}
	for _, to := range stringsForSearch(query.To...) {
		criteria.Header.Add("TO", to)
	}
	for _, subject := range stringsForSearch(query.SubjectContains...) {
		criteria.Header.Add("SUBJECT", subject)
	}
	if query.Seen != nil {
		if *query.Seen {
			criteria.WithFlags = append(criteria.WithFlags, imap.SeenFlag)
		} else {
			criteria.WithoutFlags = append(criteria.WithoutFlags, imap.SeenFlag)
		}
	}
	if query.Starred != nil {
		if *query.Starred {
			criteria.WithFlags = append(criteria.WithFlags, imap.FlaggedFlag)
		} else {
			criteria.WithoutFlags = append(criteria.WithoutFlags, imap.FlaggedFlag)
		}
	}
	if query.Important != nil {
		if *query.Important {
			criteria.WithFlags = append(criteria.WithFlags, imap.ImportantFlag)
		} else {
			criteria.WithoutFlags = append(criteria.WithoutFlags, imap.ImportantFlag)
		}
	}
	return criteria
}

func fetchMessagesByUID(imapClient *client.Client, uids []uint32, includeBody bool, includeBodyStructure bool) ([]fetchedMessage, error) {
	if len(uids) == 0 {
		return []fetchedMessage{}, nil
	}
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uids...)

	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, fetchGmailThreadID, fetchGmailMessageID, fetchGmailLabels}
	if includeBodyStructure {
		items = append(items, imap.FetchBodyStructure)
	}
	var bodySection *imap.BodySectionName
	if includeBody {
		bodySection = &imap.BodySectionName{Peek: true}
		items = append(items, bodySection.FetchItem())
	}

	messages := make(chan *imap.Message, len(uids)+8)
	done := make(chan error, 1)
	go func() {
		done <- imapClient.UidFetch(seqSet, items, messages)
	}()

	out := make([]fetchedMessage, 0, len(uids))
	for msg := range messages {
		entry := fetchedMessage{
			UID:            msg.Uid,
			GmailMessageID: parseIDValue(msg.Items[fetchGmailMessageID]),
			ThreadID:       parseIDValue(msg.Items[fetchGmailThreadID]),
			Envelope:       msg.Envelope,
			Flags:          append([]string(nil), msg.Flags...),
			Labels:         parseLabels(msg.Items[fetchGmailLabels]),
			InternalDate:   msg.InternalDate,
			BodyStructure:  msg.BodyStructure,
		}
		if entry.GmailMessageID == "" {
			entry.GmailMessageID = strconv.FormatUint(uint64(entry.UID), 10)
		}

		if bodySection != nil {
			if literal := msg.GetBody(bodySection); literal != nil {
				raw, err := io.ReadAll(literal)
				if err != nil {
					return nil, fmt.Errorf("gmail: reading fetched body failed: %w", err)
				}
				textBody, htmlBody, err := extractBodiesFromRaw(raw)
				if err != nil {
					return nil, fmt.Errorf("gmail: parsing fetched body failed: %w", err)
				}
				entry.TextBody = textBody
				entry.HTMLBody = htmlBody
			}
		}

		out = append(out, entry)
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("gmail: fetching messages failed: %w", err)
	}
	return out, nil
}

func messageMatchesQuery(msg fetchedMessage, query Query) bool {
	conditions := make([]bool, 0, 10)

	if strings.TrimSpace(query.Text) != "" {
		haystack := strings.ToLower(strings.Join([]string{
			envelopeSubject(msg.Envelope),
			joinAddressTokens(convertAddresses(envelopeFrom(msg.Envelope))),
			joinAddressTokens(convertAddresses(envelopeTo(msg.Envelope))),
			strings.Join(msg.Labels, " "),
		}, " "))
		conditions = append(conditions, strings.Contains(haystack, strings.ToLower(strings.TrimSpace(query.Text))))
	}
	if len(query.From) > 0 {
		conditions = append(conditions, matchAnyAddress(query.From, convertAddresses(envelopeFrom(msg.Envelope))))
	}
	if len(query.To) > 0 {
		conditions = append(conditions, matchAnyAddress(query.To, convertAddresses(envelopeTo(msg.Envelope))))
	}
	if len(query.SubjectContains) > 0 {
		subject := strings.ToLower(envelopeSubject(msg.Envelope))
		matchAll := true
		for _, needle := range query.SubjectContains {
			needle = strings.ToLower(strings.TrimSpace(needle))
			if needle == "" {
				continue
			}
			if !strings.Contains(subject, needle) {
				matchAll = false
				break
			}
		}
		conditions = append(conditions, matchAll)
	}
	if len(query.LabelAll) > 0 {
		conditions = append(conditions, labelsContainAll(msg.Labels, query.LabelAll))
	}
	if len(query.LabelAny) > 0 {
		conditions = append(conditions, labelsContainAny(msg.Labels, query.LabelAny))
	}
	if len(query.LabelNone) > 0 {
		conditions = append(conditions, !labelsContainAny(msg.Labels, query.LabelNone))
	}
	if query.Seen != nil {
		conditions = append(conditions, hasFlag(msg.Flags, imap.SeenFlag) == *query.Seen)
	}
	if query.Starred != nil {
		conditions = append(conditions, hasFlag(msg.Flags, imap.FlaggedFlag) == *query.Starred)
	}
	if query.Important != nil {
		important := hasFlag(msg.Flags, imap.ImportantFlag) || labelsContainAny(msg.Labels, []string{imap.ImportantAttr})
		conditions = append(conditions, important == *query.Important)
	}
	if query.HasAttachment != nil {
		conditions = append(conditions, bodyHasAttachment(msg.BodyStructure) == *query.HasAttachment)
	}
	if query.After != nil {
		conditions = append(conditions, !envelopeDate(msg.Envelope, msg.InternalDate).Before(*query.After))
	}
	if query.Before != nil {
		conditions = append(conditions, envelopeDate(msg.Envelope, msg.InternalDate).Before(*query.Before))
	}
	if strings.TrimSpace(query.ThreadID) != "" {
		conditions = append(conditions, strings.TrimSpace(msg.ThreadID) == strings.TrimSpace(query.ThreadID))
	}

	if len(conditions) == 0 {
		return true
	}
	if query.Match == MatchAny {
		for _, condition := range conditions {
			if condition {
				return true
			}
		}
		return false
	}
	for _, condition := range conditions {
		if !condition {
			return false
		}
	}
	return true
}

func dedupeByEntity(messages []fetchedMessage, entity Entity) []fetchedMessage {
	if entity == EntityThread {
		byThread := make(map[string]fetchedMessage, len(messages))
		for _, message := range messages {
			if message.ThreadID == "" {
				continue
			}
			if existing, ok := byThread[message.ThreadID]; !ok || envelopeDate(message.Envelope, message.InternalDate).After(envelopeDate(existing.Envelope, existing.InternalDate)) {
				byThread[message.ThreadID] = message
			}
		}
		out := make([]fetchedMessage, 0, len(byThread))
		for _, message := range byThread {
			out = append(out, message)
		}
		return out
	}

	byMessage := make(map[string]fetchedMessage, len(messages))
	for _, message := range messages {
		key := message.GmailMessageID
		if key == "" {
			key = strconv.FormatUint(uint64(message.UID), 10)
		}
		if _, exists := byMessage[key]; !exists {
			byMessage[key] = message
		}
	}
	out := make([]fetchedMessage, 0, len(byMessage))
	for _, message := range byMessage {
		out = append(out, message)
	}
	return out
}

func sortFetched(messages []fetchedMessage, order Sort) {
	field := order.By
	if field == "" {
		field = SortByDate
	}
	direction := order.Order
	if direction == "" {
		direction = SortOrderDesc
	}

	sort.Slice(messages, func(i, j int) bool {
		if field == SortByDate || field == SortByRelevance {
			di := envelopeDate(messages[i].Envelope, messages[i].InternalDate)
			dj := envelopeDate(messages[j].Envelope, messages[j].InternalDate)
			if di.Equal(dj) {
				if direction == SortOrderAsc {
					return messages[i].GmailMessageID < messages[j].GmailMessageID
				}
				return messages[i].GmailMessageID > messages[j].GmailMessageID
			}
			if direction == SortOrderAsc {
				return di.Before(dj)
			}
			return di.After(dj)
		}
		return messages[i].GmailMessageID < messages[j].GmailMessageID
	})
}

func buildRefFromFetched(msg fetchedMessage, entity Entity) Ref {
	if entity == EntityThread {
		return Ref{Entity: EntityThread, ID: msg.ThreadID}
	}
	return Ref{Entity: EntityMessage, ID: msg.GmailMessageID}
}

func shouldFetchBody(input GetInput) bool {
	if input.Body.IncludeText {
		return true
	}
	for _, field := range input.Fields {
		if field == FieldTextBody || field == FieldHTMLBody || field == FieldSnippet {
			return true
		}
	}
	return false
}

func resolveRefUIDs(imapClient *client.Client, ref Ref) ([]uint32, error) {
	switch ref.Entity {
	case EntityThread:
		uids, err := searchUIDByXGM(imapClient, "X-GM-THRID", ref.ID)
		if err != nil {
			return nil, err
		}
		return uids, nil
	case EntityMessage:
		uids, err := searchUIDByXGM(imapClient, "X-GM-MSGID", ref.ID)
		if err != nil {
			return nil, err
		}
		if len(uids) > 0 {
			return uids, nil
		}
		uid, parseErr := strconv.ParseUint(ref.ID, 10, 32)
		if parseErr == nil {
			return []uint32{uint32(uid)}, nil
		}
		return []uint32{}, nil
	default:
		return nil, fmt.Errorf("gmail: unsupported entity %q", ref.Entity)
	}
}

func applyMutationOp(imapClient *client.Client, seqSet *imap.SeqSet, op MutationOp) error {
	switch op.Type {
	case MutationAddLabel:
		label := strings.TrimSpace(op.Value)
		if label == "" {
			return errors.New("gmail: add_label requires value")
		}
		cmd := &gmailStoreLabels{SeqSet: seqSet, Add: true, Labels: []string{label}}
		if _, err := imapClient.Execute(cmd, nil); err != nil {
			return fmt.Errorf("gmail: add_label failed: %w", err)
		}
		return nil
	case MutationRemoveLabel:
		label := strings.TrimSpace(op.Value)
		if label == "" {
			return errors.New("gmail: remove_label requires value")
		}
		cmd := &gmailStoreLabels{SeqSet: seqSet, Add: false, Labels: []string{label}}
		if _, err := imapClient.Execute(cmd, nil); err != nil {
			return fmt.Errorf("gmail: remove_label failed: %w", err)
		}
		return nil
	case MutationSetSeen:
		seen, err := parseMutationBool(op.Value, true)
		if err != nil {
			return err
		}
		storeItem := imap.FormatFlagsOp(imap.AddFlags, true)
		if !seen {
			storeItem = imap.FormatFlagsOp(imap.RemoveFlags, true)
		}
		if err := imapClient.UidStore(seqSet, storeItem, []any{imap.SeenFlag}, nil); err != nil {
			return fmt.Errorf("gmail: set_seen failed: %w", err)
		}
		return nil
	case MutationSetStarred:
		starred, err := parseMutationBool(op.Value, true)
		if err != nil {
			return err
		}
		storeItem := imap.FormatFlagsOp(imap.AddFlags, true)
		if !starred {
			storeItem = imap.FormatFlagsOp(imap.RemoveFlags, true)
		}
		if err := imapClient.UidStore(seqSet, storeItem, []any{imap.FlaggedFlag}, nil); err != nil {
			return fmt.Errorf("gmail: set_starred failed: %w", err)
		}
		return nil
	case MutationMoveMailbox:
		mailbox := strings.TrimSpace(op.Value)
		if mailbox == "" {
			return errors.New("gmail: move_mailbox requires value")
		}
		if err := uidMove(imapClient, seqSet, mailbox); err != nil {
			return fmt.Errorf("gmail: move_mailbox failed: %w", err)
		}
		return nil
	case MutationTrash:
		if err := uidMove(imapClient, seqSet, gmailTrash); err != nil {
			return fmt.Errorf("gmail: trash failed: %w", err)
		}
		return nil
	case MutationUntrash:
		if _, err := imapClient.Select(gmailTrash, false); err != nil {
			return fmt.Errorf("gmail: selecting trash failed: %w", err)
		}
		if err := uidMove(imapClient, seqSet, gmailAllMail); err != nil {
			return fmt.Errorf("gmail: untrash failed: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("gmail: unsupported mutation op %q", op.Type)
	}
}

func parseMutationBool(value string, defaultValue bool) (bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("gmail: invalid bool value %q", value)
	}
	return parsed, nil
}

func uidMove(imapClient *client.Client, seqSet *imap.SeqSet, destination string) error {
	if err := imapClient.UidMove(seqSet, destination); err == nil {
		return nil
	}
	if err := imapClient.UidCopy(seqSet, destination); err != nil {
		return err
	}
	storeItem := imap.FormatFlagsOp(imap.AddFlags, true)
	if err := imapClient.UidStore(seqSet, storeItem, []any{imap.DeletedFlag}, nil); err != nil {
		return err
	}
	if err := imapClient.Expunge(nil); err != nil {
		return err
	}
	return nil
}

func lookupReplyTarget(ref Ref) (Item, error) {
	getOut, err := Get(GetInput{
		Refs:   []Ref{ref},
		Fields: []Field{FieldMessageID, FieldThreadID, FieldSubject},
	})
	if err != nil {
		return Item{}, err
	}
	if len(getOut.Items) == 0 {
		return Item{}, fmt.Errorf("gmail: reply target %s not found", ref.ID)
	}
	return getOut.Items[len(getOut.Items)-1], nil
}

func validateOutgoingMessage(msg OutgoingMessage) error {
	recipients := uniqueRecipients(msg.To, msg.Cc, msg.Bcc)
	if len(recipients) == 0 {
		return errors.New("gmail: at least one recipient is required")
	}
	if strings.TrimSpace(msg.TextBody) == "" && strings.TrimSpace(msg.HTMLBody) == "" {
		return errors.New("gmail: either text body or html body is required")
	}
	return nil
}

func uniqueRecipients(groups ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, group := range groups {
		for _, recipient := range group {
			recipient = strings.TrimSpace(recipient)
			if recipient == "" {
				continue
			}
			if _, ok := seen[recipient]; ok {
				continue
			}
			seen[recipient] = struct{}{}
			out = append(out, recipient)
		}
	}
	return out
}

func buildOutgoingMessage(from string, msg OutgoingMessage, messageID string) ([]byte, error) {
	subject := sanitizeHeader(msg.Subject)
	if subject == "" {
		subject = "(no subject)"
	}

	headers := []string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", strings.Join(uniqueRecipients(msg.To), ", ")),
		fmt.Sprintf("Subject: %s", subject),
		fmt.Sprintf("Date: %s", time.Now().Format(time.RFC1123Z)),
		fmt.Sprintf("Message-ID: %s", normalizeMessageID(messageID)),
		"MIME-Version: 1.0",
	}
	if cc := uniqueRecipients(msg.Cc); len(cc) > 0 {
		headers = append(headers, fmt.Sprintf("Cc: %s", strings.Join(cc, ", ")))
	}
	if inReplyTo := normalizeMessageID(msg.InReplyToMessageID); inReplyTo != "" {
		headers = append(headers, fmt.Sprintf("In-Reply-To: %s", inReplyTo))
	}
	if len(msg.References) > 0 {
		references := make([]string, 0, len(msg.References))
		for _, reference := range msg.References {
			normalized := normalizeMessageID(reference)
			if normalized != "" {
				references = append(references, normalized)
			}
		}
		if len(references) > 0 {
			headers = append(headers, fmt.Sprintf("References: %s", strings.Join(references, " ")))
		}
	}

	textBody := normalizeBody(msg.TextBody)
	htmlBody := normalizeBody(msg.HTMLBody)

	if textBody != "" && htmlBody != "" {
		boundary := fmt.Sprintf("cuh-gmail-%d", time.Now().UnixNano())
		headers = append(headers, fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q", boundary))
		var builder strings.Builder
		builder.WriteString(strings.Join(headers, "\r\n"))
		builder.WriteString("\r\n\r\n")
		builder.WriteString("--" + boundary + "\r\n")
		builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		builder.WriteString(textBody)
		builder.WriteString("\r\n")
		builder.WriteString("--" + boundary + "\r\n")
		builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		builder.WriteString(htmlBody)
		builder.WriteString("\r\n")
		builder.WriteString("--" + boundary + "--\r\n")
		return []byte(builder.String()), nil
	}

	if htmlBody != "" {
		headers = append(headers, "Content-Type: text/html; charset=UTF-8")
		return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + htmlBody + "\r\n"), nil
	}

	headers = append(headers, "Content-Type: text/plain; charset=UTF-8")
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + textBody + "\r\n"), nil
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func normalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	return strings.TrimSpace(body)
}

func generateMessageID(address string) string {
	domain := "localhost"
	if at := strings.LastIndex(address, "@"); at >= 0 && at < len(address)-1 {
		domain = address[at+1:]
	}
	return fmt.Sprintf("<%d.%s>", time.Now().UnixNano(), domain)
}

func normalizeMessageID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return value
	}
	return "<" + strings.Trim(value, "<>") + ">"
}

func extractBodiesFromRaw(raw []byte) (string, string, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return "", "", err
	}
	textBody, htmlBody, err := extractBodiesFromEntity(textproto.MIMEHeader(msg.Header), body)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(textBody), strings.TrimSpace(htmlBody), nil
}

func extractBodiesFromEntity(header textproto.MIMEHeader, body []byte) (string, string, error) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		mediaType = "text/plain"
	}

	decoded, err := decodeTransferEncoding(header.Get("Content-Transfer-Encoding"), body)
	if err != nil {
		return "", "", err
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return "", "", nil
		}
		reader := multipart.NewReader(bytes.NewReader(decoded), boundary)
		plainParts := make([]string, 0, 4)
		htmlParts := make([]string, 0, 2)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", "", err
			}
			partBody, err := io.ReadAll(part)
			if err != nil {
				return "", "", err
			}
			partText, partHTML, err := extractBodiesFromEntity(textproto.MIMEHeader(part.Header), partBody)
			if err != nil {
				return "", "", err
			}
			if partText != "" {
				plainParts = append(plainParts, partText)
			}
			if partHTML != "" {
				htmlParts = append(htmlParts, partHTML)
			}
		}
		return strings.Join(plainParts, "\n"), strings.Join(htmlParts, "\n"), nil
	}

	switch mediaType {
	case "text/plain":
		return string(decoded), "", nil
	case "text/html":
		return "", string(decoded), nil
	case "message/rfc822":
		nested, err := mail.ReadMessage(bytes.NewReader(decoded))
		if err != nil {
			return "", "", err
		}
		nestedBody, err := io.ReadAll(nested.Body)
		if err != nil {
			return "", "", err
		}
		return extractBodiesFromEntity(textproto.MIMEHeader(nested.Header), nestedBody)
	default:
		return "", "", nil
	}
}

func decodeTransferEncoding(encoding string, body []byte) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "7bit", "8bit", "binary":
		return body, nil
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(body))
		return io.ReadAll(reader)
	case "base64":
		clean := strings.ReplaceAll(string(body), "\r", "")
		clean = strings.ReplaceAll(clean, "\n", "")
		decoded, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return body, nil
		}
		return decoded, nil
	default:
		return body, nil
	}
}

func makeSnippet(textBody string, htmlBody string, fallback string) string {
	source := strings.TrimSpace(textBody)
	if source == "" {
		source = strings.TrimSpace(htmlBody)
	}
	if source == "" {
		source = strings.TrimSpace(fallback)
	}
	source = strings.Join(strings.Fields(source), " ")
	return truncateString(source, 240)
}

func truncateString(value string, maxChars int) string {
	if maxChars <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	return string(runes[:maxChars])
}

func matchAnyAddress(needles []string, addresses []Address) bool {
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle == "" {
			continue
		}
		for _, address := range addresses {
			hay := strings.ToLower(strings.TrimSpace(address.Name + " " + address.Email))
			if strings.Contains(hay, needle) {
				return true
			}
		}
	}
	return false
}

func labelsContainAll(existing []string, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	set := map[string]struct{}{}
	for _, label := range existing {
		set[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
	}
	for _, label := range expected {
		if _, ok := set[strings.ToLower(strings.TrimSpace(label))]; !ok {
			return false
		}
	}
	return true
}

func labelsContainAny(existing []string, candidates []string) bool {
	if len(candidates) == 0 {
		return false
	}
	set := map[string]struct{}{}
	for _, label := range existing {
		set[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
	}
	for _, label := range candidates {
		if _, ok := set[strings.ToLower(strings.TrimSpace(label))]; ok {
			return true
		}
	}
	return false
}

func hasFlag(flags []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, flag := range flags {
		if strings.ToLower(strings.TrimSpace(flag)) == target {
			return true
		}
	}
	return false
}

func bodyHasAttachment(body *imap.BodyStructure) bool {
	if body == nil {
		return false
	}
	if strings.EqualFold(body.Disposition, "attachment") {
		return true
	}
	if filename, err := body.Filename(); err == nil && strings.TrimSpace(filename) != "" {
		return true
	}
	for _, part := range body.Parts {
		if bodyHasAttachment(part) {
			return true
		}
	}
	if body.BodyStructure != nil {
		return bodyHasAttachment(body.BodyStructure)
	}
	return false
}

func intersectUIDs(a []uint32, b []uint32) []uint32 {
	set := make(map[uint32]struct{}, len(a))
	for _, id := range a {
		set[id] = struct{}{}
	}
	out := make([]uint32, 0, min(len(a), len(b)))
	for _, id := range b {
		if _, ok := set[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

func searchUIDByXGM(imapClient *client.Client, atom string, value string) ([]uint32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("gmail: search value is required")
	}

	searchCmd := &gmailSearch{Atom: atom, Value: value}
	searchResp := &responses.Search{}
	if _, err := imapClient.Execute(searchCmd, searchResp); err != nil {
		return nil, fmt.Errorf("gmail: searching %s failed: %w", atom, err)
	}
	return searchResp.Ids, nil
}

func parseIDValue(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case uint64:
		return strconv.FormatUint(value, 10)
	case int64:
		return strconv.FormatInt(value, 10)
	case int:
		return strconv.Itoa(value)
	case string:
		return value
	default:
		return fmt.Sprintf("%v", value)
	}
}

func parseLabels(v any) []string {
	switch value := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), value...)
	case []any:
		labels := make([]string, 0, len(value))
		for _, raw := range value {
			labels = append(labels, fmt.Sprintf("%v", raw))
		}
		return labels
	case string:
		if value == "" {
			return nil
		}
		return []string{value}
	default:
		return []string{fmt.Sprintf("%v", value)}
	}
}

func convertAddresses(addrs []*imap.Address) []Address {
	out := make([]Address, 0, len(addrs))
	for _, addr := range addrs {
		if addr == nil {
			continue
		}
		out = append(out, Address{
			Name:  strings.TrimSpace(addr.PersonalName),
			Email: strings.TrimSpace(addr.Address()),
		})
	}
	return out
}

func joinAddressTokens(addrs []Address) string {
	parts := make([]string, 0, len(addrs)*2)
	for _, addr := range addrs {
		if addr.Name != "" {
			parts = append(parts, addr.Name)
		}
		if addr.Email != "" {
			parts = append(parts, addr.Email)
		}
	}
	return strings.Join(parts, " ")
}

func stringsForSearch(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func envelopeSubject(env *imap.Envelope) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(env.Subject)
}

func envelopeFrom(env *imap.Envelope) []*imap.Address {
	if env == nil {
		return nil
	}
	return env.From
}

func envelopeTo(env *imap.Envelope) []*imap.Address {
	if env == nil {
		return nil
	}
	return env.To
}

func envelopeDate(env *imap.Envelope, fallback time.Time) time.Time {
	if env != nil && !env.Date.IsZero() {
		return env.Date
	}
	return fallback
}

func envelopeMessageID(env *imap.Envelope) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(env.MessageId)
}

func envelopeInReplyTo(env *imap.Envelope) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(env.InReplyTo)
}

func loadCredentials() (address string, appPassword string, err error) {
	address = strings.TrimSpace(os.Getenv(envGmailAddress))
	if address == "" {
		return "", "", fmt.Errorf("gmail: %s is required", envGmailAddress)
	}

	appPassword = strings.ReplaceAll(os.Getenv(envGmailAppPassword), " ", "")
	if appPassword == "" {
		return "", "", fmt.Errorf("gmail: %s is required", envGmailAppPassword)
	}

	return address, appPassword, nil
}

func connectIMAP(address string, appPassword string) (*client.Client, error) {
	imapClient, err := client.DialTLS(gmailIMAPAddress, &tls.Config{ServerName: "imap.gmail.com"})
	if err != nil {
		return nil, fmt.Errorf("gmail: IMAP dial failed: %w", err)
	}

	if err := imapClient.Login(address, appPassword); err != nil {
		imapClient.Logout()
		return nil, fmt.Errorf("gmail: IMAP login failed: %w", err)
	}

	return imapClient, nil
}

func connectSMTP(address string, appPassword string) (*smtp.Client, error) {
	conn, err := tls.Dial("tcp", gmailSMTPAddress, &tls.Config{ServerName: gmailSMTPHost})
	if err != nil {
		return nil, fmt.Errorf("gmail: SMTP TLS dial failed: %w", err)
	}

	smtpClient := smtp.NewClient(conn)
	auth := sasl.NewPlainClient("", address, appPassword)
	if err := smtpClient.Auth(auth); err != nil {
		smtpClient.Close()
		return nil, fmt.Errorf("gmail: SMTP auth failed: %w", err)
	}

	return smtpClient, nil
}

type gmailSearch struct {
	Atom  string
	Value string
}

func (s *gmailSearch) Command() *imap.Command {
	return &imap.Command{
		Name:      "UID SEARCH",
		Arguments: []any{imap.RawString(s.Atom + " " + s.Value)},
	}
}

type gmailStoreLabels struct {
	SeqSet *imap.SeqSet
	Add    bool
	Labels []string
}

func (s *gmailStoreLabels) Command() *imap.Command {
	op := "-X-GM-LABELS"
	if s.Add {
		op = "+X-GM-LABELS"
	}

	quoted := make([]string, 0, len(s.Labels))
	for _, label := range s.Labels {
		if strings.Contains(label, " ") {
			quoted = append(quoted, fmt.Sprintf("\"%s\"", label))
			continue
		}
		quoted = append(quoted, label)
	}

	return &imap.Command{
		Name:      "UID STORE",
		Arguments: []any{s.SeqSet, imap.RawString(op), imap.RawString("(" + strings.Join(quoted, " ") + ")")},
	}
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
