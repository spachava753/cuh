//go:build darwin

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/nalgeon/be"
)

// testPrefix is prepended to all test-created data to avoid collisions
// with the user's real contacts.
const testPrefix = "CUHTest_"

func ptr[T any](v T) *T { return &v }

// cleanup helpers --------------------------------------------------------

func cleanupContact(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	if id == "" {
		return
	}
	err := DeleteContact(ctx, id)
	if err != nil {
		t.Logf("cleanup: delete contact %s: %v", id, err)
	}
}

func cleanupGroup(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	if id == "" {
		return
	}
	err := DeleteGroup(ctx, id)
	if err != nil {
		t.Logf("cleanup: delete group %s: %v", id, err)
	}
}

// authorization ----------------------------------------------------------

func TestCheckAuthorization(t *testing.T) {
	ctx := context.Background()
	status := CheckAuthorization(ctx)
	t.Logf("authorization status: %s (%d)", status, status)
	// If not authorized, skip remaining tests
	if status != AuthorizationStatusAuthorized {
		t.Skipf("contacts access not authorized (status=%s); grant access to Terminal/IDE to run live tests", status)
	}
}

func requireAuthorized(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	status := CheckAuthorization(ctx)
	if status != AuthorizationStatusAuthorized {
		t.Skipf("contacts access not authorized (status=%s)", status)
	}
}

// containers -------------------------------------------------------------

func TestListContainers(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	containers, err := ListContainers(ctx)
	be.Err(t, err, nil)
	be.True(t, len(containers) > 0)

	for _, c := range containers {
		t.Logf("container: id=%s name=%q type=%s", c.Identifier, c.Name, c.ContainerType)
	}
}

func TestDefaultContainerID(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	id, err := DefaultContainerID(ctx)
	be.Err(t, err, nil)
	be.True(t, id != "")
	t.Logf("default container: %s", id)
}

func TestGetContainer(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	id, err := DefaultContainerID(ctx)
	be.Err(t, err, nil)

	c, err := GetContainer(ctx, id)
	be.Err(t, err, nil)
	be.Equal(t, c.Identifier, id)
	t.Logf("container: id=%s name=%q type=%s", c.Identifier, c.Name, c.ContainerType)
}

// contacts CRUD ----------------------------------------------------------

func TestCreateGetDeleteContact(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	defaultContainerID, err := DefaultContainerID(ctx)
	be.Err(t, err, nil)

	// Create
	input := CreateContactInput{
		Contact: Contact{
			ContainerID:      defaultContainerID,
			GivenName:        testPrefix + "John",
			FamilyName:       testPrefix + "Doe",
			Nickname:         "JD",
			JobTitle:         "Engineer",
			OrganizationName: "TestCorp",
			PhoneNumbers: []LabeledValue[string]{
				{Label: "mobile", Value: "+1234567890"},
				{Label: "home", Value: "+0987654321"},
			},
			EmailAddresses: []LabeledValue[string]{
				{Label: "work", Value: "john.doe@test.example.com"},
			},
			PostalAddresses: []LabeledValue[PostalAddress]{
				{Label: "home", Value: PostalAddress{
					Street:     "123 Test St",
					City:       "Testville",
					State:      "TS",
					PostalCode: "12345",
					Country:    "Testland",
				}},
			},
			Birthday: &DateComponents{Year: 1990, Month: 6, Day: 15},
		},
	}

	created, err := CreateContact(ctx, input)
	be.Err(t, err, nil)
	be.True(t, created.Identifier != "")
	be.Equal(t, created.ContainerID, defaultContainerID)
	t.Logf("created contact: %s", created.Identifier)
	defer cleanupContact(t, ctx, created.Identifier)

	// Get
	fetched, err := GetContact(ctx, created.Identifier)
	be.Err(t, err, nil)
	be.Equal(t, fetched.Identifier, created.Identifier)
	be.Equal(t, fetched.ContainerID, defaultContainerID)
	be.Equal(t, fetched.GivenName, testPrefix+"John")
	be.Equal(t, fetched.FamilyName, testPrefix+"Doe")
	be.Equal(t, fetched.Nickname, "JD")
	be.Equal(t, fetched.JobTitle, "Engineer")
	be.Equal(t, fetched.OrganizationName, "TestCorp")
	be.True(t, len(fetched.PhoneNumbers) >= 2)
	be.True(t, len(fetched.EmailAddresses) >= 1)
	be.True(t, len(fetched.PostalAddresses) >= 1)
	be.True(t, fetched.Birthday != nil)
	if fetched.Birthday != nil {
		be.Equal(t, fetched.Birthday.Year, 1990)
		be.Equal(t, fetched.Birthday.Month, 6)
		be.Equal(t, fetched.Birthday.Day, 15)
	}

	// Verify postal address
	if len(fetched.PostalAddresses) > 0 {
		pa := fetched.PostalAddresses[0].Value
		be.Equal(t, pa.Street, "123 Test St")
		be.Equal(t, pa.City, "Testville")
	}

	// FullName
	fullName := fetched.FullName()
	be.True(t, fullName != "")
	t.Logf("full name: %s", fullName)

	// Delete
	err = DeleteContact(ctx, created.Identifier)
	be.Err(t, err, nil)

	// Verify deleted
	_, err = GetContact(ctx, created.Identifier)
	be.Err(t, err)
}

func TestCreateContactOrganization(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	input := CreateContactInput{
		Contact: Contact{
			ContactType:      ContactTypeOrganization,
			OrganizationName: testPrefix + "TestOrg Inc",
			EmailAddresses: []LabeledValue[string]{
				{Label: "work", Value: "info@testorg.example.com"},
			},
		},
	}

	created, err := CreateContact(ctx, input)
	be.Err(t, err, nil)
	defer cleanupContact(t, ctx, created.Identifier)

	be.Equal(t, created.ContactType, ContactTypeOrganization)
	be.Equal(t, created.OrganizationName, testPrefix+"TestOrg Inc")
	t.Logf("org contact full name: %q", created.FullName())
}

func TestUpdateContact(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	created, err := CreateContact(ctx, CreateContactInput{
		Contact: Contact{
			GivenName:  testPrefix + "Update",
			FamilyName: testPrefix + "Contact",
		},
	})
	be.Err(t, err, nil)
	defer cleanupContact(t, ctx, created.Identifier)

	updated, err := UpdateContact(ctx, UpdateContactInput{
		Identifier: created.Identifier,
		Nickname:   ptr("UpdatedNick"),
		JobTitle:   ptr("Staff Engineer"),
	})
	be.Err(t, err, nil)
	be.Equal(t, updated.Nickname, "UpdatedNick")
	be.Equal(t, updated.JobTitle, "Staff Engineer")

	phones := []LabeledValue[string]{{Label: "mobile", Value: "+15550001111"}}
	updated, err = UpdateContact(ctx, UpdateContactInput{
		Identifier:   created.Identifier,
		PhoneNumbers: &phones,
	})
	be.Err(t, err, nil)
	be.Equal(t, len(updated.PhoneNumbers), 1)
}

// ListContacts -----------------------------------------------------------

func TestListContacts(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	// Create two test contacts
	c1, err := CreateContact(ctx, CreateContactInput{
		Contact: Contact{
			GivenName:  testPrefix + "Alice",
			FamilyName: testPrefix + "ListTest",
		},
	})
	be.Err(t, err, nil)
	defer cleanupContact(t, ctx, c1.Identifier)

	c2, err := CreateContact(ctx, CreateContactInput{
		Contact: Contact{
			GivenName:  testPrefix + "Bob",
			FamilyName: testPrefix + "ListTest",
		},
	})
	be.Err(t, err, nil)
	defer cleanupContact(t, ctx, c2.Identifier)

	t.Run("no filters", func(t *testing.T) {
		count := 0
		for _, err := range ListContacts(ctx, ListContactsInput{}) {
			be.Err(t, err, nil)
			count++
		}
		be.True(t, count >= 2)
		t.Logf("total contacts: %d", count)
	})

	t.Run("filter contains", func(t *testing.T) {
		count := 0
		for c, err := range ListContacts(ctx, ListContactsInput{
			Filters: []Filter{
				{Field: ContactFieldFamilyName, Value: testPrefix + "ListTest", Op: FilterEquals},
			},
		}) {
			be.Err(t, err, nil)
			be.Equal(t, c.FamilyName, testPrefix+"ListTest")
			count++
		}
		be.Equal(t, count, 2)
	})

	t.Run("filter not contains", func(t *testing.T) {
		for c, err := range ListContacts(ctx, ListContactsInput{
			Filters: []Filter{
				{Field: ContactFieldGivenName, Value: testPrefix + "Alice", Op: FilterEquals},
				{Field: ContactFieldFamilyName, Value: testPrefix + "ListTest", Op: FilterEquals},
			},
		}) {
			be.Err(t, err, nil)
			be.Equal(t, c.GivenName, testPrefix+"Alice")
		}
	})

	t.Run("offset", func(t *testing.T) {
		count := 0
		for _, err := range ListContacts(ctx, ListContactsInput{
			Filters: []Filter{
				{Field: ContactFieldFamilyName, Value: testPrefix + "ListTest", Op: FilterEquals},
			},
			Offset: 1,
		}) {
			be.Err(t, err, nil)
			count++
		}
		be.Equal(t, count, 1)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx2, cancel := context.WithCancel(ctx)
		cancel() // cancel immediately
		count := 0
		for _, err := range ListContacts(ctx2, ListContactsInput{}) {
			if err != nil {
				be.Equal(t, err, context.Canceled)
				break
			}
			count++
		}
		// Should have seen at most 1 before cancellation, or 0
		t.Logf("contacts yielded before cancel: %d", count)
	})
}

func TestListContactsFilterContains(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	c1, err := CreateContact(ctx, CreateContactInput{
		Contact: Contact{
			GivenName:  testPrefix + "FilterContains",
			FamilyName: testPrefix + "Unique987654",
		},
	})
	be.Err(t, err, nil)
	defer cleanupContact(t, ctx, c1.Identifier)

	count := 0
	for c, err := range ListContacts(ctx, ListContactsInput{
		Filters: []Filter{
			{Field: ContactFieldFamilyName, Value: "Unique987654", Op: FilterContains},
		},
	}) {
		be.Err(t, err, nil)
		be.True(t, c.Identifier != "")
		count++
	}
	be.Equal(t, count, 1)
}

// groups -----------------------------------------------------------------

func TestCreateGetDeleteGroup(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	// Create
	g, err := CreateGroup(ctx, CreateGroupInput{
		Name: testPrefix + "TestGroup",
	})
	be.Err(t, err, nil)
	be.True(t, g.Identifier != "")
	be.Equal(t, g.Name, testPrefix+"TestGroup")
	t.Logf("created group: %s", g.Identifier)
	defer cleanupGroup(t, ctx, g.Identifier)

	// Get
	fetched, err := GetGroup(ctx, g.Identifier)
	be.Err(t, err, nil)
	be.Equal(t, fetched.Identifier, g.Identifier)
	be.Equal(t, fetched.Name, testPrefix+"TestGroup")

	// List groups
	groups, err := ListGroups(ctx, ListGroupsInput{IncludeHierarchy: true})
	be.Err(t, err, nil)
	found := false
	for _, grp := range groups {
		if grp.Identifier == g.Identifier {
			found = true
			break
		}
	}
	be.True(t, found)

	// Delete
	err = DeleteGroup(ctx, g.Identifier)
	be.Err(t, err, nil)

	// Verify deleted
	_, err = GetGroup(ctx, g.Identifier)
	be.Err(t, err)
}

func TestListGroupsInContainer(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	defaultID, err := DefaultContainerID(ctx)
	be.Err(t, err, nil)

	g, err := CreateGroup(ctx, CreateGroupInput{
		Name: testPrefix + "ContainerGroup",
	})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, g.Identifier)

	groups, err := ListGroups(ctx, ListGroupsInput{ContainerID: defaultID, IncludeHierarchy: true})
	be.Err(t, err, nil)

	found := false
	for _, grp := range groups {
		if grp.Identifier == g.Identifier {
			found = true
			break
		}
	}
	be.True(t, found)
}

// membership -------------------------------------------------------------

func TestAddRemoveContactGroup(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	// Create contact
	c, err := CreateContact(ctx, CreateContactInput{
		Contact: Contact{
			GivenName:  testPrefix + "Member",
			FamilyName: testPrefix + "GroupTest",
		},
	})
	be.Err(t, err, nil)
	defer cleanupContact(t, ctx, c.Identifier)

	// Create group
	g, err := CreateGroup(ctx, CreateGroupInput{
		Name: testPrefix + "MembershipGroup",
	})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, g.Identifier)

	// Add contact to group
	err = AddContactToGroup(ctx, c.Identifier, g.Identifier)
	be.Err(t, err, nil)

	// Verify membership via ListContactsInGroup
	members, err := ListContactsInGroup(ctx, g.Identifier)
	be.Err(t, err, nil)
	found := false
	for _, m := range members {
		if m.Identifier == c.Identifier {
			found = true
			break
		}
	}
	be.True(t, found)

	// Remove contact from group (uses osascript workaround).
	err = RemoveContactFromGroup(ctx, c.Identifier, g.Identifier)
	be.Err(t, err, nil)

	// Verify removed
	members, err = ListContactsInGroup(ctx, g.Identifier)
	be.Err(t, err, nil)
	found = false
	for _, m := range members {
		if m.Identifier == c.Identifier {
			found = true
			break
		}
	}
	be.True(t, !found)
}

func TestDeleteGroupWithContacts(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	// Create contact
	c, err := CreateContact(ctx, CreateContactInput{
		Contact: Contact{
			GivenName:  testPrefix + "InGroup",
			FamilyName: testPrefix + "DeleteGroupTest",
		},
	})
	be.Err(t, err, nil)
	defer cleanupContact(t, ctx, c.Identifier)

	// Create group and add contact
	g, err := CreateGroup(ctx, CreateGroupInput{
		Name: testPrefix + "DeleteGroupWithMembers",
	})
	be.Err(t, err, nil)

	err = AddContactToGroup(ctx, c.Identifier, g.Identifier)
	be.Err(t, err, nil)

	// Delete group — contact should survive
	err = DeleteGroup(ctx, g.Identifier)
	be.Err(t, err, nil)

	// Contact still exists
	fetched, err := GetContact(ctx, c.Identifier)
	be.Err(t, err, nil)
	be.Equal(t, fetched.GivenName, testPrefix+"InGroup")
}

// subgroups (parent/child groups) ----------------------------------------

func TestCreateSubgroup(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	parent, err := CreateGroup(ctx, CreateGroupInput{
		Name: testPrefix + "ParentGroup",
	})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, parent.Identifier)

	child, err := CreateGroup(ctx, CreateGroupInput{
		Name:          testPrefix + "ChildGroup",
		ParentGroupID: parent.Identifier,
	})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, child.Identifier)

	be.True(t, child.Identifier != "")
	be.Equal(t, child.Name, testPrefix+"ChildGroup")
	be.Equal(t, child.ParentGroupID, parent.Identifier)

	subgroups, err := ListSubgroups(ctx, parent.Identifier)
	be.Err(t, err, nil)
	found := false
	for _, sg := range subgroups {
		if sg.Identifier == child.Identifier {
			found = true
			break
		}
	}
	be.True(t, found)
}

func TestUpdateGroup(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	parentA, err := CreateGroup(ctx, CreateGroupInput{Name: testPrefix + "ParentA"})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, parentA.Identifier)

	parentB, err := CreateGroup(ctx, CreateGroupInput{Name: testPrefix + "ParentB"})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, parentB.Identifier)

	child, err := CreateGroup(ctx, CreateGroupInput{
		Name:          testPrefix + "ChildToMove",
		ParentGroupID: parentA.Identifier,
	})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, child.Identifier)

	newName := testPrefix + "ChildMoved"
	updated, err := UpdateGroup(ctx, UpdateGroupInput{
		Identifier:    child.Identifier,
		Name:          &newName,
		ParentGroupID: &parentB.Identifier,
	})
	be.Err(t, err, nil)
	be.Equal(t, updated.Name, newName)
	be.Equal(t, updated.ParentGroupID, parentB.Identifier)
}

// edge cases / error handling --------------------------------------------

func TestGetContactNotFound(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	_, err := GetContact(ctx, "nonexistent-identifier-12345")
	be.Err(t, err)
	be.True(t, errors.Is(err, ErrNotFound))
	t.Logf("expected error: %v", err)
}

func TestGetGroupNotFound(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	_, err := GetGroup(ctx, "nonexistent-group-12345")
	be.Err(t, err)
	be.True(t, errors.Is(err, ErrNotFound))
	t.Logf("expected error: %v", err)
}

func TestDeleteContactNotFound(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	err := DeleteContact(ctx, "nonexistent-identifier-12345")
	be.Err(t, err)
	be.True(t, errors.Is(err, ErrNotFound))
	t.Logf("expected error: %v", err)
}

func TestDeleteGroupNotFound(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	err := DeleteGroup(ctx, "nonexistent-group-12345")
	be.Err(t, err)
	be.True(t, errors.Is(err, ErrNotFound))
	t.Logf("expected error: %v", err)
}

func TestEmptyIdentifier(t *testing.T) {
	ctx := context.Background()

	_, err := GetContact(ctx, "")
	be.Err(t, err)

	err = DeleteContact(ctx, "")
	be.Err(t, err)

	_, err = GetGroup(ctx, "")
	be.Err(t, err)

	err = DeleteGroup(ctx, "")
	be.Err(t, err)

	err = AddContactToGroup(ctx, "", "group")
	be.Err(t, err)

	err = AddContactToGroup(ctx, "contact", "")
	be.Err(t, err)

	err = RemoveContactFromGroup(ctx, "", "group")
	be.Err(t, err)

	_, err = GetContainer(ctx, "")
	be.Err(t, err)

	_, err = ListContactsInGroup(ctx, "")
	be.Err(t, err)
}

func TestCreateGroupEmptyName(t *testing.T) {
	ctx := context.Background()
	_, err := CreateGroup(ctx, CreateGroupInput{Name: ""})
	be.Err(t, err)
	be.True(t, errors.Is(err, ErrInvalidArgument))
}

// ListContactsInGroup with empty group ------------------------------------

func TestListContactsInGroupEmpty(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	g, err := CreateGroup(ctx, CreateGroupInput{
		Name: testPrefix + "EmptyGroup",
	})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, g.Identifier)

	members, err := ListContactsInGroup(ctx, g.Identifier)
	be.Err(t, err, nil)
	be.Equal(t, len(members), 0)
}

// Multiple contacts in group ----------------------------------------------

func TestMultipleContactsInGroup(t *testing.T) {
	requireAuthorized(t)
	ctx := context.Background()

	g, err := CreateGroup(ctx, CreateGroupInput{
		Name: testPrefix + "MultiMemberGroup",
	})
	be.Err(t, err, nil)
	defer cleanupGroup(t, ctx, g.Identifier)

	var contactIDs []string
	for i := 0; i < 3; i++ {
		c, err := CreateContact(ctx, CreateContactInput{
			Contact: Contact{
				GivenName:  testPrefix + "Multi",
				FamilyName: testPrefix + "Member",
			},
		})
		be.Err(t, err, nil)
		contactIDs = append(contactIDs, c.Identifier)
		defer cleanupContact(t, ctx, c.Identifier)

		err = AddContactToGroup(ctx, c.Identifier, g.Identifier)
		be.Err(t, err, nil)
	}

	members, err := ListContactsInGroup(ctx, g.Identifier)
	be.Err(t, err, nil)
	be.Equal(t, len(members), 3)
}

// String() methods -------------------------------------------------------

func TestValidateFilters(t *testing.T) {
	err := ValidateFilters([]Filter{{Field: ContactField("note"), Value: "x", Op: FilterContains}})
	be.Err(t, err)
	be.True(t, errors.Is(err, ErrInvalidArgument))
}

func TestStringMethods(t *testing.T) {
	be.Equal(t, ContactTypePerson.String(), "person")
	be.Equal(t, ContactTypeOrganization.String(), "organization")
	be.Equal(t, ContainerTypeLocal.String(), "local")
	be.Equal(t, ContainerTypeCardDAV.String(), "cardDAV")
	be.Equal(t, ContainerTypeExchange.String(), "exchange")
	be.Equal(t, ContainerTypeUnassigned.String(), "unassigned")
	be.Equal(t, AuthorizationStatusAuthorized.String(), "authorized")
	be.Equal(t, AuthorizationStatusDenied.String(), "denied")
	be.Equal(t, AuthorizationStatusRestricted.String(), "restricted")
	be.Equal(t, AuthorizationStatusNotDetermined.String(), "not_determined")
}

// FullName edge cases -----------------------------------------------------

func TestFullNameEmpty(t *testing.T) {
	c := Contact{}
	be.Equal(t, c.FullName(), "")
}

func TestFullNameOrganization(t *testing.T) {
	c := Contact{
		ContactType:      ContactTypeOrganization,
		OrganizationName: "Acme Corp",
	}
	be.Equal(t, c.FullName(), "Acme Corp")
}

func TestFullNameWithPrefix(t *testing.T) {
	c := Contact{
		NamePrefix: "Dr.",
		GivenName:  "Jane",
		FamilyName: "Smith",
		NameSuffix: "PhD",
	}
	be.Equal(t, c.FullName(), "Dr. Jane Smith PhD")
}
