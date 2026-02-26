package messages

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nalgeon/be"
)

const (
	messagesLiveTestFlagEnv     = "MESSAGES_LIVE_TEST"
	messagesLiveContactQueryEnv = "MESSAGES_TEST_CONTACT_QUERY"
	livePollInterval            = 3 * time.Second
	liveWaitWindow              = 60 * time.Second
)

func TestLiveMessagingHelpers(t *testing.T) {
	if os.Getenv(messagesLiveTestFlagEnv) != "1" {
		t.Skipf("set %s=1 to run live Messages integration tests", messagesLiveTestFlagEnv)
	}

	contacts, err := ListContacts(100)
	be.Err(t, err, nil)
	be.True(t, len(contacts) > 0)

	query := strings.TrimSpace(os.Getenv(messagesLiveContactQueryEnv))
	if query == "" {
		query = "Karen Ram"
	}

	contact, err := resolveLiveTestContact(query)
	be.Err(t, err, nil)

	_, err = ListMessages(MessageQuery{Limit: 20})
	be.Err(t, err, nil)

	_, err = ListMessages(MessageQuery{
		Contact:   contact.ChatID,
		ReadState: MessageReadStateUnread,
		Limit:     30,
	})
	be.Err(t, err, nil)

	_, err = ListUnreadConversations(20)
	be.Err(t, err, nil)

	fromMe := true
	beforeSendGUID := ""
	before, err := ListMessages(MessageQuery{Contact: contact.ChatID, FromMe: &fromMe, Limit: 5})
	be.Err(t, err, nil)
	if len(before) > 0 {
		beforeSendGUID = before[0].GUID
	}

	be.Err(t, SendMessageToContact(contact.ChatID, "just saying"), nil)

	sentGUID, err := waitForNewOutboundMessageGUID(contact, beforeSendGUID, liveWaitWindow)
	be.Err(t, err, nil)
	be.True(t, sentGUID != "")
}

func resolveLiveTestContact(query string) (Contact, error) {
	if contact, err := ResolveContact(query); err == nil {
		return contact, nil
	}

	contacts, err := ListContacts(300)
	if err != nil {
		return Contact{}, err
	}

	for _, contact := range contacts {
		name := strings.ToLower(contact.Name)
		if strings.Contains(name, "ram") && (strings.Contains(name, "karen") || strings.Contains(name, "kiran")) {
			return contact, nil
		}
	}

	for _, contact := range contacts {
		if strings.Contains(strings.ToLower(contact.Name), "ram") {
			return Contact{}, fmt.Errorf("messages: live contact query %q is ambiguous; set %s to an exact ID", query, messagesLiveContactQueryEnv)
		}
	}

	return Contact{}, fmt.Errorf("messages: live contact query %q not found", query)
}

func waitForNewOutboundMessageGUID(contact Contact, previousGUID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		fromMe := true
		messages, err := ListMessages(MessageQuery{
			Contact: contact.ChatID,
			FromMe:  &fromMe,
			Limit:   40,
		})
		if err != nil {
			return "", err
		}

		for _, message := range messages {
			if message.GUID == "" {
				continue
			}
			if previousGUID == "" || message.GUID != previousGUID {
				return message.GUID, nil
			}
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("new outbound message not found in %s", timeout)
		}
		time.Sleep(livePollInterval)
	}
}
