//go:build darwin && cgo

package contacts

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Contacts
#include "bridge_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

type cleanupStack struct {
	fns []func()
}

func (c *cleanupStack) add(fn func()) {
	if fn != nil {
		c.fns = append(c.fns, fn)
	}
}

func (c *cleanupStack) done() {
	for i := len(c.fns) - 1; i >= 0; i-- {
		c.fns[i]()
	}
}

func cString(cs *cleanupStack, s string) *C.char {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	p := C.CString(s)
	cs.add(func() { C.free(unsafe.Pointer(p)) })
	return p
}

func goString(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func toCStringArray(cs *cleanupStack, values []string) (**C.char, C.int) {
	if len(values) == 0 {
		return nil, 0
	}
	ptr := C.malloc(C.size_t(len(values)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	if ptr == nil {
		return nil, 0
	}
	arr := unsafe.Slice((**C.char)(ptr), len(values))
	for i, value := range values {
		arr[i] = C.CString(value)
	}
	cs.add(func() {
		for i := range arr {
			if arr[i] != nil {
				C.free(unsafe.Pointer(arr[i]))
			}
		}
		C.free(ptr)
	})
	return (**C.char)(ptr), C.int(len(values))
}

func toCRefs(cs *cleanupStack, refs []Ref) (*C.ContactsRef, C.int) {
	if len(refs) == 0 {
		return nil, 0
	}
	ptr := C.malloc(C.size_t(len(refs)) * C.size_t(C.sizeof_ContactsRef))
	if ptr == nil {
		return nil, 0
	}
	arr := unsafe.Slice((*C.ContactsRef)(ptr), len(refs))
	for i := range refs {
		arr[i] = C.ContactsRef{
			id:           C.CString(refs[i].ID),
			container_id: C.CString(refs[i].ContainerID),
			account_id:   C.CString(refs[i].AccountID),
		}
	}
	cs.add(func() {
		for i := range arr {
			if arr[i].id != nil {
				C.free(unsafe.Pointer(arr[i].id))
			}
			if arr[i].container_id != nil {
				C.free(unsafe.Pointer(arr[i].container_id))
			}
			if arr[i].account_id != nil {
				C.free(unsafe.Pointer(arr[i].account_id))
			}
		}
		C.free(ptr)
	})
	return (*C.ContactsRef)(ptr), C.int(len(refs))
}

func toCLabeledValues(cs *cleanupStack, values []LabeledValue) (*C.ContactsLabeledValue, C.int) {
	if len(values) == 0 {
		return nil, 0
	}
	ptr := C.malloc(C.size_t(len(values)) * C.size_t(C.sizeof_ContactsLabeledValue))
	if ptr == nil {
		return nil, 0
	}
	arr := unsafe.Slice((*C.ContactsLabeledValue)(ptr), len(values))
	for i := range values {
		arr[i] = C.ContactsLabeledValue{
			label: C.CString(values[i].Label),
			value: C.CString(values[i].Value),
		}
	}
	cs.add(func() {
		for i := range arr {
			if arr[i].label != nil {
				C.free(unsafe.Pointer(arr[i].label))
			}
			if arr[i].value != nil {
				C.free(unsafe.Pointer(arr[i].value))
			}
		}
		C.free(ptr)
	})
	return (*C.ContactsLabeledValue)(ptr), C.int(len(values))
}

func errorCodeFromC(code C.int) ErrorCode {
	switch code {
	case C.CONTACTS_ERR_PERMISSION_DENIED:
		return ErrorCodePermissionDenied
	case C.CONTACTS_ERR_NOT_FOUND:
		return ErrorCodeNotFound
	case C.CONTACTS_ERR_CONFLICT:
		return ErrorCodeConflict
	case C.CONTACTS_ERR_VALIDATION:
		return ErrorCodeValidation
	case C.CONTACTS_ERR_STORE:
		return ErrorCodeStore
	default:
		return ErrorCodeUnknown
	}
}

func errorFromC(cerr C.ContactsError) error {
	if cerr.code == C.CONTACTS_ERR_NONE {
		msg := strings.TrimSpace(goString(cerr.message))
		if msg == "" {
			return nil
		}
	}
	return &Error{Code: errorCodeFromC(cerr.code), Message: strings.TrimSpace(goString(cerr.message))}
}

func writeResultFromC(item C.ContactsWriteResult) WriteResult {
	result := WriteResult{
		Ref: Ref{
			ID:          goString(item.ref.id),
			ContainerID: goString(item.ref.container_id),
			AccountID:   goString(item.ref.account_id),
		},
		Succeeded: item.succeeded != 0,
		Created:   item.created != 0,
		Updated:   item.updated != 0,
	}
	if item.error_code != C.CONTACTS_ERR_NONE || strings.TrimSpace(goString(item.error_message)) != "" {
		result.Err = &Error{Code: errorCodeFromC(item.error_code), Message: strings.TrimSpace(goString(item.error_message))}
	}
	return result
}

func parseCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("contacts: invalid cursor %q", cursor)
	}
	return offset, nil
}

func authorizationStatus() (AuthStatus, error) {
	switch C.contacts_authorization_status() {
	case C.CONTACTS_AUTH_NOT_DETERMINED:
		return AuthStatusNotDetermined, nil
	case C.CONTACTS_AUTH_RESTRICTED:
		return AuthStatusRestricted, nil
	case C.CONTACTS_AUTH_DENIED:
		return AuthStatusDenied, nil
	case C.CONTACTS_AUTH_AUTHORIZED:
		return AuthStatusAuthorized, nil
	default:
		return "", &Error{Code: ErrorCodeUnknown, Message: "unknown authorization status"}
	}
}

func requestAccess() error {
	var cerr C.ContactsError
	defer C.contacts_free_error(&cerr)
	if C.contacts_request_access(&cerr) == 0 {
		return errorFromC(cerr)
	}
	return nil
}

func find(input FindInput) (FindOutput, error) {
	offset, err := parseCursor(input.Page.Cursor)
	if err != nil {
		return FindOutput{}, err
	}

	match := C.int(C.CONTACTS_MATCH_ALL)
	if input.Query.Match == MatchAny {
		match = C.CONTACTS_MATCH_ANY
	}

	sortBy := C.int(C.CONTACTS_SORT_GIVEN_NAME)
	if input.Sort.By == SortByFamilyName {
		sortBy = C.CONTACTS_SORT_FAMILY_NAME
	}
	sortOrder := C.int(C.CONTACTS_SORT_ASC)
	if input.Sort.Order == SortOrderDesc {
		sortOrder = C.CONTACTS_SORT_DESC
	}

	cs := &cleanupStack{}
	defer cs.done()

	groupIDs, groupIDsLen := toCStringArray(cs, input.Query.GroupIDsAny)
	ids, idsLen := toCStringArray(cs, input.Query.IDs)

	req := C.ContactsFindRequest{
		name_contains:         cString(cs, input.Query.NameContains),
		organization_contains: cString(cs, input.Query.OrganizationContains),
		email_domain:          cString(cs, input.Query.EmailDomain),
		note_contains:         cString(cs, input.Query.NoteContains),
		group_ids_any:         groupIDs,
		group_ids_any_len:     groupIDsLen,
		ids:                   ids,
		ids_len:               idsLen,
		match_policy:          match,
		limit:                 C.int(input.Page.Limit),
		offset:                C.int(offset),
		include_meta:          C.int(0),
		sort_by:               sortBy,
		sort_order:            sortOrder,
	}
	if input.IncludeMeta {
		req.include_meta = 1
	}

	var out C.ContactsFindResult
	var cerr C.ContactsError
	defer C.contacts_free_error(&cerr)
	if C.contacts_find(&req, &out, &cerr) == 0 {
		return FindOutput{}, errorFromC(cerr)
	}
	defer C.contacts_free_find_result(&out)

	items := unsafe.Slice(out.items, int(out.items_len))
	findOut := FindOutput{
		Refs: make([]Ref, 0, len(items)),
	}
	if input.IncludeMeta {
		findOut.Meta = make([]Meta, 0, len(items))
	}

	for i := range items {
		ref := Ref{
			ID:          goString(items[i].id),
			ContainerID: goString(items[i].container_id),
			AccountID:   goString(items[i].account_id),
		}
		findOut.Refs = append(findOut.Refs, ref)
		if input.IncludeMeta {
			meta := Meta{
				Ref:          ref,
				DisplayName:  goString(items[i].display_name),
				Organization: goString(items[i].organization),
			}
			if items[i].modified_at_unix > 0 {
				meta.ModifiedAt = time.Unix(int64(items[i].modified_at_unix), 0)
			}
			findOut.Meta = append(findOut.Meta, meta)
		}
	}
	if out.next_offset >= 0 {
		findOut.NextCursor = strconv.Itoa(int(out.next_offset))
	}

	return findOut, nil
}

func fieldMask(fields []Field) C.uint32_t {
	if len(fields) == 0 {
		return 0
	}
	var mask C.uint32_t
	for _, field := range fields {
		switch field {
		case FieldNames:
			mask |= C.CONTACTS_FIELD_NAMES
		case FieldOrganization:
			mask |= C.CONTACTS_FIELD_ORGANIZATION
		case FieldEmails:
			mask |= C.CONTACTS_FIELD_EMAILS
		case FieldPhones:
			mask |= C.CONTACTS_FIELD_PHONES
		case FieldNote:
			mask |= C.CONTACTS_FIELD_NOTE
		case FieldGroups:
			mask |= C.CONTACTS_FIELD_GROUPS
		}
	}
	return mask
}

func get(input GetInput) (GetOutput, error) {
	cs := &cleanupStack{}
	defer cs.done()

	refs, refsLen := toCRefs(cs, input.Refs)
	req := C.ContactsGetRequest{
		refs:       refs,
		refs_len:   refsLen,
		field_mask: fieldMask(input.Fields),
	}

	var out C.ContactsGetResult
	var cerr C.ContactsError
	defer C.contacts_free_error(&cerr)
	if C.contacts_get(&req, &out, &cerr) == 0 {
		return GetOutput{}, errorFromC(cerr)
	}
	defer C.contacts_free_get_result(&out)

	cItems := unsafe.Slice(out.items, int(out.items_len))
	result := GetOutput{Items: make([]Item, 0, len(cItems))}
	for i := range cItems {
		item := Item{
			Ref: Ref{
				ID:          goString(cItems[i].ref.id),
				ContainerID: goString(cItems[i].ref.container_id),
				AccountID:   goString(cItems[i].ref.account_id),
			},
			GivenName:    goString(cItems[i].given_name),
			FamilyName:   goString(cItems[i].family_name),
			MiddleName:   goString(cItems[i].middle_name),
			Nickname:     goString(cItems[i].nickname),
			Organization: goString(cItems[i].organization),
			JobTitle:     goString(cItems[i].job_title),
			Note:         goString(cItems[i].note),
		}
		if cItems[i].modified_at_unix > 0 {
			item.ModifiedAt = time.Unix(int64(cItems[i].modified_at_unix), 0)
		}

		emails := unsafe.Slice(cItems[i].emails, int(cItems[i].emails_len))
		if len(emails) > 0 {
			item.Emails = make([]LabeledValue, 0, len(emails))
			for j := range emails {
				item.Emails = append(item.Emails, LabeledValue{Label: goString(emails[j].label), Value: goString(emails[j].value)})
			}
		}

		phones := unsafe.Slice(cItems[i].phones, int(cItems[i].phones_len))
		if len(phones) > 0 {
			item.Phones = make([]LabeledValue, 0, len(phones))
			for j := range phones {
				item.Phones = append(item.Phones, LabeledValue{Label: goString(phones[j].label), Value: goString(phones[j].value)})
			}
		}

		groups := unsafe.Slice(cItems[i].group_ids, int(cItems[i].group_ids_len))
		if len(groups) > 0 {
			item.GroupIDs = make([]string, 0, len(groups))
			for j := range groups {
				item.GroupIDs = append(item.GroupIDs, goString(groups[j]))
			}
		}

		result.Items = append(result.Items, item)
	}
	return result, nil
}

func toCDrafts(cs *cleanupStack, drafts []ContactDraft) (*C.ContactsDraft, C.int) {
	if len(drafts) == 0 {
		return nil, 0
	}
	ptr := C.malloc(C.size_t(len(drafts)) * C.size_t(C.sizeof_ContactsDraft))
	if ptr == nil {
		return nil, 0
	}
	arr := unsafe.Slice((*C.ContactsDraft)(ptr), len(drafts))
	for i := range drafts {
		emails, emailsLen := toCLabeledValues(cs, drafts[i].Emails)
		phones, phonesLen := toCLabeledValues(cs, drafts[i].Phones)
		groupIDs, groupIDsLen := toCStringArray(cs, drafts[i].GroupIDs)
		arr[i] = C.ContactsDraft{
			container_id:  cString(cs, drafts[i].ContainerID),
			given_name:    cString(cs, drafts[i].GivenName),
			family_name:   cString(cs, drafts[i].FamilyName),
			middle_name:   cString(cs, drafts[i].MiddleName),
			nickname:      cString(cs, drafts[i].Nickname),
			organization:  cString(cs, drafts[i].Organization),
			job_title:     cString(cs, drafts[i].JobTitle),
			note:          cString(cs, drafts[i].Note),
			emails:        emails,
			emails_len:    emailsLen,
			phones:        phones,
			phones_len:    phonesLen,
			group_ids:     groupIDs,
			group_ids_len: groupIDsLen,
		}
	}
	cs.add(func() { C.free(ptr) })
	return (*C.ContactsDraft)(ptr), C.int(len(drafts))
}

func toCPatches(cs *cleanupStack, patches []ContactPatch) (*C.ContactsPatch, C.int) {
	if len(patches) == 0 {
		return nil, 0
	}
	ptr := C.malloc(C.size_t(len(patches)) * C.size_t(C.sizeof_ContactsPatch))
	if ptr == nil {
		return nil, 0
	}
	arr := unsafe.Slice((*C.ContactsPatch)(ptr), len(patches))
	for i := range patches {
		cp := C.ContactsPatch{}
		cp.ref = C.ContactsRef{
			id:           C.CString(patches[i].Ref.ID),
			container_id: C.CString(patches[i].Ref.ContainerID),
			account_id:   C.CString(patches[i].Ref.AccountID),
		}
		cs.add(func() {
			if cp.ref.id != nil {
				C.free(unsafe.Pointer(cp.ref.id))
			}
			if cp.ref.container_id != nil {
				C.free(unsafe.Pointer(cp.ref.container_id))
			}
			if cp.ref.account_id != nil {
				C.free(unsafe.Pointer(cp.ref.account_id))
			}
		})

		if patches[i].Changes.GivenName != nil {
			cp.set_given_name = 1
			cp.given_name = cString(cs, *patches[i].Changes.GivenName)
		}
		if patches[i].Changes.FamilyName != nil {
			cp.set_family_name = 1
			cp.family_name = cString(cs, *patches[i].Changes.FamilyName)
		}
		if patches[i].Changes.MiddleName != nil {
			cp.set_middle_name = 1
			cp.middle_name = cString(cs, *patches[i].Changes.MiddleName)
		}
		if patches[i].Changes.Nickname != nil {
			cp.set_nickname = 1
			cp.nickname = cString(cs, *patches[i].Changes.Nickname)
		}
		if patches[i].Changes.Organization != nil {
			cp.set_organization = 1
			cp.organization = cString(cs, *patches[i].Changes.Organization)
		}
		if patches[i].Changes.JobTitle != nil {
			cp.set_job_title = 1
			cp.job_title = cString(cs, *patches[i].Changes.JobTitle)
		}
		if patches[i].Changes.Note != nil {
			cp.set_note = 1
			cp.note = cString(cs, *patches[i].Changes.Note)
		}
		if patches[i].Changes.Emails != nil {
			cp.set_emails = 1
			emails, emailsLen := toCLabeledValues(cs, *patches[i].Changes.Emails)
			cp.replace_emails = emails
			cp.replace_emails_len = emailsLen
		}
		if patches[i].Changes.Phones != nil {
			cp.set_phones = 1
			phones, phonesLen := toCLabeledValues(cs, *patches[i].Changes.Phones)
			cp.replace_phones = phones
			cp.replace_phones_len = phonesLen
		}

		addGroups, addGroupsLen := toCStringArray(cs, patches[i].Changes.AddGroupIDs)
		removeGroups, removeGroupsLen := toCStringArray(cs, patches[i].Changes.RemoveGroupIDs)
		cp.add_group_ids = addGroups
		cp.add_group_ids_len = addGroupsLen
		cp.remove_group_ids = removeGroups
		cp.remove_group_ids_len = removeGroupsLen

		arr[i] = cp
	}
	cs.add(func() { C.free(ptr) })
	return (*C.ContactsPatch)(ptr), C.int(len(patches))
}

func upsert(input UpsertInput) (UpsertOutput, error) {
	cs := &cleanupStack{}
	defer cs.done()

	creates, createsLen := toCDrafts(cs, input.Create)
	patches, patchesLen := toCPatches(cs, input.Patch)
	req := C.ContactsUpsertRequest{
		creates:     creates,
		creates_len: createsLen,
		patches:     patches,
		patches_len: patchesLen,
	}

	var out C.ContactsUpsertResult
	var cerr C.ContactsError
	defer C.contacts_free_error(&cerr)
	if C.contacts_upsert(&req, &out, &cerr) == 0 {
		return UpsertOutput{}, errorFromC(cerr)
	}
	defer C.contacts_free_upsert_result(&out)

	items := unsafe.Slice(out.items, int(out.items_len))
	res := UpsertOutput{Results: make([]WriteResult, 0, len(items))}
	for i := range items {
		res.Results = append(res.Results, writeResultFromC(items[i]))
	}
	return res, nil
}

func mutationTypeToC(t MutationType) C.int {
	switch t {
	case MutationSetNote:
		return C.CONTACTS_MUTATION_SET_NOTE
	case MutationSetOrganization:
		return C.CONTACTS_MUTATION_SET_ORGANIZATION
	case MutationSetJobTitle:
		return C.CONTACTS_MUTATION_SET_JOB_TITLE
	case MutationSetGivenName:
		return C.CONTACTS_MUTATION_SET_GIVEN_NAME
	case MutationSetFamilyName:
		return C.CONTACTS_MUTATION_SET_FAMILY_NAME
	case MutationAddToGroup:
		return C.CONTACTS_MUTATION_ADD_TO_GROUP
	case MutationRemoveFromGroup:
		return C.CONTACTS_MUTATION_REMOVE_FROM_GROUP
	case MutationDelete:
		return C.CONTACTS_MUTATION_DELETE
	default:
		return 0
	}
}

func toCMutationOps(cs *cleanupStack, ops []MutationOp) (*C.ContactsMutationOp, C.int) {
	if len(ops) == 0 {
		return nil, 0
	}
	ptr := C.malloc(C.size_t(len(ops)) * C.size_t(C.sizeof_ContactsMutationOp))
	if ptr == nil {
		return nil, 0
	}
	arr := unsafe.Slice((*C.ContactsMutationOp)(ptr), len(ops))
	for i := range ops {
		arr[i] = C.ContactsMutationOp{
			_type: mutationTypeToC(ops[i].Type),
			value: C.CString(ops[i].Value),
		}
	}
	cs.add(func() {
		for i := range arr {
			if arr[i].value != nil {
				C.free(unsafe.Pointer(arr[i].value))
			}
		}
		C.free(ptr)
	})
	return (*C.ContactsMutationOp)(ptr), C.int(len(ops))
}

func mutate(input MutateInput) (MutateOutput, error) {
	cs := &cleanupStack{}
	defer cs.done()

	refs, refsLen := toCRefs(cs, input.Refs)
	ops, opsLen := toCMutationOps(cs, input.Ops)
	req := C.ContactsMutateRequest{
		refs:     refs,
		refs_len: refsLen,
		ops:      ops,
		ops_len:  opsLen,
	}

	var out C.ContactsMutateResult
	var cerr C.ContactsError
	defer C.contacts_free_error(&cerr)
	if C.contacts_mutate(&req, &out, &cerr) == 0 {
		return MutateOutput{}, errorFromC(cerr)
	}
	defer C.contacts_free_mutate_result(&out)

	items := unsafe.Slice(out.items, int(out.items_len))
	res := MutateOutput{Results: make([]WriteResult, 0, len(items))}
	for i := range items {
		res.Results = append(res.Results, writeResultFromC(items[i]))
	}
	return res, nil
}

func groupsActionToC(action GroupsAction) C.int {
	switch action {
	case GroupsActionList:
		return C.CONTACTS_GROUPS_LIST
	case GroupsActionCreate:
		return C.CONTACTS_GROUPS_CREATE
	case GroupsActionRename:
		return C.CONTACTS_GROUPS_RENAME
	case GroupsActionDelete:
		return C.CONTACTS_GROUPS_DELETE
	default:
		return C.CONTACTS_GROUPS_LIST
	}
}

func groups(input GroupsInput) (GroupsOutput, error) {
	cs := &cleanupStack{}
	defer cs.done()

	req := C.ContactsGroupsRequest{
		action:       groupsActionToC(input.Action),
		group_id:     cString(cs, input.Group.ID),
		name:         cString(cs, input.Name),
		container_id: cString(cs, input.ContainerID),
	}

	var out C.ContactsGroupsResult
	var cerr C.ContactsError
	defer C.contacts_free_error(&cerr)
	if C.contacts_groups(&req, &out, &cerr) == 0 {
		return GroupsOutput{}, errorFromC(cerr)
	}
	defer C.contacts_free_groups_result(&out)

	groupsOut := GroupsOutput{}

	cg := unsafe.Slice(out.groups, int(out.groups_len))
	if len(cg) > 0 {
		groupsOut.Groups = make([]Group, 0, len(cg))
		for i := range cg {
			groupsOut.Groups = append(groupsOut.Groups, Group{
				GroupRef: GroupRef{
					ID:          goString(cg[i].id),
					ContainerID: goString(cg[i].container_id),
					AccountID:   goString(cg[i].account_id),
				},
				Name: goString(cg[i].name),
			})
		}
	}

	cr := unsafe.Slice(out.results, int(out.results_len))
	if len(cr) > 0 {
		groupsOut.Results = make([]GroupResult, 0, len(cr))
		for i := range cr {
			gr := GroupResult{
				Group: GroupRef{
					ID:          goString(cr[i].ref.id),
					ContainerID: goString(cr[i].ref.container_id),
					AccountID:   goString(cr[i].ref.account_id),
				},
				Succeeded: cr[i].succeeded != 0,
				Created:   cr[i].created != 0,
				Updated:   cr[i].updated != 0,
			}
			if cr[i].error_code != C.CONTACTS_ERR_NONE || strings.TrimSpace(goString(cr[i].error_message)) != "" {
				gr.Err = &Error{Code: errorCodeFromC(cr[i].error_code), Message: strings.TrimSpace(goString(cr[i].error_message))}
			}
			groupsOut.Results = append(groupsOut.Results, gr)
		}
	}

	return groupsOut, nil
}
