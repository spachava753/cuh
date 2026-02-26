package gmail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sort"
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
	fetchGmailThreadID imap.FetchItem = "X-GM-THRID"
	fetchGmailLabels   imap.FetchItem = "X-GM-LABELS"
)

// Thread represents a deduplicated Gmail conversation.
type Thread struct {
	ID           string
	Subject      string
	From         string
	MessageCount int
	Labels       []string
	LastMessage  time.Time
}

// ThreadMessage represents one message from a Gmail thread.
type ThreadMessage struct {
	UID     uint32
	Subject string
	From    string
	Date    time.Time
	Labels  []string
}

// SendEmail sends an email through a configured Gmail account.
func SendEmail(to string, subject string, body string) error {
	if strings.TrimSpace(to) == "" || strings.TrimSpace(subject) == "" || strings.TrimSpace(body) == "" {
		return errors.New("gmail: to, subject, and body are required")
	}

	from, appPassword, err := loadCredentials()
	if err != nil {
		return err
	}

	smtpClient, err := connectSMTP(from, appPassword)
	if err != nil {
		return err
	}
	defer smtpClient.Close()

	if err := smtpClient.Mail(from, nil); err != nil {
		return fmt.Errorf("gmail: MAIL FROM failed: %w", err)
	}
	if err := smtpClient.Rcpt(to, nil); err != nil {
		return fmt.Errorf("gmail: RCPT TO failed: %w", err)
	}

	writer, err := smtpClient.Data()
	if err != nil {
		return fmt.Errorf("gmail: DATA failed: %w", err)
	}

	if _, err := writer.Write([]byte(buildPlainTextMessage(from, to, subject, body))); err != nil {
		return fmt.Errorf("gmail: writing message failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("gmail: finalizing message failed: %w", err)
	}

	if err := smtpClient.Quit(); err != nil {
		return fmt.Errorf("gmail: QUIT failed: %w", err)
	}

	return nil
}

// ListThreads lists recent Gmail threads for the selected mailbox.
// If folder is empty, [Gmail]/All Mail is used.
func ListThreads(folder string, count int) ([]Thread, error) {
	if count <= 0 {
		return nil, errors.New("gmail: count must be greater than zero")
	}
	if strings.TrimSpace(folder) == "" {
		folder = gmailAllMail
	}

	address, appPassword, err := loadCredentials()
	if err != nil {
		return nil, err
	}

	imapClient, err := connectIMAP(address, appPassword)
	if err != nil {
		return nil, err
	}
	defer imapClient.Logout()

	mailbox, err := imapClient.Select(folder, true)
	if err != nil {
		return nil, fmt.Errorf("gmail: selecting mailbox %q failed: %w", folder, err)
	}
	if mailbox.Messages == 0 {
		return []Thread{}, nil
	}

	from := uint32(1)
	if mailbox.Messages > uint32(count) {
		from = mailbox.Messages - uint32(count) + 1
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(from, mailbox.Messages)

	items := []imap.FetchItem{imap.FetchEnvelope, fetchGmailThreadID, fetchGmailLabels}
	messages := make(chan *imap.Message, count+16)
	done := make(chan error, 1)
	go func() {
		done <- imapClient.Fetch(seqSet, items, messages)
	}()

	threads := map[string]*Thread{}
	for msg := range messages {
		threadID := parseThreadID(msg.Items[fetchGmailThreadID])
		if threadID == "" {
			continue
		}

		entry, ok := threads[threadID]
		if !ok {
			entry = &Thread{
				ID:      threadID,
				Subject: "(no subject)",
			}
			threads[threadID] = entry
		}

		entry.MessageCount++
		if msg.Envelope != nil {
			if msg.Envelope.Subject != "" {
				entry.Subject = msg.Envelope.Subject
			}
			if len(msg.Envelope.From) > 0 {
				entry.From = msg.Envelope.From[0].Address()
			}
			if msg.Envelope.Date.After(entry.LastMessage) {
				entry.LastMessage = msg.Envelope.Date
			}
		}

		entry.Labels = mergeLabels(entry.Labels, parseLabels(msg.Items[fetchGmailLabels]))
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("gmail: fetching threads failed: %w", err)
	}

	result := make([]Thread, 0, len(threads))
	for _, thread := range threads {
		result = append(result, *thread)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].LastMessage.Equal(result[j].LastMessage) {
			return result[i].ID > result[j].ID
		}
		return result[i].LastMessage.After(result[j].LastMessage)
	})

	return result, nil
}

// GetThread returns all messages for a specific Gmail thread.
func GetThread(threadID string) ([]ThreadMessage, error) {
	if strings.TrimSpace(threadID) == "" {
		return nil, errors.New("gmail: threadID is required")
	}

	address, appPassword, err := loadCredentials()
	if err != nil {
		return nil, err
	}

	imapClient, err := connectIMAP(address, appPassword)
	if err != nil {
		return nil, err
	}
	defer imapClient.Logout()

	if _, err := imapClient.Select(gmailAllMail, true); err != nil {
		return nil, fmt.Errorf("gmail: selecting mailbox %q failed: %w", gmailAllMail, err)
	}

	uids, err := searchThreadUIDs(imapClient, threadID)
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, fmt.Errorf("gmail: thread %s not found", threadID)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uids...)

	messages := make(chan *imap.Message, len(uids)+8)
	done := make(chan error, 1)
	go func() {
		done <- imapClient.UidFetch(seqSet, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, fetchGmailLabels}, messages)
	}()

	result := make([]ThreadMessage, 0, len(uids))
	for msg := range messages {
		entry := ThreadMessage{
			UID:    msg.Uid,
			Labels: parseLabels(msg.Items[fetchGmailLabels]),
		}
		if msg.Envelope != nil {
			entry.Subject = msg.Envelope.Subject
			entry.Date = msg.Envelope.Date
			if len(msg.Envelope.From) > 0 {
				entry.From = msg.Envelope.From[0].Address()
			}
		}
		result = append(result, entry)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("gmail: fetching thread failed: %w", err)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Date.Equal(result[j].Date) {
			return result[i].UID < result[j].UID
		}
		return result[i].Date.Before(result[j].Date)
	})

	return result, nil
}

// DeleteThread moves an entire Gmail thread to Trash.
func DeleteThread(threadID string) error {
	return deleteOrLabelThread(threadID, nil, false, true)
}

// AddLabelToThread applies a label to all messages in the thread.
func AddLabelToThread(threadID string, label string) error {
	if strings.TrimSpace(label) == "" {
		return errors.New("gmail: label is required")
	}
	return deleteOrLabelThread(threadID, []string{label}, true, false)
}

// RemoveLabelFromThread removes a label from all messages in the thread.
func RemoveLabelFromThread(threadID string, label string) error {
	if strings.TrimSpace(label) == "" {
		return errors.New("gmail: label is required")
	}
	return deleteOrLabelThread(threadID, []string{label}, false, false)
}

func deleteOrLabelThread(threadID string, labels []string, addLabel bool, deleteThread bool) error {
	if strings.TrimSpace(threadID) == "" {
		return errors.New("gmail: threadID is required")
	}

	address, appPassword, err := loadCredentials()
	if err != nil {
		return err
	}

	imapClient, err := connectIMAP(address, appPassword)
	if err != nil {
		return err
	}
	defer imapClient.Logout()

	if _, err := imapClient.Select(gmailAllMail, false); err != nil {
		return fmt.Errorf("gmail: selecting mailbox %q failed: %w", gmailAllMail, err)
	}

	uids, err := searchThreadUIDs(imapClient, threadID)
	if err != nil {
		return err
	}
	if len(uids) == 0 {
		return fmt.Errorf("gmail: thread %s not found", threadID)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uids...)

	if deleteThread {
		if err := imapClient.UidCopy(seqSet, gmailTrash); err != nil {
			return fmt.Errorf("gmail: copying thread to trash failed: %w", err)
		}

		storeItem := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := imapClient.UidStore(seqSet, storeItem, []any{imap.DeletedFlag}, nil); err != nil {
			return fmt.Errorf("gmail: marking thread deleted failed: %w", err)
		}

		if err := imapClient.Expunge(nil); err != nil {
			return fmt.Errorf("gmail: expunge failed: %w", err)
		}

		return nil
	}

	storeCmd := &gmailStoreLabels{
		SeqSet: seqSet,
		Add:    addLabel,
		Labels: labels,
	}
	if _, err := imapClient.Execute(storeCmd, nil); err != nil {
		return fmt.Errorf("gmail: updating thread labels failed: %w", err)
	}

	return nil
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

func buildPlainTextMessage(from string, to string, subject string, body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")

	headers := []string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		fmt.Sprintf("Date: %s", time.Now().Format(time.RFC1123Z)),
		fmt.Sprintf("Message-ID: <%d.%s>", time.Now().UnixNano(), from),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}

	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body
}

func parseThreadID(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case uint64:
		return fmt.Sprintf("%d", value)
	case int64:
		return fmt.Sprintf("%d", value)
	case int:
		return fmt.Sprintf("%d", value)
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
		return value
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

func mergeLabels(existing []string, incoming []string) []string {
	if len(incoming) == 0 {
		return existing
	}

	set := make(map[string]struct{}, len(existing)+len(incoming))
	for _, label := range existing {
		set[label] = struct{}{}
	}
	for _, label := range incoming {
		set[label] = struct{}{}
	}

	merged := make([]string, 0, len(set))
	for label := range set {
		merged = append(merged, label)
	}
	sort.Strings(merged)
	return merged
}

func searchThreadUIDs(imapClient *client.Client, threadID string) ([]uint32, error) {
	searchCmd := &gmailThreadSearch{ThreadID: threadID}
	searchResp := &responses.Search{}
	if _, err := imapClient.Execute(searchCmd, searchResp); err != nil {
		return nil, fmt.Errorf("gmail: searching thread failed: %w", err)
	}
	return searchResp.Ids, nil
}

type gmailThreadSearch struct {
	ThreadID string
}

func (s *gmailThreadSearch) Command() *imap.Command {
	return &imap.Command{
		Name:      "UID SEARCH",
		Arguments: []any{imap.RawString("X-GM-THRID " + s.ThreadID)},
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
