package contacts

import (
	"errors"
	"fmt"
	"time"
)

// ErrUnsupportedPlatform is returned when the contacts backend is unavailable on
// the current OS/runtime.
var ErrUnsupportedPlatform = errors.New("contacts: unsupported platform")

// AuthStatus describes Contacts permission state for the current process.
type AuthStatus string

const (
	// AuthStatusNotDetermined indicates access has not been requested yet.
	AuthStatusNotDetermined AuthStatus = "not_determined"
	// AuthStatusRestricted indicates policy restrictions prevent access.
	AuthStatusRestricted AuthStatus = "restricted"
	// AuthStatusDenied indicates the user denied access.
	AuthStatusDenied AuthStatus = "denied"
	// AuthStatusAuthorized indicates Contacts access is granted.
	AuthStatusAuthorized AuthStatus = "authorized"
)

// ErrorCode classifies backend errors from Contacts.framework.
type ErrorCode string

const (
	// ErrorCodePermissionDenied indicates authorization is missing.
	ErrorCodePermissionDenied ErrorCode = "permission_denied"
	// ErrorCodeNotFound indicates a referenced contact/group does not exist.
	ErrorCodeNotFound ErrorCode = "not_found"
	// ErrorCodeConflict indicates write conflicts or duplicate state.
	ErrorCodeConflict ErrorCode = "conflict"
	// ErrorCodeValidation indicates invalid input or policy validation failures.
	ErrorCodeValidation ErrorCode = "validation"
	// ErrorCodeStore indicates a storage/backend failure.
	ErrorCodeStore ErrorCode = "store"
	// ErrorCodeUnknown indicates an unmapped error.
	ErrorCodeUnknown ErrorCode = "unknown"
)

// Error is a typed package error for backend operations.
type Error struct {
	Code    ErrorCode
	Message string
}

// Error returns the formatted error message.
func (e *Error) Error() string {
	if e == nil {
		return "contacts: <nil>"
	}
	if e.Message == "" {
		return fmt.Sprintf("contacts: %s", e.Code)
	}
	return fmt.Sprintf("contacts: %s: %s", e.Code, e.Message)
}

// Ref is a stable contact reference shared across primitives.
//
// ContainerID and AccountID are included for account/container-scoped workflows.
// AccountID is backend-defined and may equal ContainerID when the platform does
// not expose a distinct account identifier.
type Ref struct {
	ID          string
	ContainerID string
	AccountID   string
}

// GroupRef is a stable group reference shared across group operations.
type GroupRef struct {
	ID          string
	ContainerID string
	AccountID   string
}

// MatchPolicy controls query clause evaluation.
type MatchPolicy string

const (
	// MatchAll requires all populated Query clauses to match.
	MatchAll MatchPolicy = "all"
	// MatchAny requires at least one populated Query clause to match.
	MatchAny MatchPolicy = "any"
)

// Query captures typed selection filters for Find.
type Query struct {
	NameContains         string
	OrganizationContains string
	EmailDomain          string
	NoteContains         string
	GroupIDsAny          []string
	IDs                  []string
	Match                MatchPolicy
}

// Page controls paginated find output.
type Page struct {
	Limit  int
	Cursor string
}

// SortField controls find ordering.
type SortField string

const (
	// SortByGivenName orders by given name then family name.
	SortByGivenName SortField = "given_name"
	// SortByFamilyName orders by family name then given name.
	SortByFamilyName SortField = "family_name"
)

// SortOrder controls ascending/descending order.
type SortOrder string

const (
	// SortOrderAsc sorts ascending.
	SortOrderAsc SortOrder = "asc"
	// SortOrderDesc sorts descending.
	SortOrderDesc SortOrder = "desc"
)

// Sort controls Find ordering.
type Sort struct {
	By    SortField
	Order SortOrder
}

// Meta is optional lightweight metadata aligned with FindOutput.Refs.
type Meta struct {
	Ref          Ref
	DisplayName  string
	Organization string
	ModifiedAt   time.Time
}

// FindInput is the selection primitive request.
type FindInput struct {
	Query       Query
	Page        Page
	Sort        Sort
	IncludeMeta bool
}

// FindOutput is the selection primitive response.
//
// NextCursor is empty when no more pages are available.
type FindOutput struct {
	Refs       []Ref
	Meta       []Meta
	NextCursor string
}

// Field selects logical data for Get hydration.
type Field string

const (
	// FieldNames requests core name fields.
	FieldNames Field = "names"
	// FieldOrganization requests organization/title fields.
	FieldOrganization Field = "organization"
	// FieldEmails requests email addresses.
	FieldEmails Field = "emails"
	// FieldPhones requests phone numbers.
	FieldPhones Field = "phones"
	// FieldNote requests contact notes.
	FieldNote Field = "note"
	// FieldGroups requests group memberships.
	FieldGroups Field = "groups"
)

// LabeledValue is a simple labeled string value (email/phone).
type LabeledValue struct {
	Label string
	Value string
}

// Item is the hydrated contact model.
type Item struct {
	Ref
	GivenName    string
	FamilyName   string
	MiddleName   string
	Nickname     string
	Organization string
	JobTitle     string
	Note         string
	Emails       []LabeledValue
	Phones       []LabeledValue
	GroupIDs     []string
	ModifiedAt   time.Time
}

// GetInput hydrates refs into typed contact items.
type GetInput struct {
	Refs   []Ref
	Fields []Field
}

// GetOutput contains hydrated contacts.
type GetOutput struct {
	Items []Item
}

// ContactDraft is the create model for Upsert.
type ContactDraft struct {
	ContainerID  string
	GivenName    string
	FamilyName   string
	MiddleName   string
	Nickname     string
	Organization string
	JobTitle     string
	Note         string
	Emails       []LabeledValue
	Phones       []LabeledValue
	GroupIDs     []string
}

// ContactChanges is a typed patch for Upsert existing contacts.
//
// Nil pointer fields mean "no change". Non-nil pointer fields set/replace the
// corresponding scalar field. Emails/Phones pointers allow explicit replacement
// including clearing by providing an empty slice pointer.
type ContactChanges struct {
	GivenName      *string
	FamilyName     *string
	MiddleName     *string
	Nickname       *string
	Organization   *string
	JobTitle       *string
	Note           *string
	Emails         *[]LabeledValue
	Phones         *[]LabeledValue
	AddGroupIDs    []string
	RemoveGroupIDs []string
}

// ContactPatch applies ContactChanges to a target Ref.
type ContactPatch struct {
	Ref     Ref
	Changes ContactChanges
}

// UpsertInput creates and patches contacts.
type UpsertInput struct {
	Create []ContactDraft
	Patch  []ContactPatch
}

// WriteResult reports per-item write status.
type WriteResult struct {
	Ref       Ref
	Succeeded bool
	Created   bool
	Updated   bool
	Err       error
}

// UpsertOutput reports per-create/per-patch results.
type UpsertOutput struct {
	Results []WriteResult
}

// MutationType enumerates explicit mutable operations.
type MutationType string

const (
	// MutationSetNote replaces the note field with MutationOp.Value.
	MutationSetNote MutationType = "set_note"
	// MutationSetOrganization replaces the organization field.
	MutationSetOrganization MutationType = "set_organization"
	// MutationSetJobTitle replaces the job title field.
	MutationSetJobTitle MutationType = "set_job_title"
	// MutationSetGivenName replaces the given name field.
	MutationSetGivenName MutationType = "set_given_name"
	// MutationSetFamilyName replaces the family name field.
	MutationSetFamilyName MutationType = "set_family_name"
	// MutationAddToGroup adds the contact to a group ID in Value.
	MutationAddToGroup MutationType = "add_to_group"
	// MutationRemoveFromGroup removes the contact from a group ID in Value.
	MutationRemoveFromGroup MutationType = "remove_from_group"
	// MutationDelete deletes the contact.
	MutationDelete MutationType = "delete"
)

// MutationOp is one explicit state transition.
type MutationOp struct {
	Type  MutationType
	Value string
}

// MutateInput applies ops to each target Ref.
type MutateInput struct {
	Refs []Ref
	Ops  []MutationOp
}

// MutateOutput reports per-ref mutation status.
type MutateOutput struct {
	Results []WriteResult
}

// Group stores discoverable group metadata.
type Group struct {
	GroupRef
	Name string
}

// GroupsAction selects a group operation.
type GroupsAction string

const (
	// GroupsActionList lists current groups.
	GroupsActionList GroupsAction = "list"
	// GroupsActionCreate creates a new group.
	GroupsActionCreate GroupsAction = "create"
	// GroupsActionRename renames an existing group.
	GroupsActionRename GroupsAction = "rename"
	// GroupsActionDelete deletes an existing group.
	GroupsActionDelete GroupsAction = "delete"
)

// GroupsInput is the request for Groups.
//
// For create, set Name and optional ContainerID.
// For rename, set Group.ID and Name.
// For delete, set Group.ID.
type GroupsInput struct {
	Action      GroupsAction
	Group       GroupRef
	Name        string
	ContainerID string
}

// GroupResult reports mutating group operation status.
type GroupResult struct {
	Group     GroupRef
	Succeeded bool
	Created   bool
	Updated   bool
	Err       error
}

// GroupsOutput contains current group catalog and mutating results.
type GroupsOutput struct {
	Groups  []Group
	Results []GroupResult
}

// AuthorizationStatus returns the current contacts permission status.
func AuthorizationStatus() (AuthStatus, error) {
	return authorizationStatus()
}

// RequestAccess requests contacts permission for the current process.
func RequestAccess() error {
	return requestAccess()
}

// Find selects contact refs using typed filters and pagination.
func Find(input FindInput) (FindOutput, error) {
	return find(input)
}

// Get hydrates refs into typed contacts.
func Get(input GetInput) (GetOutput, error) {
	return get(input)
}

// Upsert creates new contacts and patches existing contacts.
func Upsert(input UpsertInput) (UpsertOutput, error) {
	return upsert(input)
}

// Mutate applies explicit state transition ops to target refs.
func Mutate(input MutateInput) (MutateOutput, error) {
	return mutate(input)
}

// Groups lists and mutates contact groups.
func Groups(input GroupsInput) (GroupsOutput, error) {
	return groups(input)
}
