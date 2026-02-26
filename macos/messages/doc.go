// Package messages provides macOS Messages helpers for contact discovery,
// read-oriented conversation workflows, and sending iMessage/SMS/RCS messages.
//
// The package is intended for computer-use automation scripts and CPE agents.
//
// Data sources
//
//   - AppleScript (Messages.app): send operations and contact name enrichment.
//   - SQLite (~/Library/Messages/chat.db): fast read/list queries.
//
// Exported API (recommended usage order)
//
//  1. ListUnreadConversations(limit)
//     Quick triage: which contacts currently have unread inbound messages.
//  2. ListContacts(limit)
//     Browse recent contacts/chats with metadata such as service, unread count,
//     message count, and last message timestamp.
//  3. ResolveContact(query)
//     Turn a user-facing query (name, chat id, handle) into a deterministic
//     contact target. Returns a clear ambiguity error when multiple matches
//     exist.
//  4. ListMessages(query)
//     Read messages with filters for Contact (name/chat id/handle), ReadState
//     (MessageReadStateAll, MessageReadStateRead, MessageReadStateUnread),
//     FromMe (inbound vs outbound), and Limit.
//  5. SendMessageToContact(contactQuery, body)
//     Send a message by name, chat id, or handle.
//
// Primary models
//
//   - Contact: resolved contact/chat metadata.
//   - MessageQuery: filters for ListMessages.
//   - Message: one message row with sender/read/timestamps and contact info.
//   - UnreadConversation: unread summary per contact/chat.
//   - MessageReadState: read-filter enum for ListMessages.
//
// Operational notes
//
//   - This package intentionally does not implement message deletion.
//   - Sending requires macOS Automation permission for the calling process to
//     control Messages.app (System Settings -> Privacy & Security -> Automation).
//   - SQLite access uses github.com/mattn/go-sqlite3 (CGO required).
package messages
