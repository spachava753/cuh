package contacts_test

import (
	"fmt"
	"strings"

	"github.com/spachava753/cuh/macos/contacts"
)

func composeUpdateSingleContactOrganization() error {
	findOut, err := contacts.Find(contacts.FindInput{
		Query: contacts.Query{
			NameContains:         "Priya",
			OrganizationContains: "Acme",
			Match:                contacts.MatchAll,
		},
		Page: contacts.Page{Limit: 20},
	})
	if err != nil {
		return err
	}
	if len(findOut.Refs) == 0 {
		return nil
	}

	_, err = contacts.Mutate(contacts.MutateInput{
		Refs: []contacts.Ref{findOut.Refs[0]},
		Ops: []contacts.MutationOp{{
			Type:  contacts.MutationSetOrganization,
			Value: "Acme Ventures",
		}},
	})
	return err
}

func composeCreateGroupAndAddByDomain() error {
	groupsOut, err := contacts.Groups(contacts.GroupsInput{
		Action: contacts.GroupsActionCreate,
		Name:   "Vendors 2026",
	})
	if err != nil {
		return err
	}

	groupID := ""
	for _, result := range groupsOut.Results {
		if result.Succeeded && result.Group.ID != "" {
			groupID = result.Group.ID
			break
		}
	}
	if groupID == "" {
		for _, group := range groupsOut.Groups {
			if strings.EqualFold(group.Name, "Vendors 2026") {
				groupID = group.ID
				break
			}
		}
	}
	if groupID == "" {
		return fmt.Errorf("group not created")
	}

	findOut, err := contacts.Find(contacts.FindInput{
		Query: contacts.Query{EmailDomain: "vendorco.com"},
		Page:  contacts.Page{Limit: 250},
	})
	if err != nil {
		return err
	}
	if len(findOut.Refs) == 0 {
		return nil
	}

	_, err = contacts.Mutate(contacts.MutateInput{
		Refs: findOut.Refs,
		Ops: []contacts.MutationOp{{
			Type:  contacts.MutationAddToGroup,
			Value: groupID,
		}},
	})
	return err
}

func composeCreateThenPatchContact() error {
	upsertOut, err := contacts.Upsert(contacts.UpsertInput{
		Create: []contacts.ContactDraft{{
			GivenName:    "Dr.",
			FamilyName:   "Lee",
			Organization: "Lee Clinic",
			Emails: []contacts.LabeledValue{{
				Label: "work",
				Value: "dr.lee@clinic.example",
			}},
		}},
	})
	if err != nil {
		return err
	}
	if len(upsertOut.Results) == 0 || !upsertOut.Results[0].Succeeded {
		return nil
	}

	jobTitle := "Primary Physician"
	_, err = contacts.Upsert(contacts.UpsertInput{
		Patch: []contacts.ContactPatch{{
			Ref: upsertOut.Results[0].Ref,
			Changes: contacts.ContactChanges{
				JobTitle: &jobTitle,
			},
		}},
	})
	return err
}
