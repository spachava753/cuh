// Package contacts provides agent-oriented primitives for managing macOS
// Contacts (Contacts.framework) via cgo.
//
// The API is intentionally primitive-first: callers compose recipes from a
// small set of explicit read and write operations rather than relying on a
// large catalog of one-off workflows.
//
// Primitive groups:
//
//   - Contacts: [CreateContact], [GetContact], [ListContacts], [UpdateContact],
//     [DeleteContact].
//   - Groups: [CreateGroup], [GetGroup], [ListGroups], [ListSubgroups],
//     [UpdateGroup], [DeleteGroup].
//   - Membership: [AddContactToGroup], [RemoveContactFromGroup],
//     [ListContactsInGroup].
//   - Containers: [ListContainers], [GetContainer], [DefaultContainerID].
//   - Authorization: [CheckAuthorization], [RequestAuthorization].
//
// Groups and subgroups are represented by the same [Group] type. A subgroup is
// just a group with ParentGroupID set.
//
// Suggested import path from calling code:
//
//	import "github.com/spachava753/cuh/macos/contacts"
//
// # Safety Model
//
// Most mutating operations delegate directly to Contacts.framework via
// CNSaveRequest and then perform read-after-write verification. Errors are
// returned as typed sentinel causes (for example [ErrNotFound],
// [ErrInvalidArgument], [ErrPermissionDenied], [ErrVerificationFailed]) wrapped
// in [OpError] for operation context.
//
// [RemoveContactFromGroup] uses osascript (AppleScript) as a platform
// workaround because CNSaveRequest removeMember:fromGroup: can silently fail on
// macOS 14.6+/15.x.
//
// # Composition Pattern
//
// Typical flow:
//
//  1. Check authorization with [CheckAuthorization] and call
//     [RequestAuthorization] if needed.
//  2. Choose scope with [DefaultContainerID] or [ListContainers].
//  3. Find/select entities using [ListContacts], [GetContact], [ListGroups].
//  4. Apply explicit mutations with Create/Update/Delete operations.
//  5. Verify and synchronize membership with [ListContactsInGroup],
//     [AddContactToGroup], [RemoveContactFromGroup].
//
// # Cookbook Recipes
//
// The snippets below are recipe-level examples built only from exported
// primitives. They are meant for composition patterns that agents and scripts
// commonly need.
//
// 1) List contacts that match one field, then apply additional in-memory
// filtering:
//
//	func contactsWithWorkEmailAndNoPhone(ctx context.Context) ([]contacts.Contact, error) {
//		in := contacts.ListContactsInput{
//			Filters: []contacts.Filter{
//				{
//					Field: contacts.ContactFieldEmailAddresses,
//					Op:    contacts.FilterContains,
//					Value: "@example.com",
//				},
//			},
//		}
//
//		out := make([]contacts.Contact, 0)
//		for c, err := range contacts.ListContacts(ctx, in) {
//			if err != nil {
//				return nil, err
//			}
//			// Second-stage filter done in caller logic.
//			if len(c.PhoneNumbers) == 0 {
//				out = append(out, c)
//			}
//		}
//		return out, nil
//	}
//
// 2) Page through contacts using Offset and a caller-defined limit:
//
//	func contactsPageByFamilyName(ctx context.Context, familyName string, offset, limit int) ([]contacts.Contact, error) {
//		if limit <= 0 {
//			return []contacts.Contact{}, nil
//		}
//
//		in := contacts.ListContactsInput{
//			Filters: []contacts.Filter{
//				{Field: contacts.ContactFieldFamilyName, Op: contacts.FilterEquals, Value: familyName},
//			},
//			Offset: offset,
//		}
//
//		out := make([]contacts.Contact, 0, limit)
//		for c, err := range contacts.ListContacts(ctx, in) {
//			if err != nil {
//				return nil, err
//			}
//			out = append(out, c)
//			if len(out) == limit {
//				break
//			}
//		}
//		return out, nil
//	}
//
// 3) Create multiple contacts in a batch with per-item success/failure:
//
//	type BatchCreateResult struct {
//		Input   contacts.CreateContactInput
//		Created contacts.Contact
//		Err     error
//	}
//
//	func batchCreateContacts(ctx context.Context, inputs []contacts.CreateContactInput) ([]BatchCreateResult, error) {
//		defaultContainerID, err := contacts.DefaultContainerID(ctx)
//		if err != nil {
//			return nil, err
//		}
//
//		results := make([]BatchCreateResult, 0, len(inputs))
//		for _, in := range inputs {
//			if in.ContainerID == "" {
//				in.ContainerID = defaultContainerID
//			}
//
//			created, err := contacts.CreateContact(ctx, in)
//			results = append(results, BatchCreateResult{
//				Input:   in,
//				Created: created,
//				Err:     err,
//			})
//		}
//		return results, nil
//	}
//
// 4) Create contacts with an auto-incrementing name suffix:
//
//	func createAutoIncrementContact(ctx context.Context, prefix string, familyName string) (contacts.Contact, error) {
//		next := 1
//		in := contacts.ListContactsInput{
//			Filters: []contacts.Filter{
//				{Field: contacts.ContactFieldGivenName, Op: contacts.FilterContains, Value: prefix},
//			},
//		}
//
//		for c, err := range contacts.ListContacts(ctx, in) {
//			if err != nil {
//				return contacts.Contact{}, err
//			}
//			if !strings.HasPrefix(c.GivenName, prefix) {
//				continue
//			}
//
//			suffix := strings.TrimSpace(strings.TrimPrefix(c.GivenName, prefix))
//			n, err := strconv.Atoi(suffix)
//			if err == nil && n >= next {
//				next = n + 1
//			}
//		}
//
//		return contacts.CreateContact(ctx, contacts.CreateContactInput{
//			Contact: contacts.Contact{
//				GivenName:  fmt.Sprintf("%s%d", prefix, next),
//				FamilyName: familyName,
//			},
//		})
//	}
//
// 5) Ensure a group exists, then sync matching contacts into membership:
//
//	func ensureGroupByName(ctx context.Context, containerID, name string) (contacts.Group, error) {
//		groups, err := contacts.ListGroups(ctx, contacts.ListGroupsInput{
//			ContainerID:      containerID,
//			IncludeHierarchy: true,
//		})
//		if err != nil {
//			return contacts.Group{}, err
//		}
//
//		for _, g := range groups {
//			if g.Name == name {
//				return g, nil
//			}
//		}
//
//		return contacts.CreateGroup(ctx, contacts.CreateGroupInput{
//			Name:        name,
//			ContainerID: containerID,
//		})
//	}
//
//	func syncEngineersToGroup(ctx context.Context, containerID string) error {
//		group, err := ensureGroupByName(ctx, containerID, "Engineering")
//		if err != nil {
//			return err
//		}
//
//		existing, err := contacts.ListContactsInGroup(ctx, group.Identifier)
//		if err != nil {
//			return err
//		}
//		inGroup := make(map[string]struct{}, len(existing))
//		for _, c := range existing {
//			inGroup[c.Identifier] = struct{}{}
//		}
//
//		in := contacts.ListContactsInput{
//			Filters: []contacts.Filter{
//				{Field: contacts.ContactFieldJobTitle, Op: contacts.FilterContains, Value: "Engineer"},
//			},
//		}
//
//		for c, err := range contacts.ListContacts(ctx, in) {
//			if err != nil {
//				return err
//			}
//			if _, ok := inGroup[c.Identifier]; ok {
//				continue
//			}
//			if err := contacts.AddContactToGroup(ctx, c.Identifier, group.Identifier); err != nil {
//				return err
//			}
//		}
//		return nil
//	}
//
// 6) Create a parent-child group hierarchy in a specific container:
//
//	func createTeamHierarchy(ctx context.Context, containerID string) (contacts.Group, contacts.Group, error) {
//		parent, err := contacts.CreateGroup(ctx, contacts.CreateGroupInput{
//			Name:        "Teams",
//			ContainerID: containerID,
//		})
//		if err != nil {
//			return contacts.Group{}, contacts.Group{}, err
//		}
//
//		child, err := contacts.CreateGroup(ctx, contacts.CreateGroupInput{
//			Name:          "Backend",
//			ContainerID:   containerID,
//			ParentGroupID: parent.Identifier,
//		})
//		if err != nil {
//			return contacts.Group{}, contacts.Group{}, err
//		}
//		return parent, child, nil
//	}
//
// # Error Handling Pattern
//
// Use [errors.Is] for coarse-grained typed handling and [errors.As] for
// operation context:
//
//	if err != nil {
//		if errors.Is(err, contacts.ErrNotFound) {
//			// Item does not exist.
//		}
//
//		var opErr *contacts.OpError
//		if errors.As(err, &opErr) {
//			fmt.Printf("operation=%s id=%s cause=%v\n", opErr.Op, opErr.ID, opErr.Err)
//		}
//	}
//
// # Context
//
// All functions accept context.Context. Long-running enumerations check for
// context cancellation between iterations where possible. Individual cgo calls
// into Contacts.framework are not interruptible.
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
// [CreateContactInput] is still settable (writes do not require the
// entitlement), but fetched contacts will have an empty Note unless the calling
// app has the notes entitlement. For this reason, filter fields intentionally
// do not expose a Note constant.
//
// # Testing
//
// Live tests create and clean up their own data, and do not mutate unrelated
// user contacts. Tests require Contacts access to be granted to the terminal or
// IDE process running `go test`.
package contacts
