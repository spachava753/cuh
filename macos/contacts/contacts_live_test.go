package contacts

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nalgeon/be"
)

const (
	contactsLiveTestEnv = "CUH_CONTACTS_LIVE_TEST"
	pollInterval        = 500 * time.Millisecond
	waitTimeout         = 20 * time.Second
)

func TestLivePrimitiveLifecycle(t *testing.T) {
	if os.Getenv(contactsLiveTestEnv) != "1" {
		t.Skipf("set %s=1 to run live Contacts integration tests", contactsLiveTestEnv)
	}

	status, err := AuthorizationStatus()
	be.Err(t, err, nil)
	if status != AuthStatusAuthorized {
		err = RequestAccess()
		if err != nil {
			t.Skipf("contacts permission is required: %v", err)
		}
	}

	_, err = Groups(GroupsInput{Action: GroupsActionList})
	be.Err(t, err, nil)

	groupName := fmt.Sprintf("cuh-contacts-live-%d", time.Now().UnixNano())
	renameGroupName := groupName + "-renamed"
	groupOut, err := Groups(GroupsInput{Action: GroupsActionCreate, Name: groupName})
	be.Err(t, err, nil)

	groupID := firstCreatedGroupID(groupOut)
	if groupID == "" {
		groupID = findGroupIDByName(groupOut.Groups, groupName)
	}
	be.True(t, groupID != "")

	defer func() {
		if groupID == "" {
			return
		}
		_, _ = Groups(GroupsInput{Action: GroupsActionDelete, Group: GroupRef{ID: groupID}})
	}()

	renameOut, err := Groups(GroupsInput{Action: GroupsActionRename, Group: GroupRef{ID: groupID}, Name: renameGroupName})
	be.Err(t, err, nil)
	be.True(t, len(renameOut.Results) > 0)
	be.True(t, renameOut.Results[0].Succeeded)

	marker := fmt.Sprintf("cuh-contacts-live-marker-%d", time.Now().UnixNano())
	upsertOut, err := Upsert(UpsertInput{
		Create: []ContactDraft{{
			GivenName:    marker,
			FamilyName:   "LiveTest",
			Organization: "CUH",
			Emails: []LabeledValue{{
				Label: "work",
				Value: fmt.Sprintf("cuh-live-%d@example.test", time.Now().UnixNano()),
			}},
		}},
	})
	be.Err(t, err, nil)
	be.True(t, len(upsertOut.Results) == 1)
	be.True(t, upsertOut.Results[0].Succeeded)
	be.True(t, upsertOut.Results[0].Ref.ID != "")

	contactRef := upsertOut.Results[0].Ref
	defer func() {
		if contactRef.ID == "" {
			return
		}
		_, _ = Mutate(MutateInput{
			Refs: []Ref{contactRef},
			Ops:  []MutationOp{{Type: MutationDelete}},
		})
	}()

	findOut, err := Find(FindInput{
		Query: Query{IDs: []string{contactRef.ID}},
		Page:  Page{Limit: 10},
	})
	be.Err(t, err, nil)
	be.True(t, containsRefID(findOut.Refs, contactRef.ID))

	getOut, err := Get(GetInput{
		Refs:   []Ref{contactRef},
		Fields: []Field{FieldNames, FieldEmails, FieldGroups, FieldOrganization},
	})
	be.Err(t, err, nil)
	be.True(t, len(getOut.Items) > 0)

	mutateOut, err := Mutate(MutateInput{
		Refs: []Ref{contactRef},
		Ops: []MutationOp{{
			Type:  MutationAddToGroup,
			Value: groupID,
		}},
	})
	be.Err(t, err, nil)
	be.True(t, len(mutateOut.Results) == 1)
	be.True(t, mutateOut.Results[0].Succeeded)

	be.Err(t, waitForGroupMembership(contactRef, groupID, true, waitTimeout), nil)

	mutateOut, err = Mutate(MutateInput{
		Refs: []Ref{contactRef},
		Ops: []MutationOp{{
			Type:  MutationRemoveFromGroup,
			Value: groupID,
		}},
	})
	be.Err(t, err, nil)
	be.True(t, len(mutateOut.Results) == 1)
	if mutateOut.Results[0].Succeeded {
		be.Err(t, waitForGroupMembership(contactRef, groupID, false, waitTimeout), nil)
	} else {
		be.True(t, allowedMembershipRemovalError(mutateOut.Results[0].Err))
	}

	mutateOut, err = Mutate(MutateInput{
		Refs: []Ref{contactRef},
		Ops:  []MutationOp{{Type: MutationDelete}},
	})
	be.Err(t, err, nil)
	be.True(t, len(mutateOut.Results) == 1)
	be.True(t, mutateOut.Results[0].Succeeded)
	contactRef = Ref{}

	be.Err(t, waitForContactMissing(upsertOut.Results[0].Ref.ID, waitTimeout), nil)

	deleteGroupOut, err := Groups(GroupsInput{Action: GroupsActionDelete, Group: GroupRef{ID: groupID}})
	be.Err(t, err, nil)
	be.True(t, len(deleteGroupOut.Results) > 0)
	be.True(t, deleteGroupOut.Results[0].Succeeded)
	groupID = ""
}

func firstCreatedGroupID(out GroupsOutput) string {
	for _, result := range out.Results {
		if result.Succeeded && result.Group.ID != "" {
			return result.Group.ID
		}
	}
	return ""
}

func findGroupIDByName(groups []Group, name string) string {
	for _, group := range groups {
		if strings.EqualFold(group.Name, name) {
			return group.ID
		}
	}
	return ""
}

func containsRefID(refs []Ref, id string) bool {
	for _, ref := range refs {
		if ref.ID == id {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func waitForGroupMembership(ref Ref, groupID string, expected bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		out, err := Get(GetInput{Refs: []Ref{ref}, Fields: []Field{FieldGroups}})
		if err != nil {
			return err
		}
		if len(out.Items) == 0 {
			return fmt.Errorf("contact %q not found while checking group membership", ref.ID)
		}
		hasGroup := containsString(out.Items[0].GroupIDs, groupID)
		if hasGroup == expected {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("group membership %q expected=%v not observed within %s", groupID, expected, timeout)
		}
		time.Sleep(pollInterval)
	}
}

func waitForContactMissing(id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		out, err := Find(FindInput{Query: Query{IDs: []string{id}}, Page: Page{Limit: 5}})
		if err != nil {
			return err
		}
		if !containsRefID(out.Refs, id) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("contact %q still present after %s", id, timeout)
		}
		time.Sleep(pollInterval)
	}
}

func allowedMembershipRemovalError(err error) bool {
	if err == nil {
		return false
	}
	contactErr, ok := err.(*Error)
	if !ok {
		return false
	}
	if contactErr.Code != ErrorCodeConflict {
		return false
	}
	return strings.Contains(strings.ToLower(contactErr.Message), "membership remove did not persist")
}
