//go:build darwin

package contacts

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os/exec"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------
// Contact types
// ---------------------------------------------------------------------

// ContactType distinguishes person contacts from organization contacts.
type ContactType int

const (
	// ContactTypePerson is a person contact (default).
	ContactTypePerson ContactType = 0
	// ContactTypeOrganization is an organization contact.
	ContactTypeOrganization ContactType = 1
)

// LabeledValue pairs a label (e.g. "home", "work") with a value.
// The Identifier is assigned by the Contacts framework and is stable across
// fetches. It is empty for values that have not yet been persisted.
type LabeledValue[T any] struct {
	Identifier string
	Label      string
	Value      T
}

// PostalAddress holds a structured mailing address.
type PostalAddress struct {
	Street         string
	City           string
	State          string
	PostalCode     string
	Country        string
	ISOCountryCode string
}

// ContactRelation holds a related contact name.
type ContactRelation struct {
	Name string
}

// SocialProfile holds a social-network profile reference.
type SocialProfile struct {
	URLString string
	Username  string
	Service   string
}

// InstantMessage holds an instant-messaging handle.
type InstantMessage struct {
	Username string
	Service  string
}

// DateComponents holds a date without requiring a full time.Time.
// Month and Day are 1-based. Any field may be zero if not set.
type DateComponents struct {
	Year  int
	Month int
	Day   int
}

// Contact is the model for a macOS contact.
//
// A Contact value can represent either a unified projection (`Unified=true`) or
// a constituent record (`Unified=false`). For unified projections, LinkedIDs
// contains the linked constituent identifiers. For constituent records,
// LinkedIDs is empty.
//
// Fields are populated based on what was requested via the keys-to-fetch
// mechanism of the Contacts.framework. ContainerID identifies the owning
// container/account when available. Unset multi-value fields are nil (not empty
// slices).
type Contact struct {
	Identifier         string
	Unified            bool
	LinkedIDs          []string
	ContainerID        string
	ContactType        ContactType
	NamePrefix         string
	GivenName          string
	MiddleName         string
	FamilyName         string
	PreviousFamilyName string
	NameSuffix         string
	Nickname           string
	PhoneticGivenName  string
	PhoneticMiddleName string
	PhoneticFamilyName string
	OrganizationName   string
	DepartmentName     string
	JobTitle           string
	Note               string
	Birthday           *DateComponents
	PhoneNumbers       []LabeledValue[string]
	EmailAddresses     []LabeledValue[string]
	PostalAddresses    []LabeledValue[PostalAddress]
	URLAddresses       []LabeledValue[string]
	ContactRelations   []LabeledValue[ContactRelation]
	SocialProfiles     []LabeledValue[SocialProfile]
	InstantMessages    []LabeledValue[InstantMessage]
	Dates              []LabeledValue[DateComponents]
	ImageDataAvailable bool
	ImageData          []byte
	ThumbnailImageData []byte
}

// CreateContactInput specifies fields for a new contact.
//
// Only writable, non-zero/non-nil fields from Contact are set on the created
// contact. Read-only fields (Identifier, ImageDataAvailable, and
// ThumbnailImageData) are ignored. Contact.ContainerID selects the destination
// container; if empty, the default container is used.
type CreateContactInput struct {
	// Contact defines the contact values to persist.
	Contact Contact
}

// ContactField identifies a contact field that can be filtered.
type ContactField string

const (
	// ContactFieldGivenName matches the contact's givenName field.
	ContactFieldGivenName ContactField = "givenName"
	// ContactFieldFamilyName matches the contact's familyName field.
	ContactFieldFamilyName ContactField = "familyName"
	// ContactFieldMiddleName matches the contact's middleName field.
	ContactFieldMiddleName ContactField = "middleName"
	// ContactFieldOrganizationName matches the organizationName field.
	ContactFieldOrganizationName ContactField = "organizationName"
	// ContactFieldDepartmentName matches the departmentName field.
	ContactFieldDepartmentName ContactField = "departmentName"
	// ContactFieldJobTitle matches the jobTitle field.
	ContactFieldJobTitle ContactField = "jobTitle"
	// ContactFieldNickname matches the nickname field.
	ContactFieldNickname ContactField = "nickname"
	// ContactFieldNamePrefix matches the namePrefix field.
	ContactFieldNamePrefix ContactField = "namePrefix"
	// ContactFieldNameSuffix matches the nameSuffix field.
	ContactFieldNameSuffix ContactField = "nameSuffix"
	// ContactFieldEmailAddresses matches values in emailAddresses.
	ContactFieldEmailAddresses ContactField = "emailAddresses"
	// ContactFieldPhoneNumbers matches values in phoneNumbers.
	ContactFieldPhoneNumbers ContactField = "phoneNumbers"
	// ContactFieldUnified matches whether listing returns unified projections.
	// Value must be parseable as bool and operator must be FilterEquals.
	ContactFieldUnified ContactField = "unified"
	// ContactFieldContainerID matches the contact container identifier.
	// For unified listings, this matches if any linked constituent is in the
	// provided container.
	ContactFieldContainerID ContactField = "containerID"
)

// FilterOp specifies how a filter matches against a field value.
type FilterOp int

const (
	// FilterEquals matches when the field value equals the filter value
	// (case-insensitive).
	FilterEquals FilterOp = iota
	// FilterContains matches when the field value contains the filter value
	// (case-insensitive).
	FilterContains
	// FilterNotContains matches when the field value does not contain the
	// filter value (case-insensitive).
	FilterNotContains
)

// Filter specifies a single field-level filter for listing contacts.
type Filter struct {
	Field ContactField
	Value string
	Op    FilterOp
}

// ListContactsInput controls contact enumeration.
//
// Filters are ANDed together. Offset controls the starting position for
// pagination (0-based).
type ListContactsInput struct {
	Filters []Filter
	Offset  int
}

// ContactIdentity describes how an input identifier resolves in Contacts.
//
// CanonicalID is the unified canonical identifier. Unified reports whether the
// input identifier itself resolves to a unified projection (non-mutable).
// LinkedIDs are linked constituent record identifiers, and ContainerIDs are the
// corresponding constituent container identifiers.
type ContactIdentity struct {
	InputID      string
	CanonicalID  string
	Unified      bool
	LinkedIDs    []string
	ContainerIDs []string
}

// UpdateContactInput specifies mutable fields for updating a contact.
// Nil pointers mean "leave unchanged".
type UpdateContactInput struct {
	Identifier         string
	ContactType        *ContactType
	NamePrefix         *string
	GivenName          *string
	MiddleName         *string
	FamilyName         *string
	PreviousFamilyName *string
	NameSuffix         *string
	Nickname           *string
	PhoneticGivenName  *string
	PhoneticMiddleName *string
	PhoneticFamilyName *string
	OrganizationName   *string
	DepartmentName     *string
	JobTitle           *string
	Birthday           *DateComponents
	ClearBirthday      bool
	PhoneNumbers       *[]LabeledValue[string]
	EmailAddresses     *[]LabeledValue[string]
	PostalAddresses    *[]LabeledValue[PostalAddress]
	URLAddresses       *[]LabeledValue[string]
	ContactRelations   *[]LabeledValue[ContactRelation]
	SocialProfiles     *[]LabeledValue[SocialProfile]
	InstantMessages    *[]LabeledValue[InstantMessage]
	Dates              *[]LabeledValue[DateComponents]
	ImageData          *[]byte
}

// ---------------------------------------------------------------------
// Group types
// ---------------------------------------------------------------------

// Group represents a macOS contact group.
//
// ParentGroupID is non-empty when this group is a subgroup of another group.
// SubgroupIDs contains direct children when requested.
type Group struct {
	Identifier    string
	Name          string
	ContainerID   string
	ParentGroupID string
	SubgroupIDs   []string
}

// CreateGroupInput specifies parameters for creating a new group.
type CreateGroupInput struct {
	Name string
	// ContainerID is the container to add the group to.
	// If empty, the default container is used.
	ContainerID string
	// ParentGroupID, if non-empty, makes this group a subgroup of the
	// specified parent group.
	ParentGroupID string
}

// ListGroupsInput controls group enumeration.
type ListGroupsInput struct {
	ContainerID      string
	IncludeHierarchy bool
}

// UpdateGroupInput specifies mutable group fields.
// Nil pointers mean "leave unchanged".
type UpdateGroupInput struct {
	Identifier    string
	Name          *string
	ParentGroupID *string
}

// ---------------------------------------------------------------------
// Container types
// ---------------------------------------------------------------------

// ContainerType identifies the backing store type for a container.
type ContainerType int

const (
	// ContainerTypeUnassigned is an unknown container type.
	ContainerTypeUnassigned ContainerType = 0
	// ContainerTypeLocal is a local on-device container.
	ContainerTypeLocal ContainerType = 1
	// ContainerTypeExchange is an Exchange container.
	ContainerTypeExchange ContainerType = 2
	// ContainerTypeCardDAV is a CardDAV container (e.g. iCloud).
	ContainerTypeCardDAV ContainerType = 3
)

// Container represents a contacts container (account/store).
type Container struct {
	Identifier    string
	Name          string
	ContainerType ContainerType
}

// AuthorizationStatus reflects the app's authorization to access contacts.
type AuthorizationStatus int

const (
	AuthorizationStatusNotDetermined AuthorizationStatus = 0
	AuthorizationStatusRestricted    AuthorizationStatus = 1
	AuthorizationStatusDenied        AuthorizationStatus = 2
	AuthorizationStatusAuthorized    AuthorizationStatus = 3
)

// Typed package-level errors.
var (
	// ErrNotFound indicates the target entity does not exist.
	ErrNotFound = errors.New("contacts: not found")
	// ErrPermissionDenied indicates Contacts access was denied or restricted.
	ErrPermissionDenied = errors.New("contacts: permission denied")
	// ErrInvalidArgument indicates a caller-provided input was invalid.
	ErrInvalidArgument = errors.New("contacts: invalid argument")
	// ErrUnsupported indicates the requested operation is unsupported.
	ErrUnsupported = errors.New("contacts: unsupported")
	// ErrVerificationFailed indicates read-after-write verification failed.
	ErrVerificationFailed = errors.New("contacts: verification failed")
	// ErrUnifiedContactNotMutable indicates a mutation target is a unified ID.
	ErrUnifiedContactNotMutable = errors.New("contacts: unified contact not mutable")
	// ErrGroupContainerMismatch indicates contact/group container mismatch.
	ErrGroupContainerMismatch = errors.New("contacts: group container mismatch")
)

// OpError captures operation-level failures with typed causes.
type OpError struct {
	Op  string
	ID  string
	Err error
}

func (e *OpError) Error() string {
	if e == nil {
		return ""
	}
	if e.ID != "" {
		return fmt.Sprintf("contacts: %s (%s): %v", e.Op, e.ID, e.Err)
	}
	return fmt.Sprintf("contacts: %s: %v", e.Op, e.Err)
}

func (e *OpError) Unwrap() error { return e.Err }

func validContactField(field ContactField) bool {
	switch field {
	case ContactFieldGivenName,
		ContactFieldFamilyName,
		ContactFieldMiddleName,
		ContactFieldOrganizationName,
		ContactFieldDepartmentName,
		ContactFieldJobTitle,
		ContactFieldNickname,
		ContactFieldNamePrefix,
		ContactFieldNameSuffix,
		ContactFieldEmailAddresses,
		ContactFieldPhoneNumbers,
		ContactFieldUnified,
		ContactFieldContainerID:
		return true
	default:
		return false
	}
}

// ValidateFilters validates filter fields and operators.
func ValidateFilters(filters []Filter) error {
	var unifiedSeen bool
	var unifiedValue bool
	for i, f := range filters {
		if !validContactField(f.Field) {
			return fmt.Errorf("%w: filter[%d] field %q is unsupported", ErrInvalidArgument, i, f.Field)
		}
		if f.Op < FilterEquals || f.Op > FilterNotContains {
			return fmt.Errorf("%w: filter[%d] has invalid operator %d", ErrInvalidArgument, i, f.Op)
		}
		switch f.Field {
		case ContactFieldUnified:
			if f.Op != FilterEquals {
				return fmt.Errorf("%w: filter[%d] field %q only supports FilterEquals", ErrInvalidArgument, i, f.Field)
			}
			v, err := strconv.ParseBool(strings.TrimSpace(f.Value))
			if err != nil {
				return fmt.Errorf("%w: filter[%d] field %q requires bool value", ErrInvalidArgument, i, f.Field)
			}
			if unifiedSeen && unifiedValue != v {
				return fmt.Errorf("%w: conflicting %q filters", ErrInvalidArgument, f.Field)
			}
			unifiedSeen = true
			unifiedValue = v
		case ContactFieldContainerID:
			if f.Op != FilterEquals {
				return fmt.Errorf("%w: filter[%d] field %q only supports FilterEquals", ErrInvalidArgument, i, f.Field)
			}
		}
	}
	return nil
}

func classifyBridgeError(msg string) error {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return nil
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "not found"), strings.Contains(lower, "does not exist"):
		return fmt.Errorf("%w: %s", ErrNotFound, trimmed)
	case strings.Contains(lower, "denied"), strings.Contains(lower, "not authorized"), strings.Contains(lower, "authorization"):
		return fmt.Errorf("%w: %s", ErrPermissionDenied, trimmed)
	case strings.Contains(lower, "unified"), strings.Contains(lower, "not mutable"):
		return fmt.Errorf("%w: %s", ErrUnifiedContactNotMutable, trimmed)
	case strings.Contains(lower, "container mismatch"), strings.Contains(lower, "cross-container"):
		return fmt.Errorf("%w: %s", ErrGroupContainerMismatch, trimmed)
	case strings.Contains(lower, "unsupported"):
		return fmt.Errorf("%w: %s", ErrUnsupported, trimmed)
	case strings.Contains(lower, "invalid"), strings.Contains(lower, "required"):
		return fmt.Errorf("%w: %s", ErrInvalidArgument, trimmed)
	default:
		return errors.New(trimmed)
	}
}

func newBridgeOpError(op, id, message string) error {
	if strings.TrimSpace(message) == "" {
		return nil
	}
	return &OpError{Op: op, ID: id, Err: classifyBridgeError(message)}
}

func newInvalidArg(op, id, message string) error {
	return &OpError{Op: op, ID: id, Err: fmt.Errorf("%w: %s", ErrInvalidArgument, strings.TrimSpace(message))}
}

func newVerificationError(op, id, message string) error {
	return &OpError{Op: op, ID: id, Err: fmt.Errorf("%w: %s", ErrVerificationFailed, strings.TrimSpace(message))}
}

func cloneSlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

// String returns a human-readable representation of the authorization status.
func (s AuthorizationStatus) String() string {
	switch s {
	case AuthorizationStatusNotDetermined:
		return "not_determined"
	case AuthorizationStatusRestricted:
		return "restricted"
	case AuthorizationStatusDenied:
		return "denied"
	case AuthorizationStatusAuthorized:
		return "authorized"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// String returns a human-readable representation of the container type.
func (t ContainerType) String() string {
	switch t {
	case ContainerTypeUnassigned:
		return "unassigned"
	case ContainerTypeLocal:
		return "local"
	case ContainerTypeExchange:
		return "exchange"
	case ContainerTypeCardDAV:
		return "cardDAV"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// String returns a human-readable representation of the contact type.
func (t ContactType) String() string {
	switch t {
	case ContactTypePerson:
		return "person"
	case ContactTypeOrganization:
		return "organization"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// FullName returns a simple concatenation of name parts for display.
// For organization contacts, it returns OrganizationName if no given/family
// name is set.
func (c Contact) FullName() string {
	parts := make([]string, 0, 5)
	if c.NamePrefix != "" {
		parts = append(parts, c.NamePrefix)
	}
	if c.GivenName != "" {
		parts = append(parts, c.GivenName)
	}
	if c.MiddleName != "" {
		parts = append(parts, c.MiddleName)
	}
	if c.FamilyName != "" {
		parts = append(parts, c.FamilyName)
	}
	if c.NameSuffix != "" {
		parts = append(parts, c.NameSuffix)
	}
	if len(parts) == 0 && c.OrganizationName != "" {
		return c.OrganizationName
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

func mergeContactPatch(current Contact, input UpdateContactInput) Contact {
	merged := current
	if input.ContactType != nil {
		merged.ContactType = *input.ContactType
	}
	if input.NamePrefix != nil {
		merged.NamePrefix = *input.NamePrefix
	}
	if input.GivenName != nil {
		merged.GivenName = *input.GivenName
	}
	if input.MiddleName != nil {
		merged.MiddleName = *input.MiddleName
	}
	if input.FamilyName != nil {
		merged.FamilyName = *input.FamilyName
	}
	if input.PreviousFamilyName != nil {
		merged.PreviousFamilyName = *input.PreviousFamilyName
	}
	if input.NameSuffix != nil {
		merged.NameSuffix = *input.NameSuffix
	}
	if input.Nickname != nil {
		merged.Nickname = *input.Nickname
	}
	if input.PhoneticGivenName != nil {
		merged.PhoneticGivenName = *input.PhoneticGivenName
	}
	if input.PhoneticMiddleName != nil {
		merged.PhoneticMiddleName = *input.PhoneticMiddleName
	}
	if input.PhoneticFamilyName != nil {
		merged.PhoneticFamilyName = *input.PhoneticFamilyName
	}
	if input.OrganizationName != nil {
		merged.OrganizationName = *input.OrganizationName
	}
	if input.DepartmentName != nil {
		merged.DepartmentName = *input.DepartmentName
	}
	if input.JobTitle != nil {
		merged.JobTitle = *input.JobTitle
	}
	if input.Birthday != nil {
		birthday := *input.Birthday
		merged.Birthday = &birthday
	}
	if input.ClearBirthday {
		merged.Birthday = nil
	}
	if input.PhoneNumbers != nil {
		merged.PhoneNumbers = cloneSlice(*input.PhoneNumbers)
	}
	if input.EmailAddresses != nil {
		merged.EmailAddresses = cloneSlice(*input.EmailAddresses)
	}
	if input.PostalAddresses != nil {
		merged.PostalAddresses = cloneSlice(*input.PostalAddresses)
	}
	if input.URLAddresses != nil {
		merged.URLAddresses = cloneSlice(*input.URLAddresses)
	}
	if input.ContactRelations != nil {
		merged.ContactRelations = cloneSlice(*input.ContactRelations)
	}
	if input.SocialProfiles != nil {
		merged.SocialProfiles = cloneSlice(*input.SocialProfiles)
	}
	if input.InstantMessages != nil {
		merged.InstantMessages = cloneSlice(*input.InstantMessages)
	}
	if input.Dates != nil {
		merged.Dates = cloneSlice(*input.Dates)
	}
	if input.ImageData != nil {
		merged.ImageData = cloneSlice(*input.ImageData)
	}
	return merged
}

func hasUpdateContactChanges(input UpdateContactInput) bool {
	return input.ContactType != nil ||
		input.NamePrefix != nil ||
		input.GivenName != nil ||
		input.MiddleName != nil ||
		input.FamilyName != nil ||
		input.PreviousFamilyName != nil ||
		input.NameSuffix != nil ||
		input.Nickname != nil ||
		input.PhoneticGivenName != nil ||
		input.PhoneticMiddleName != nil ||
		input.PhoneticFamilyName != nil ||
		input.OrganizationName != nil ||
		input.DepartmentName != nil ||
		input.JobTitle != nil ||
		input.Birthday != nil ||
		input.ClearBirthday ||
		input.PhoneNumbers != nil ||
		input.EmailAddresses != nil ||
		input.PostalAddresses != nil ||
		input.URLAddresses != nil ||
		input.ContactRelations != nil ||
		input.SocialProfiles != nil ||
		input.InstantMessages != nil ||
		input.Dates != nil ||
		input.ImageData != nil
}

func verifyUpdatedContact(updated Contact, input UpdateContactInput) error {
	if input.ContactType != nil && updated.ContactType != *input.ContactType {
		return fmt.Errorf("contactType mismatch")
	}
	if input.NamePrefix != nil && updated.NamePrefix != *input.NamePrefix {
		return fmt.Errorf("namePrefix mismatch")
	}
	if input.GivenName != nil && updated.GivenName != *input.GivenName {
		return fmt.Errorf("givenName mismatch")
	}
	if input.MiddleName != nil && updated.MiddleName != *input.MiddleName {
		return fmt.Errorf("middleName mismatch")
	}
	if input.FamilyName != nil && updated.FamilyName != *input.FamilyName {
		return fmt.Errorf("familyName mismatch")
	}
	if input.PreviousFamilyName != nil && updated.PreviousFamilyName != *input.PreviousFamilyName {
		return fmt.Errorf("previousFamilyName mismatch")
	}
	if input.NameSuffix != nil && updated.NameSuffix != *input.NameSuffix {
		return fmt.Errorf("nameSuffix mismatch")
	}
	if input.Nickname != nil && updated.Nickname != *input.Nickname {
		return fmt.Errorf("nickname mismatch")
	}
	if input.PhoneticGivenName != nil && updated.PhoneticGivenName != *input.PhoneticGivenName {
		return fmt.Errorf("phoneticGivenName mismatch")
	}
	if input.PhoneticMiddleName != nil && updated.PhoneticMiddleName != *input.PhoneticMiddleName {
		return fmt.Errorf("phoneticMiddleName mismatch")
	}
	if input.PhoneticFamilyName != nil && updated.PhoneticFamilyName != *input.PhoneticFamilyName {
		return fmt.Errorf("phoneticFamilyName mismatch")
	}
	if input.OrganizationName != nil && updated.OrganizationName != *input.OrganizationName {
		return fmt.Errorf("organizationName mismatch")
	}
	if input.DepartmentName != nil && updated.DepartmentName != *input.DepartmentName {
		return fmt.Errorf("departmentName mismatch")
	}
	if input.JobTitle != nil && updated.JobTitle != *input.JobTitle {
		return fmt.Errorf("jobTitle mismatch")
	}
	if input.Birthday != nil {
		if updated.Birthday == nil || *updated.Birthday != *input.Birthday {
			return fmt.Errorf("birthday mismatch")
		}
	}
	if input.ClearBirthday && updated.Birthday != nil {
		return fmt.Errorf("birthday was not cleared")
	}
	if input.PhoneNumbers != nil && len(updated.PhoneNumbers) != len(*input.PhoneNumbers) {
		return fmt.Errorf("phoneNumbers length mismatch")
	}
	if input.EmailAddresses != nil && len(updated.EmailAddresses) != len(*input.EmailAddresses) {
		return fmt.Errorf("emailAddresses length mismatch")
	}
	if input.PostalAddresses != nil && len(updated.PostalAddresses) != len(*input.PostalAddresses) {
		return fmt.Errorf("postalAddresses length mismatch")
	}
	if input.URLAddresses != nil && len(updated.URLAddresses) != len(*input.URLAddresses) {
		return fmt.Errorf("urlAddresses length mismatch")
	}
	if input.ContactRelations != nil && len(updated.ContactRelations) != len(*input.ContactRelations) {
		return fmt.Errorf("contactRelations length mismatch")
	}
	if input.SocialProfiles != nil && len(updated.SocialProfiles) != len(*input.SocialProfiles) {
		return fmt.Errorf("socialProfiles length mismatch")
	}
	if input.InstantMessages != nil && len(updated.InstantMessages) != len(*input.InstantMessages) {
		return fmt.Errorf("instantMessages length mismatch")
	}
	if input.Dates != nil && len(updated.Dates) != len(*input.Dates) {
		return fmt.Errorf("dates length mismatch")
	}
	if input.ImageData != nil && len(updated.ImageData) != len(*input.ImageData) {
		return fmt.Errorf("imageData length mismatch")
	}
	return nil
}

func hasUpdateGroupChanges(input UpdateGroupInput) bool {
	return input.Name != nil || input.ParentGroupID != nil
}

func verifyUpdatedGroup(updated Group, input UpdateGroupInput) error {
	if input.Name != nil && updated.Name != *input.Name {
		return fmt.Errorf("name mismatch")
	}
	if input.ParentGroupID != nil && updated.ParentGroupID != *input.ParentGroupID {
		return fmt.Errorf("parentGroupID mismatch")
	}
	return nil
}

func containsContactID(contacts []Contact, contactID string) bool {
	for _, c := range contacts {
		if c.Identifier == contactID {
			return true
		}
	}
	return false
}

// CheckAuthorization returns the current authorization status for accessing
// contacts. This does not prompt the user.
func CheckAuthorization(_ context.Context) AuthorizationStatus {
	return AuthorizationStatus(checkAuthorizationStatus())
}

// RequestAuthorization requests access to contacts from the user.
func RequestAuthorization(ctx context.Context) (AuthorizationStatus, error) {
	if err := ctx.Err(); err != nil {
		return CheckAuthorization(ctx), err
	}
	status, errStr := requestAccess()
	if errStr != "" {
		return AuthorizationStatus(status), newBridgeOpError("RequestAuthorization", "", errStr)
	}
	return AuthorizationStatus(status), nil
}

// GetContact fetches a single contact as a unified projection.
func GetContact(ctx context.Context, identifier string) (Contact, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return Contact{}, newInvalidArg("GetContact", "", "identifier is required")
	}
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	c, errStr := getContact(identifier, true)
	if errStr != "" {
		return Contact{}, newBridgeOpError("GetContact", identifier, errStr)
	}
	return c, nil
}

// ResolveContactIdentity resolves identifier semantics without hydrating full
// contact fields.
func ResolveContactIdentity(ctx context.Context, identifier string) (ContactIdentity, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ContactIdentity{}, newInvalidArg("ResolveContactIdentity", "", "identifier is required")
	}
	if err := ctx.Err(); err != nil {
		return ContactIdentity{}, err
	}
	identity, errStr := resolveContactIdentity(identifier)
	if errStr != "" {
		return ContactIdentity{}, newBridgeOpError("ResolveContactIdentity", identifier, errStr)
	}
	return identity, nil
}

func ensureNonUnifiedContactIdentity(ctx context.Context, op, identifier string) (ContactIdentity, error) {
	identity, err := ResolveContactIdentity(ctx, identifier)
	if err != nil {
		return ContactIdentity{}, err
	}
	if identity.Unified {
		return ContactIdentity{}, &OpError{
			Op:  op,
			ID:  identifier,
			Err: fmt.Errorf("%w: identifier %q resolves to unified projection %q", ErrUnifiedContactNotMutable, identifier, identity.CanonicalID),
		}
	}
	return identity, nil
}

func hasContainerIntersection(containerIDs []string, containerID string) bool {
	for _, id := range containerIDs {
		if id == containerID {
			return true
		}
	}
	return false
}

func getConstituentContact(identifier string) (Contact, string) {
	return getContact(identifier, false)
}

// ListContacts returns an iterator over contacts matching the given filters.
// ContactFieldUnified controls unified vs non-unified projection mode.
func ListContacts(ctx context.Context, input ListContactsInput) iter.Seq2[Contact, error] {
	return func(yield func(Contact, error) bool) {
		if input.Offset < 0 {
			yield(Contact{}, newInvalidArg("ListContacts", "", "offset must be >= 0"))
			return
		}
		if err := ValidateFilters(input.Filters); err != nil {
			yield(Contact{}, &OpError{Op: "ListContacts", Err: err})
			return
		}
		if err := ctx.Err(); err != nil {
			yield(Contact{}, err)
			return
		}

		contacts, errStr := listContacts(input.Filters)
		if errStr != "" {
			yield(Contact{}, newBridgeOpError("ListContacts", "", errStr))
			return
		}

		skipped := 0
		for _, c := range contacts {
			if err := ctx.Err(); err != nil {
				yield(Contact{}, err)
				return
			}
			if skipped < input.Offset {
				skipped++
				continue
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

// CreateContact creates a new contact and returns the created record.
func CreateContact(ctx context.Context, input CreateContactInput) (Contact, error) {
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	identifier, errStr := createContact(input)
	if errStr != "" {
		return Contact{}, newBridgeOpError("CreateContact", "", errStr)
	}
	if identifier == "" {
		return Contact{}, newVerificationError("CreateContact", "", "bridge returned empty identifier")
	}
	created, err := GetContact(ctx, identifier)
	if err != nil {
		return Contact{}, err
	}
	return created, nil
}

// UpdateContact updates mutable contact fields and verifies persistence.
// Unified identifiers are rejected with ErrUnifiedContactNotMutable.
func UpdateContact(ctx context.Context, input UpdateContactInput) (Contact, error) {
	input.Identifier = strings.TrimSpace(input.Identifier)
	if input.Identifier == "" {
		return Contact{}, newInvalidArg("UpdateContact", "", "identifier is required")
	}
	if !hasUpdateContactChanges(input) {
		return Contact{}, newInvalidArg("UpdateContact", input.Identifier, "at least one field must be set")
	}
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	if _, err := ensureNonUnifiedContactIdentity(ctx, "UpdateContact", input.Identifier); err != nil {
		return Contact{}, err
	}

	current, errStr := getConstituentContact(input.Identifier)
	if errStr != "" {
		return Contact{}, newBridgeOpError("UpdateContact", input.Identifier, errStr)
	}
	merged := mergeContactPatch(current, input)
	merged.Identifier = input.Identifier
	merged.Unified = false
	merged.LinkedIDs = nil

	if errStr := updateContact(merged); errStr != "" {
		return Contact{}, newBridgeOpError("UpdateContact", input.Identifier, errStr)
	}
	updated, errStr := getConstituentContact(input.Identifier)
	if errStr != "" {
		return Contact{}, newBridgeOpError("UpdateContact", input.Identifier, errStr)
	}
	if err := verifyUpdatedContact(updated, input); err != nil {
		return Contact{}, newVerificationError("UpdateContact", input.Identifier, err.Error())
	}
	return updated, nil
}

// DeleteContact deletes the contact with the given identifier.
// Unified identifiers are rejected with ErrUnifiedContactNotMutable.
func DeleteContact(ctx context.Context, identifier string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return newInvalidArg("DeleteContact", "", "identifier is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := ensureNonUnifiedContactIdentity(ctx, "DeleteContact", identifier); err != nil {
		return err
	}
	if errStr := deleteContact(identifier); errStr != "" {
		return newBridgeOpError("DeleteContact", identifier, errStr)
	}
	_, errStr := getConstituentContact(identifier)
	if errStr == "" {
		return newVerificationError("DeleteContact", identifier, "contact still exists after delete")
	}
	lookupErr := newBridgeOpError("DeleteContact", identifier, errStr)
	if errors.Is(lookupErr, ErrNotFound) {
		return nil
	}
	return lookupErr
}

// GetGroup fetches a single group by identifier.
func GetGroup(ctx context.Context, identifier string) (Group, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return Group{}, newInvalidArg("GetGroup", "", "identifier is required")
	}
	if err := ctx.Err(); err != nil {
		return Group{}, err
	}
	groups, errStr := listGroups("", true)
	if errStr != "" {
		return Group{}, newBridgeOpError("GetGroup", identifier, errStr)
	}
	for _, g := range groups {
		if g.Identifier == identifier {
			return g, nil
		}
	}
	return Group{}, &OpError{Op: "GetGroup", ID: identifier, Err: fmt.Errorf("%w: group %q not found", ErrNotFound, identifier)}
}

// ListGroups returns groups optionally scoped to one container.
func ListGroups(ctx context.Context, input ListGroupsInput) ([]Group, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	groups, errStr := listGroups(strings.TrimSpace(input.ContainerID), input.IncludeHierarchy)
	if errStr != "" {
		return nil, newBridgeOpError("ListGroups", input.ContainerID, errStr)
	}
	return groups, nil
}

// ListSubgroups returns direct children of the specified parent group.
func ListSubgroups(ctx context.Context, parentGroupID string) ([]Group, error) {
	parentGroupID = strings.TrimSpace(parentGroupID)
	if parentGroupID == "" {
		return nil, newInvalidArg("ListSubgroups", "", "parentGroupID is required")
	}
	groups, err := ListGroups(ctx, ListGroupsInput{IncludeHierarchy: true})
	if err != nil {
		return nil, err
	}
	out := make([]Group, 0)
	for _, g := range groups {
		if g.ParentGroupID == parentGroupID {
			out = append(out, g)
		}
	}
	return out, nil
}

// CreateGroup creates a new group and verifies the resulting state.
func CreateGroup(ctx context.Context, input CreateGroupInput) (Group, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Group{}, newInvalidArg("CreateGroup", "", "group name is required")
	}
	if err := ctx.Err(); err != nil {
		return Group{}, err
	}
	identifier, errStr := createGroup(input)
	if errStr != "" {
		return Group{}, newBridgeOpError("CreateGroup", "", errStr)
	}
	if identifier == "" {
		return Group{}, newVerificationError("CreateGroup", "", "bridge returned empty identifier")
	}
	created, err := GetGroup(ctx, identifier)
	if err != nil {
		return Group{}, err
	}
	if input.ParentGroupID != "" && created.ParentGroupID != input.ParentGroupID {
		return Group{}, newVerificationError("CreateGroup", identifier, "parentGroupID was not persisted")
	}
	return created, nil
}

// UpdateGroup updates mutable group fields and verifies persistence.
func UpdateGroup(ctx context.Context, input UpdateGroupInput) (Group, error) {
	input.Identifier = strings.TrimSpace(input.Identifier)
	if input.Identifier == "" {
		return Group{}, newInvalidArg("UpdateGroup", "", "identifier is required")
	}
	if !hasUpdateGroupChanges(input) {
		return Group{}, newInvalidArg("UpdateGroup", input.Identifier, "at least one field must be set")
	}
	if input.ParentGroupID != nil && *input.ParentGroupID == input.Identifier {
		return Group{}, newInvalidArg("UpdateGroup", input.Identifier, "parentGroupID cannot equal identifier")
	}
	if err := ctx.Err(); err != nil {
		return Group{}, err
	}
	if errStr := updateGroup(input.Identifier, input.Name, input.ParentGroupID); errStr != "" {
		return Group{}, newBridgeOpError("UpdateGroup", input.Identifier, errStr)
	}
	updated, err := GetGroup(ctx, input.Identifier)
	if err != nil {
		return Group{}, err
	}
	if err := verifyUpdatedGroup(updated, input); err != nil {
		return Group{}, newVerificationError("UpdateGroup", input.Identifier, err.Error())
	}
	return updated, nil
}

// DeleteGroup deletes the group with the given identifier.
func DeleteGroup(ctx context.Context, identifier string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return newInvalidArg("DeleteGroup", "", "identifier is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if errStr := deleteGroup(identifier); errStr != "" {
		return newBridgeOpError("DeleteGroup", identifier, errStr)
	}
	_, err := GetGroup(ctx, identifier)
	if err == nil {
		return newVerificationError("DeleteGroup", identifier, "group still exists after delete")
	}
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// AddContactToGroup adds a contact to a group and verifies membership.
//
// Membership is record/container scoped. Unified identifiers are rejected with
// ErrUnifiedContactNotMutable.
func AddContactToGroup(ctx context.Context, contactID, groupID string) error {
	contactID = strings.TrimSpace(contactID)
	groupID = strings.TrimSpace(groupID)
	if contactID == "" || groupID == "" {
		return newInvalidArg("AddContactToGroup", "", "contactID and groupID are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	identity, err := ensureNonUnifiedContactIdentity(ctx, "AddContactToGroup", contactID)
	if err != nil {
		return err
	}
	group, err := GetGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if group.ContainerID != "" && len(identity.ContainerIDs) > 0 && !hasContainerIntersection(identity.ContainerIDs, group.ContainerID) {
		return &OpError{
			Op:  "AddContactToGroup",
			ID:  groupID,
			Err: fmt.Errorf("%w: contact containers %v do not include group container %q", ErrGroupContainerMismatch, identity.ContainerIDs, group.ContainerID),
		}
	}
	if errStr := addContactToGroup(contactID, groupID); errStr != "" {
		return newBridgeOpError("AddContactToGroup", groupID, errStr)
	}
	members, err := ListContactsInGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if !containsContactID(members, contactID) {
		return newVerificationError("AddContactToGroup", groupID, "contact is not a persisted member")
	}
	return nil
}

// RemoveContactFromGroup removes a contact from a group.
//
// Membership is record/container scoped. Unified identifiers are rejected with
// ErrUnifiedContactNotMutable.
//
// This uses osascript (AppleScript) to perform the removal because the
// Contacts.framework CNSaveRequest removeMember:fromGroup: method has a
// known bug on macOS 14.6+ / 15.x where the removal silently fails.
func RemoveContactFromGroup(ctx context.Context, contactID, groupID string) error {
	contactID = strings.TrimSpace(contactID)
	groupID = strings.TrimSpace(groupID)
	if contactID == "" || groupID == "" {
		return newInvalidArg("RemoveContactFromGroup", "", "contactID and groupID are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	identity, err := ensureNonUnifiedContactIdentity(ctx, "RemoveContactFromGroup", contactID)
	if err != nil {
		return err
	}
	group, err := GetGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if group.ContainerID != "" && len(identity.ContainerIDs) > 0 && !hasContainerIntersection(identity.ContainerIDs, group.ContainerID) {
		return &OpError{
			Op:  "RemoveContactFromGroup",
			ID:  groupID,
			Err: fmt.Errorf("%w: contact containers %v do not include group container %q", ErrGroupContainerMismatch, identity.ContainerIDs, group.ContainerID),
		}
	}
	if err := removeContactFromGroupViaOSAScript(ctx, contactID, groupID); err != nil {
		return &OpError{Op: "RemoveContactFromGroup", ID: groupID, Err: err}
	}
	members, err := ListContactsInGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if containsContactID(members, contactID) {
		return newVerificationError("RemoveContactFromGroup", groupID, "contact is still a member")
	}
	return nil
}

// removeContactFromGroupViaOSAScript uses osascript to remove a contact
// from a group, working around the CNSaveRequest removeMember:fromGroup: bug.
func removeContactFromGroupViaOSAScript(ctx context.Context, contactID, groupID string) error {
	if strings.Contains(contactID, `"`) || strings.Contains(groupID, `"`) {
		return fmt.Errorf("invalid identifier: contains quote")
	}

	script := fmt.Sprintf(`tell application "Contacts"
	set thePerson to first person whose id is "%s"
	set theGroup to first group whose id is "%s"
	remove thePerson from theGroup
	save
end tell`, contactID, groupID)

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript remove member failed: %s (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// GetContainer fetches a single container by identifier.
func GetContainer(ctx context.Context, identifier string) (Container, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return Container{}, newInvalidArg("GetContainer", "", "identifier is required")
	}
	if err := ctx.Err(); err != nil {
		return Container{}, err
	}
	c, errStr := getContainer(identifier)
	if errStr != "" {
		return Container{}, newBridgeOpError("GetContainer", identifier, errStr)
	}
	return c, nil
}

// ListContainers returns all available containers.
func ListContainers(ctx context.Context) ([]Container, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	containers, errStr := listContainers()
	if errStr != "" {
		return nil, newBridgeOpError("ListContainers", "", errStr)
	}
	return containers, nil
}

// DefaultContainerID returns the identifier of the default container.
func DefaultContainerID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id, errStr := defaultContainerID()
	if errStr != "" {
		return "", newBridgeOpError("DefaultContainerID", "", errStr)
	}
	if strings.TrimSpace(id) == "" {
		return "", newVerificationError("DefaultContainerID", "", "empty container identifier")
	}
	return id, nil
}

// ListContactsInGroup returns constituent contacts that are members of the
// specified group (Unified=false for all returned contacts).
func ListContactsInGroup(ctx context.Context, groupID string) ([]Contact, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, newInvalidArg("ListContactsInGroup", "", "groupID is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contacts, errStr := listContactsInGroup(groupID)
	if errStr != "" {
		return nil, newBridgeOpError("ListContactsInGroup", groupID, errStr)
	}
	return contacts, nil
}
