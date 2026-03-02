// Package contacts provides agent-oriented primitives for managing macOS
// Contacts (Contacts.framework) via cgo.
//
// The package exposes five orthogonal primitive groups:
//
//   - Contacts: create, get, list (with typed filtering), update, delete.
//   - Groups: create (with optional parent), get, list, update, delete.
//   - Membership: add/remove a contact to/from a group.
//   - Containers: list and get container metadata.
//   - Authorization: check and request Contacts access.
//
// Groups model both groups and subgroups uniformly. A group may have a parent
// group (making it a subgroup) and/or child groups. The API does not
// distinguish between "groups" and "subgroups" — they are the same entity with
// optional parent/child relationships.
//
// # Safety Model
//
// Most mutating operations delegate directly to Contacts.framework via
// CNSaveRequest and then perform read-after-write verification. The package
// returns typed sentinel errors (e.g. [ErrNotFound], [ErrInvalidArgument])
// wrapped in [OpError] for operation context.
//
// [RemoveContactFromGroup] uses osascript (AppleScript) instead, because the
// CNSaveRequest removeMember:fromGroup: method has a known bug on
// macOS 14.6+/15.x where removal silently fails.
//
// # Composition Pattern
//
//  1. Use ListContacts with filters + pagination to find contacts.
//  2. Use GetContact to hydrate a single contact by identifier.
//  3. Use CreateContact / UpdateContact / DeleteContact for mutations.
//  4. Use ListGroups/ListSubgroups + UpdateGroup for hierarchy workflows.
//  5. Use AddContactToGroup / RemoveContactFromGroup for membership.
//
// # Context
//
// All functions accept context.Context. Long-running enumerations check for
// context cancellation between iterations where possible. Note that individual
// cgo calls into Contacts.framework are not interruptible.
//
// # Build Constraints
//
// This package only builds on macOS (darwin) due to Contacts.framework
// dependency. All .go files use //go:build darwin.
//
// # Notes Field
//
// The CNContactNoteKey requires the com.apple.developer.contacts.notes
// entitlement on macOS 13+. This package intentionally omits the Note field
// from default fetch requests to avoid error 134092. The Note field on
// [CreateContactInput] is still settable (writes do not require the entitlement),
// but fetched contacts will have an empty Note unless the calling app has
// the notes entitlement. For this reason, filter fields intentionally do not
// expose a Note constant.
//
// # Testing
//
// Tests create their own test data and clean up afterward, never touching the
// user's existing contacts. Live tests run by default but require Contacts
// access to be granted to the terminal/IDE.
package contacts
