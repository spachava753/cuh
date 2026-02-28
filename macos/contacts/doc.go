// Package contacts provides primitive-first contact management for macOS agents.
//
// The package intentionally exposes only five core primitives:
//
//   - Find: select contact references with typed query filters and pagination.
//   - Get: hydrate references into typed contact items.
//   - Upsert: create contacts and patch existing contacts.
//   - Mutate: apply explicit state transitions to existing contacts.
//   - Groups: discover and manage the group catalog.
//
// The intended composition model is:
//
//	find/select -> get/hydrate -> decide -> mutate/upsert
//
// # Safety Model
//
// All side effects are explicit through Upsert, Mutate, and Groups actions.
// Find and Get are read-only. Mutating primitives return per-item structured
// results so callers can reason about partial success.
//
// This package does not provide dry-run mode in v1. Callers should perform
// explicit read/decide phases before writing.
//
// # Platform
//
// The live backend is implemented only on Darwin using direct cgo bindings to
// Contacts.framework (CNContactStore, CNContactFetchRequest, CNSaveRequest).
// Non-Darwin builds return ErrUnsupportedPlatform.
//
// # Authorization
//
// Use AuthorizationStatus to inspect current permission state and RequestAccess
// to request Contacts access when needed.
//
// # Known Limitation
//
// On some macOS versions/account backends, Contacts.framework may report success
// for group membership removal while membership remains unchanged.
//
// Workaround rationale: removing members from the Contacts app UI (and the same
// operation through the Contacts AppleScript dictionary) can persist for records
// where the direct Contacts.framework removeMember path does not. To avoid false
// success, this package routes group-member removal through an AppleScript-backed
// path and still verifies membership post-write. If state does not persist,
// Mutate/Upsert return typed errors rather than reporting success.
//
// In observed live runs, creating contacts with an initial note value can fail
// deterministically with a store error (for example, Cocoa error 134092), while
// creates without notes succeed. For those environments, create first without
// note and then attempt a follow-up note mutation with typed error handling.
//
// # Composition Examples
//
// 1) Update one matched contact note:
//
//	findOut, err := contacts.Find(contacts.FindInput{
//		Query: contacts.Query{NameContains: "Priya", OrganizationContains: "Acme"},
//		Page:  contacts.Page{Limit: 20},
//	})
//	if err != nil || len(findOut.Refs) == 0 {
//		// handle
//	}
//
//	_, err = contacts.Mutate(contacts.MutateInput{
//		Refs: []contacts.Ref{findOut.Refs[0]},
//		Ops: []contacts.MutationOp{{
//			Type:  contacts.MutationSetNote,
//			Value: "Met at Acme SF office",
//		}},
//	})
//
// 2) Create a group and add matching contacts:
//
//	groupsOut, err := contacts.Groups(contacts.GroupsInput{
//		Action: contacts.GroupsActionCreate,
//		Name:   "Vendors 2026",
//	})
//	if err != nil {
//		// handle
//	}
//
//	var groupID string
//	for _, result := range groupsOut.Results {
//		if result.Succeeded && result.Group.ID != "" {
//			groupID = result.Group.ID
//		}
//	}
//
//	findOut, err = contacts.Find(contacts.FindInput{
//		Query: contacts.Query{EmailDomain: "vendorco.com"},
//		Page:  contacts.Page{Limit: 200},
//	})
//	if err != nil {
//		// handle
//	}
//
//	_, err = contacts.Mutate(contacts.MutateInput{
//		Refs: findOut.Refs,
//		Ops:  []contacts.MutationOp{{Type: contacts.MutationAddToGroup, Value: groupID}},
//	})
//
// 3) Upsert lifecycle (create then patch):
//
//	upsertOut, err := contacts.Upsert(contacts.UpsertInput{
//		Create: []contacts.ContactDraft{{
//			GivenName:    "Dr.",
//			FamilyName:   "Lee",
//			Organization: "Lee Clinic",
//			Emails:       []contacts.LabeledValue{{Label: "work", Value: "dr.lee@clinic.example"}},
//		}},
//	})
//	if err != nil {
//		// handle
//	}
//
//	created := upsertOut.Results[0].Ref
//	_, err = contacts.Upsert(contacts.UpsertInput{
//		Patch: []contacts.ContactPatch{{
//			Ref: created,
//			Changes: contacts.ContactChanges{
//				Note: ptr("Emergency contact"),
//			},
//		}},
//	})
//
//nolint:revive // package comment documents API composition examples.
package contacts
