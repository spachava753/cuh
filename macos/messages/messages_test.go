package messages

import (
	"testing"

	"github.com/nalgeon/be"
)

func TestParseChatIdentifier(t *testing.T) {
	be.Equal(t, parseChatIdentifier("any;-;+15551234567"), "+15551234567")
	be.Equal(t, parseChatIdentifier("any;+;chat123"), "chat123")
	be.Equal(t, parseChatIdentifier(""), "")
}

func TestNormalizeHandleForSend(t *testing.T) {
	be.Equal(t, normalizeHandleForSend("+12105551212(smsft)"), "+12105551212")
	be.Equal(t, normalizeHandleForSend(" test@example.com "), "test@example.com")
}

func TestResolveContactFromList(t *testing.T) {
	contacts := []Contact{
		{Name: "Karen Ram", ContactID: "+17328017003", ChatID: "any;-;+17328017003", ChatIdentifier: "+17328017003", Handle: "+17328017003"},
		{Name: "Ram Joolukuntla", ContactID: "+12103792244", ChatID: "any;-;+12103792244", ChatIdentifier: "+12103792244", Handle: "+12103792244"},
	}

	resolved, err := resolveContactFromList(contacts, "Karen Ram")
	be.Err(t, err, nil)
	be.Equal(t, resolved.ContactID, "+17328017003")

	resolved, err = resolveContactFromList(contacts, "+1 (210) 379-2244")
	be.Err(t, err, nil)
	be.Equal(t, resolved.Name, "Ram Joolukuntla")

	_, err = resolveContactFromList(contacts, "ram")
	be.True(t, err != nil)
}
