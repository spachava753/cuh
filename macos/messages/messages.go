package messages

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	messagesDBRelativePath = "Library/Messages/chat.db"
	appleReferenceUnix     = int64(978307200) // 2001-01-01T00:00:00Z
)

// MessageReadState controls read filtering for [ListMessages].
type MessageReadState string

const (
	// MessageReadStateAll returns both read and unread messages.
	MessageReadStateAll MessageReadState = "all"
	// MessageReadStateRead returns only messages marked as read.
	MessageReadStateRead MessageReadState = "read"
	// MessageReadStateUnread returns only unread inbound messages.
	MessageReadStateUnread MessageReadState = "unread"
)

// Contact is a Messages contact/conversation target.
type Contact struct {
	ChatID         string
	ChatIdentifier string
	ContactID      string
	Handle         string
	Name           string
	Service        string
	LastMessage    time.Time
	MessageCount   int
	UnreadCount    int
}

// MessageQuery controls list filters for [ListMessages].
type MessageQuery struct {
	Contact   string
	ReadState MessageReadState
	FromMe    *bool
	Limit     int
}

// Message is one message row from the local Messages database.
type Message struct {
	RowID          int64
	GUID           string
	Text           string
	IsFromMe       bool
	IsRead         bool
	SentAt         time.Time
	ReadAt         *time.Time
	ContactID      string
	ContactName    string
	ChatIdentifier string
	ChatID         string
	Service        string
}

// UnreadConversation summarizes unread inbound messages for one contact/chat.
type UnreadConversation struct {
	ChatIdentifier string
	ChatID         string
	ContactID      string
	ContactName    string
	Service        string
	UnreadCount    int
	LastMessage    time.Time
}

// SendMessageToContact sends an iMessage/SMS to a contact resolved by name,
// chat id, or handle.
func SendMessageToContact(contactQuery string, body string) error {
	contactQuery = strings.TrimSpace(contactQuery)
	body = strings.TrimSpace(body)
	if contactQuery == "" || body == "" {
		return errors.New("messages: contact query and body are required")
	}

	contact, err := ResolveContact(contactQuery)
	if err == nil {
		if contact.ChatID != "" {
			return sendMessageToChatID(contact.ChatID, body)
		}
		if contact.Handle != "" {
			return sendMessageToHandle(contact.Handle, contact.Service, body)
		}
		if contact.ChatIdentifier != "" {
			return sendMessageToHandle(contact.ChatIdentifier, contact.Service, body)
		}
	}

	if looksLikeHandle(contactQuery) {
		return sendMessageToHandle(contactQuery, "", body)
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("messages: could not find a valid send target for %q", contactQuery)
}

// ResolveContact resolves a contact by exact or partial name/id/chat-id match.
func ResolveContact(query string) (Contact, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Contact{}, errors.New("messages: query is required")
	}

	contacts, err := ListContacts(1000)
	if err != nil {
		return Contact{}, err
	}
	return resolveContactFromList(contacts, query)
}

// ListContacts lists recent contacts/chats, enriched with contact names when
// available from the Messages AppleScript API.
func ListContacts(limit int) ([]Contact, error) {
	if limit <= 0 {
		limit = 50
	}

	stats, err := listContactStats(max(limit*4, 200))
	if err != nil {
		return nil, err
	}

	participants, participantsErr := listChatParticipants()

	statsByIdentifier := make(map[string]contactStat, len(stats))
	for _, stat := range stats {
		if stat.ChatIdentifier != "" {
			statsByIdentifier[stat.ChatIdentifier] = stat
		}
	}

	contacts := make([]Contact, 0, len(stats))
	seenByIdentifier := map[string]struct{}{}
	seenByChatID := map[string]struct{}{}

	for _, participant := range participants {
		identifier := parseChatIdentifier(participant.ChatID)
		stat, hasStat := statsByIdentifier[identifier]

		contact := Contact{
			ChatID:         participant.ChatID,
			ChatIdentifier: identifier,
			Handle:         firstNonEmpty(participant.Handle),
			Name:           firstNonEmpty(participant.Name),
		}
		if hasStat {
			if contact.ChatIdentifier == "" {
				contact.ChatIdentifier = stat.ChatIdentifier
			}
			contact.Handle = firstNonEmpty(contact.Handle, stat.UncanonicalizedHandle, stat.Handle)
			contact.Name = firstNonEmpty(contact.Name, stat.DisplayName)
			contact.Service = stat.Service
			contact.LastMessage = appleNanoToTime(stat.LastMessageRaw)
			contact.MessageCount = stat.MessageCount
			contact.UnreadCount = stat.UnreadCount
		}

		contact.ContactID = firstNonEmpty(contact.Handle, contact.ChatIdentifier, contact.ChatID)
		contact.Name = firstNonEmpty(contact.Name, contact.ContactID)

		if contact.ChatID != "" {
			if _, exists := seenByChatID[contact.ChatID]; exists {
				continue
			}
			seenByChatID[contact.ChatID] = struct{}{}
		}
		if contact.ChatIdentifier != "" {
			seenByIdentifier[contact.ChatIdentifier] = struct{}{}
		}
		contacts = append(contacts, contact)
	}

	for _, stat := range stats {
		if stat.ChatIdentifier != "" {
			if _, exists := seenByIdentifier[stat.ChatIdentifier]; exists {
				continue
			}
		}

		contact := Contact{
			ChatIdentifier: stat.ChatIdentifier,
			Handle:         firstNonEmpty(stat.UncanonicalizedHandle, stat.Handle),
			Name:           firstNonEmpty(stat.DisplayName, stat.UncanonicalizedHandle, stat.Handle, stat.ChatIdentifier),
			Service:        stat.Service,
			LastMessage:    appleNanoToTime(stat.LastMessageRaw),
			MessageCount:   stat.MessageCount,
			UnreadCount:    stat.UnreadCount,
		}
		contact.ContactID = firstNonEmpty(contact.Handle, contact.ChatIdentifier)
		contacts = append(contacts, contact)
	}

	sort.Slice(contacts, func(i, j int) bool {
		if contacts[i].LastMessage.Equal(contacts[j].LastMessage) {
			return contacts[i].Name < contacts[j].Name
		}
		return contacts[i].LastMessage.After(contacts[j].LastMessage)
	})

	if len(contacts) > limit {
		contacts = contacts[:limit]
	}
	if len(contacts) == 0 && participantsErr != nil {
		return nil, participantsErr
	}
	return contacts, nil
}

// ListMessages returns messages filtered by contact and read state.
func ListMessages(query MessageQuery) ([]Message, error) {
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	if query.ReadState == "" {
		query.ReadState = MessageReadStateAll
	}
	if query.ReadState != MessageReadStateAll && query.ReadState != MessageReadStateRead && query.ReadState != MessageReadStateUnread {
		return nil, fmt.Errorf("messages: invalid read state %q", query.ReadState)
	}

	var (
		identifier string
		handle     string
		chatID     string
	)
	if strings.TrimSpace(query.Contact) != "" {
		if contact, err := ResolveContact(query.Contact); err == nil {
			identifier = contact.ChatIdentifier
			handle = contact.Handle
			chatID = contact.ChatID
		} else {
			identifier = parseChatIdentifier(query.Contact)
			handle = strings.TrimSpace(query.Contact)
		}
	}

	where := []string{"COALESCE(m.is_empty, 0) = 0"}
	if identifier != "" || handle != "" || chatID != "" {
		pieces := make([]string, 0, 5)
		if identifier != "" {
			q := sqlQuote(identifier)
			pieces = append(pieces, "c.chat_identifier = "+q)
		}
		if chatID != "" {
			q := sqlQuote(chatID)
			pieces = append(pieces, "('any;-;' || c.chat_identifier) = "+q)
			pieces = append(pieces, "('any;+;' || c.chat_identifier) = "+q)
		}
		if handle != "" {
			q := sqlQuote(handle)
			pieces = append(pieces, "h.id = "+q)
			pieces = append(pieces, "h.uncanonicalized_id = "+q)
			pieces = append(pieces, "c.chat_identifier = "+q)
			pieces = append(pieces, "c.display_name = "+q)
		}
		where = append(where, "("+strings.Join(pieces, " OR ")+")")
	}

	if query.FromMe != nil {
		if *query.FromMe {
			where = append(where, "m.is_from_me = 1")
		} else {
			where = append(where, "m.is_from_me = 0")
		}
	}

	switch query.ReadState {
	case MessageReadStateRead:
		where = append(where, "m.is_read = 1")
	case MessageReadStateUnread:
		where = append(where, "m.is_from_me = 0", "m.is_read = 0")
	}

	sql := fmt.Sprintf(`
WITH chat_for_message AS (
	SELECT message_id, MIN(chat_id) AS chat_id
	FROM chat_message_join
	GROUP BY message_id
)
SELECT
	m.ROWID,
	COALESCE(m.guid, ''),
	COALESCE(m.text, ''),
	COALESCE(m.is_from_me, 0),
	COALESCE(m.is_read, 0),
	COALESCE(m.date, 0),
	COALESCE(m.date_read, 0),
	COALESCE(h.id, ''),
	COALESCE(h.uncanonicalized_id, ''),
	COALESCE(c.chat_identifier, ''),
	COALESCE(c.service_name, ''),
	COALESCE(c.display_name, '')
FROM message m
LEFT JOIN handle h ON h.ROWID = m.handle_id
LEFT JOIN chat_for_message cfm ON cfm.message_id = m.ROWID
LEFT JOIN chat c ON c.ROWID = cfm.chat_id
WHERE %s
ORDER BY m.date DESC
LIMIT %d;
`, strings.Join(where, " AND "), query.Limit)

	records, err := runSQLiteQuery(sql)
	if err != nil {
		return nil, err
	}

	nameByIdentifier, nameByHandle := contactNameLookup()

	messages := make([]Message, 0, len(records))
	for _, row := range records {
		if len(row) < 12 {
			continue
		}
		rowID, _ := strconv.ParseInt(row[0], 10, 64)
		sentAt := appleNanoToTime(row[5])

		var readAt *time.Time
		if rawRead := strings.TrimSpace(row[6]); rawRead != "" && rawRead != "0" {
			t := appleNanoToTime(rawRead)
			if !t.IsZero() {
				readAt = &t
			}
		}

		handleID := firstNonEmpty(row[8], row[7])
		identifier := row[9]
		chatID := ""
		if identifier != "" {
			chatID = "any;-;" + identifier
		}
		name := firstNonEmpty(nameByIdentifier[identifier], nameByHandle[handleID], row[11], handleID, identifier)

		messages = append(messages, Message{
			RowID:          rowID,
			GUID:           row[1],
			Text:           row[2],
			IsFromMe:       parseBoolInt(row[3]),
			IsRead:         parseBoolInt(row[4]),
			SentAt:         sentAt,
			ReadAt:         readAt,
			ContactID:      handleID,
			ContactName:    name,
			ChatIdentifier: identifier,
			ChatID:         chatID,
			Service:        row[10],
		})
	}

	return messages, nil
}

// ListUnreadConversations returns chats with at least one unread inbound message.
func ListUnreadConversations(limit int) ([]UnreadConversation, error) {
	if limit <= 0 {
		limit = 25
	}

	sql := fmt.Sprintf(`
WITH unread_stats AS (
	SELECT
		cmj.chat_id AS chat_id,
		MAX(m.date) AS last_date,
		SUM(CASE WHEN m.is_from_me = 0 AND m.is_read = 0 THEN 1 ELSE 0 END) AS unread_count
	FROM chat_message_join cmj
	JOIN message m ON m.ROWID = cmj.message_id
	WHERE COALESCE(m.is_empty, 0) = 0
	GROUP BY cmj.chat_id
), first_handle AS (
	SELECT chat_id, MIN(handle_id) AS handle_id
	FROM chat_handle_join
	GROUP BY chat_id
)
SELECT
	COALESCE(c.chat_identifier, ''),
	COALESCE(c.service_name, ''),
	COALESCE(c.display_name, ''),
	COALESCE(h.id, ''),
	COALESCE(h.uncanonicalized_id, ''),
	COALESCE(us.last_date, 0),
	COALESCE(us.unread_count, 0)
FROM unread_stats us
JOIN chat c ON c.ROWID = us.chat_id
LEFT JOIN first_handle fh ON fh.chat_id = c.ROWID
LEFT JOIN handle h ON h.ROWID = fh.handle_id
WHERE us.unread_count > 0
ORDER BY us.last_date DESC
LIMIT %d;
`, limit)

	records, err := runSQLiteQuery(sql)
	if err != nil {
		return nil, err
	}

	nameByIdentifier, nameByHandle := contactNameLookup()

	result := make([]UnreadConversation, 0, len(records))
	for _, row := range records {
		if len(row) < 7 {
			continue
		}
		unreadCount, _ := strconv.Atoi(row[6])
		handleID := firstNonEmpty(row[4], row[3])
		identifier := row[0]

		result = append(result, UnreadConversation{
			ChatIdentifier: identifier,
			ChatID:         buildChatID(identifier),
			ContactID:      firstNonEmpty(handleID, identifier),
			ContactName:    firstNonEmpty(nameByIdentifier[identifier], nameByHandle[handleID], row[2], handleID, identifier),
			Service:        row[1],
			UnreadCount:    unreadCount,
			LastMessage:    appleNanoToTime(row[5]),
		})
	}

	return result, nil
}

type contactStat struct {
	ChatIdentifier        string
	Service               string
	DisplayName           string
	Handle                string
	UncanonicalizedHandle string
	LastMessageRaw        string
	MessageCount          int
	UnreadCount           int
}

type chatParticipant struct {
	ChatID string
	Handle string
	Name   string
}

func listContactStats(limit int) ([]contactStat, error) {
	sql := fmt.Sprintf(`
WITH message_stats AS (
	SELECT
		cmj.chat_id AS chat_id,
		MAX(m.date) AS last_date,
		COUNT(m.ROWID) AS message_count,
		SUM(CASE WHEN m.is_from_me = 0 AND m.is_read = 0 THEN 1 ELSE 0 END) AS unread_count
	FROM chat_message_join cmj
	JOIN message m ON m.ROWID = cmj.message_id
	WHERE COALESCE(m.is_empty, 0) = 0
	GROUP BY cmj.chat_id
), first_handle AS (
	SELECT chat_id, MIN(handle_id) AS handle_id
	FROM chat_handle_join
	GROUP BY chat_id
)
SELECT
	COALESCE(c.chat_identifier, ''),
	COALESCE(c.service_name, ''),
	COALESCE(c.display_name, ''),
	COALESCE(h.id, ''),
	COALESCE(h.uncanonicalized_id, ''),
	COALESCE(ms.last_date, 0),
	COALESCE(ms.message_count, 0),
	COALESCE(ms.unread_count, 0)
FROM chat c
LEFT JOIN message_stats ms ON ms.chat_id = c.ROWID
LEFT JOIN first_handle fh ON fh.chat_id = c.ROWID
LEFT JOIN handle h ON h.ROWID = fh.handle_id
ORDER BY ms.last_date DESC
LIMIT %d;
`, limit)

	records, err := runSQLiteQuery(sql)
	if err != nil {
		return nil, err
	}

	stats := make([]contactStat, 0, len(records))
	for _, row := range records {
		if len(row) < 8 {
			continue
		}
		count, _ := strconv.Atoi(row[6])
		unread, _ := strconv.Atoi(row[7])
		stats = append(stats, contactStat{
			ChatIdentifier:        row[0],
			Service:               row[1],
			DisplayName:           row[2],
			Handle:                row[3],
			UncanonicalizedHandle: row[4],
			LastMessageRaw:        row[5],
			MessageCount:          count,
			UnreadCount:           unread,
		})
	}
	return stats, nil
}

func listChatParticipants() ([]chatParticipant, error) {
	script := []string{
		`set oldDelimiters to AppleScript's text item delimiters`,
		`set AppleScript's text item delimiters to "\n"`,
		`tell application "Messages"`,
		`set rows to {}`,
		`repeat with c in chats`,
		`set cid to id of c`,
		`set h to ""`,
		`set n to ""`,
		`try`,
		`set ps to participants of c`,
		`if (count of ps) > 0 then`,
		`set p to first item of ps`,
		`set h to handle of p`,
		`set n to full name of p`,
		`end if`,
		`end try`,
		`set end of rows to (cid & "|||" & h & "|||" & n)`,
		`end repeat`,
		`set outputText to rows as text`,
		`end tell`,
		`set AppleScript's text item delimiters to oldDelimiters`,
		`return outputText`,
	}
	out, err := runAppleScript(script, nil)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	participants := make([]chatParticipant, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|||")
		if len(parts) != 3 {
			continue
		}
		participants = append(participants, chatParticipant{
			ChatID: strings.TrimSpace(parts[0]),
			Handle: strings.TrimSpace(parts[1]),
			Name:   strings.TrimSpace(parts[2]),
		})
	}

	return participants, nil
}

func resolveContactFromList(contacts []Contact, query string) (Contact, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	queryAsID := normalizeID(query)

	exactMatches := make([]Contact, 0, 4)
	partialMatches := make([]Contact, 0, 8)

	for _, contact := range contacts {
		if contactMatchesExact(contact, normalizedQuery, queryAsID) {
			exactMatches = append(exactMatches, contact)
			continue
		}
		if contactMatchesPartial(contact, normalizedQuery, queryAsID) {
			partialMatches = append(partialMatches, contact)
		}
	}

	if len(exactMatches) == 1 {
		return exactMatches[0], nil
	}
	if len(exactMatches) > 1 {
		return Contact{}, ambiguousContactError(query, exactMatches)
	}
	if len(partialMatches) == 1 {
		return partialMatches[0], nil
	}
	if len(partialMatches) > 1 {
		return Contact{}, ambiguousContactError(query, partialMatches)
	}

	return Contact{}, fmt.Errorf("messages: contact %q not found", query)
}

func contactMatchesExact(contact Contact, normalizedQuery string, queryAsID string) bool {
	fields := []string{contact.ChatID, contact.ChatIdentifier, contact.ContactID, contact.Handle, contact.Name}
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), normalizedQuery) {
			return true
		}
		if queryAsID != "" && normalizeID(field) == queryAsID {
			return true
		}
	}
	return false
}

func contactMatchesPartial(contact Contact, normalizedQuery string, queryAsID string) bool {
	if normalizedQuery == "" {
		return false
	}
	fields := []string{contact.ChatID, contact.ChatIdentifier, contact.ContactID, contact.Handle, contact.Name}
	for _, field := range fields {
		v := strings.ToLower(strings.TrimSpace(field))
		if strings.Contains(v, normalizedQuery) {
			return true
		}
		if queryAsID != "" {
			normalizedField := normalizeID(field)
			if normalizedField != "" && strings.Contains(normalizedField, queryAsID) {
				return true
			}
		}
	}
	return false
}

func ambiguousContactError(query string, matches []Contact) error {
	labels := make([]string, 0, min(len(matches), 3))
	for i := range matches {
		if len(labels) == 3 {
			break
		}
		labels = append(labels, contactLabel(matches[i]))
	}
	return fmt.Errorf("messages: contact %q is ambiguous (%s)", query, strings.Join(labels, ", "))
}

func contactLabel(contact Contact) string {
	if contact.Name != "" && contact.ContactID != "" {
		return fmt.Sprintf("%s [%s]", contact.Name, contact.ContactID)
	}
	return firstNonEmpty(contact.Name, contact.ContactID, contact.ChatIdentifier, contact.ChatID)
}

func sendMessageToChatID(chatID string, body string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return errors.New("messages: chat id is required")
	}

	script := []string{
		`on run argv`,
		`set chatID to item 1 of argv`,
		`set bodyText to item 2 of argv`,
		`tell application "Messages"`,
		`send bodyText to chat id chatID`,
		`end tell`,
		`end run`,
	}
	_, err := runAppleScript(script, []string{chatID, body})
	if err != nil {
		return fmt.Errorf("messages: send to chat id %q failed: %w", chatID, err)
	}
	return nil
}

func sendMessageToHandle(handle string, service string, body string) error {
	handle = normalizeHandleForSend(handle)
	if handle == "" {
		return errors.New("messages: handle is required")
	}

	type sendAttempt struct {
		Service string
	}
	attempts := []sendAttempt{{Service: normalizeServiceName(service)}, {Service: "iMessage"}, {Service: "SMS"}, {Service: "RCS"}}
	seen := map[string]struct{}{}

	var lastErr error
	for _, attempt := range attempts {
		if attempt.Service == "" {
			continue
		}
		if _, ok := seen[attempt.Service]; ok {
			continue
		}
		seen[attempt.Service] = struct{}{}

		script := []string{
			`on run argv`,
			`set targetHandle to item 1 of argv`,
			`set bodyText to item 2 of argv`,
			`set desiredService to item 3 of argv`,
			`tell application "Messages"`,
			`set targetAccount to first account whose service type is desiredService`,
			`set targetParticipant to participant targetHandle of targetAccount`,
			`send bodyText to targetParticipant`,
			`end tell`,
			`end run`,
		}
		_, err := runAppleScript(script, []string{handle, body, attempt.Service})
		if err == nil {
			return nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = errors.New("no service account available")
	}
	return fmt.Errorf("messages: send to handle %q failed: %w", handle, lastErr)
}

func runAppleScript(lines []string, args []string) (string, error) {
	cmdArgs := make([]string, 0, len(lines)*2+len(args))
	for _, line := range lines {
		cmdArgs = append(cmdArgs, "-e", line)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("/usr/bin/osascript", cmdArgs...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(out.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func runSQLiteQuery(query string) ([][]string, error) {
	db, err := openMessagesDB(true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("messages: sqlite query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("messages: reading sqlite columns failed: %w", err)
	}

	records := make([][]string, 0, 64)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePointers := make([]any, len(columns))
		for i := range values {
			valuePointers[i] = &values[i]
		}
		if err := rows.Scan(valuePointers...); err != nil {
			return nil, fmt.Errorf("messages: scanning sqlite row failed: %w", err)
		}

		record := make([]string, len(columns))
		for i, value := range values {
			switch typed := value.(type) {
			case nil:
				record[i] = ""
			case []byte:
				record[i] = string(typed)
			default:
				record[i] = fmt.Sprint(typed)
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("messages: iterating sqlite rows failed: %w", err)
	}

	return records, nil
}

func openMessagesDB(readOnly bool) (*sql.DB, error) {
	dbPath, err := messagesDBPath()
	if err != nil {
		return nil, err
	}

	mode := "rw"
	if readOnly {
		mode = "ro"
	}

	dsn := fmt.Sprintf("file:%s?mode=%s&_busy_timeout=5000", strings.ReplaceAll(dbPath, " ", "%20"), mode)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("messages: opening sqlite database failed: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("messages: connecting to sqlite database failed: %w", err)
	}
	return db, nil
}

func messagesDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("messages: unable to resolve home directory: %w", err)
	}
	path := filepath.Join(home, messagesDBRelativePath)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("messages: chat database unavailable at %s: %w", path, err)
	}
	return path, nil
}

func contactNameLookup() (map[string]string, map[string]string) {
	participants, err := listChatParticipants()
	if err != nil {
		return map[string]string{}, map[string]string{}
	}
	byIdentifier := make(map[string]string, len(participants))
	byHandle := make(map[string]string, len(participants))
	for _, participant := range participants {
		name := strings.TrimSpace(participant.Name)
		if name == "" {
			continue
		}
		identifier := parseChatIdentifier(participant.ChatID)
		if identifier != "" {
			byIdentifier[identifier] = name
		}
		handle := strings.TrimSpace(participant.Handle)
		if handle != "" {
			byHandle[handle] = name
		}
	}
	return byIdentifier, byHandle
}

func appleNanoToTime(raw string) time.Time {
	nanos, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || nanos <= 0 {
		return time.Time{}
	}
	sec := nanos / int64(time.Second)
	nsec := nanos % int64(time.Second)
	return time.Unix(appleReferenceUnix+sec, nsec).UTC()
}

func parseChatIdentifier(chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ""
	}
	parts := strings.Split(chatID, ";")
	if len(parts) == 0 {
		return chatID
	}
	return parts[len(parts)-1]
}

func buildChatID(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ""
	}
	return "any;-;" + identifier
}

func parseBoolInt(raw string) bool {
	i, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return i != 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "+", "", ".", "")
	return replacer.Replace(value)
}

func normalizeHandleForSend(handle string) string {
	handle = strings.TrimSpace(handle)
	if i := strings.Index(handle, "("); i > 0 && strings.HasSuffix(handle, ")") {
		handle = strings.TrimSpace(handle[:i])
	}
	return handle
}

func looksLikeHandle(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains(value, "@") {
		return true
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || r == '+' {
			return true
		}
	}
	return false
}

func normalizeServiceName(service string) string {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "imessage":
		return "iMessage"
	case "sms":
		return "SMS"
	case "rcs":
		return "RCS"
	default:
		return ""
	}
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
