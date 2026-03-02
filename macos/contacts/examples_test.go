//go:build darwin

package contacts_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	contacts "github.com/spachava753/cuh/macos/contacts"
)

type batchCreateResult struct {
	Input   contacts.CreateContactInput
	Created contacts.Contact
	Err     error
}

func ExampleListContacts_filterAndPostFilter() {
	ctx := context.Background()

	in := contacts.ListContactsInput{
		Filters: []contacts.Filter{
			{Field: contacts.ContactFieldEmailAddresses, Op: contacts.FilterContains, Value: "@example.com"},
		},
	}

	filtered := make([]contacts.Contact, 0)
	for c, err := range contacts.ListContacts(ctx, in) {
		if err != nil {
			return
		}
		if len(c.PhoneNumbers) == 0 {
			filtered = append(filtered, c)
		}
	}
	_ = filtered
}

func ExampleListContacts_offsetPagination() {
	ctx := context.Background()
	const pageSize = 25
	offset := 0

	all := make([]contacts.Contact, 0, pageSize)
	for {
		in := contacts.ListContactsInput{
			Filters: []contacts.Filter{
				{Field: contacts.ContactFieldFamilyName, Op: contacts.FilterEquals, Value: "ExampleFamily"},
			},
			Offset: offset,
		}

		page := make([]contacts.Contact, 0, pageSize)
		for c, err := range contacts.ListContacts(ctx, in) {
			if err != nil {
				return
			}
			page = append(page, c)
			if len(page) == pageSize {
				break
			}
		}

		if len(page) == 0 {
			break
		}
		all = append(all, page...)
		offset += len(page)
		if len(page) < pageSize {
			break
		}
	}
	_ = all
}

func ExampleCreateContact_batch() {
	ctx := context.Background()

	defaultContainerID, err := contacts.DefaultContainerID(ctx)
	if err != nil {
		return
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	inputs := []contacts.CreateContactInput{
		{
			Contact: contacts.Contact{GivenName: "Batch" + suffix + "A", FamilyName: "Example", ContainerID: defaultContainerID},
		},
		{
			Contact: contacts.Contact{GivenName: "Batch" + suffix + "B", FamilyName: "Example", ContainerID: defaultContainerID},
		},
	}

	results := make([]batchCreateResult, 0, len(inputs))
	for _, in := range inputs {
		created, err := contacts.CreateContact(ctx, in)
		results = append(results, batchCreateResult{Input: in, Created: created, Err: err})
		if err == nil {
			_ = contacts.DeleteContact(ctx, created.Identifier)
		}
	}
	_ = results
}

func ExampleCreateContact_autoIncrementGivenName() {
	ctx := context.Background()

	created, err := createAutoIncrementContact(ctx, "ExampleSeed ", "AutoIncrement")
	if err != nil {
		return
	}
	_ = contacts.DeleteContact(ctx, created.Identifier)
}

func ExampleAddContactToGroup_syncSelection() {
	ctx := context.Background()

	containerID, err := contacts.DefaultContainerID(ctx)
	if err != nil {
		return
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	group, err := contacts.CreateGroup(ctx, contacts.CreateGroupInput{
		Name:        "Example Sync Group " + suffix,
		ContainerID: containerID,
	})
	if err != nil {
		return
	}
	defer func() { _ = contacts.DeleteGroup(ctx, group.Identifier) }()

	family := "ExampleSync" + suffix
	seedContacts := []contacts.CreateContactInput{
		{Contact: contacts.Contact{GivenName: "Sync" + suffix + "A", FamilyName: family, JobTitle: "Engineer", ContainerID: containerID}},
		{Contact: contacts.Contact{GivenName: "Sync" + suffix + "B", FamilyName: family, JobTitle: "Designer", ContainerID: containerID}},
	}

	createdIDs := make([]string, 0, len(seedContacts))
	for _, in := range seedContacts {
		created, err := contacts.CreateContact(ctx, in)
		if err != nil {
			return
		}
		createdIDs = append(createdIDs, created.Identifier)
	}
	defer func() {
		for _, id := range createdIDs {
			_ = contacts.DeleteContact(ctx, id)
		}
	}()

	existingMembers, err := contacts.ListContactsInGroup(ctx, group.Identifier)
	if err != nil {
		return
	}
	inGroup := make(map[string]struct{}, len(existingMembers))
	for _, c := range existingMembers {
		inGroup[c.Identifier] = struct{}{}
	}

	query := contacts.ListContactsInput{
		Filters: []contacts.Filter{
			{Field: contacts.ContactFieldFamilyName, Op: contacts.FilterEquals, Value: family},
		},
	}

	for c, err := range contacts.ListContacts(ctx, query) {
		if err != nil {
			return
		}
		if !strings.Contains(c.JobTitle, "Engineer") {
			continue
		}
		if _, ok := inGroup[c.Identifier]; ok {
			continue
		}
		if err := contacts.AddContactToGroup(ctx, c.Identifier, group.Identifier); err != nil {
			return
		}
	}

	syncedMembers, err := contacts.ListContactsInGroup(ctx, group.Identifier)
	if err != nil {
		return
	}
	_ = syncedMembers
}

func ExampleGetContact_errorHandling() {
	ctx := context.Background()

	_, err := contacts.GetContact(ctx, "missing-contact-id")
	if err == nil {
		return
	}

	if errors.Is(err, contacts.ErrNotFound) {
		// Branch on typed sentinel errors for control flow.
	}

	var opErr *contacts.OpError
	if errors.As(err, &opErr) {
		_ = fmt.Sprintf("op=%s id=%s", opErr.Op, opErr.ID)
	}
}

func createAutoIncrementContact(ctx context.Context, prefix, familyName string) (contacts.Contact, error) {
	next := 1
	in := contacts.ListContactsInput{
		Filters: []contacts.Filter{
			{Field: contacts.ContactFieldGivenName, Op: contacts.FilterContains, Value: prefix},
		},
	}

	for c, err := range contacts.ListContacts(ctx, in) {
		if err != nil {
			return contacts.Contact{}, err
		}
		if !strings.HasPrefix(c.GivenName, prefix) {
			continue
		}

		suffix := strings.TrimSpace(strings.TrimPrefix(c.GivenName, prefix))
		n, err := strconv.Atoi(suffix)
		if err == nil && n >= next {
			next = n + 1
		}
	}

	return contacts.CreateContact(ctx, contacts.CreateContactInput{
		Contact: contacts.Contact{
			GivenName:  fmt.Sprintf("%s%d", prefix, next),
			FamilyName: familyName,
		},
	})
}
