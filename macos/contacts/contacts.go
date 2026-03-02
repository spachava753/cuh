//go:build darwin

package contacts

import (
	"context"
	"fmt"
	"iter"
	"os/exec"
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
	Street     string
	City       string
	State      string
	PostalCode string
	Country    string
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

// Contact is the read model for a macOS contact.
//
// Fields are populated based on what was requested via the keys-to-fetch
// mechanism of the Contacts.framework. Unset multi-value fields are nil
// (not empty slices).
type Contact struct {
	Identifier        string
	ContactType       ContactType
	NamePrefix        string
	GivenName         string
	MiddleName        string
	FamilyName        string
	PreviousFamilyName string
	NameSuffix        string
	Nickname          string
	PhoneticGivenName string
	PhoneticMiddleName string
	PhoneticFamilyName string
	OrganizationName  string
	DepartmentName    string
	JobTitle          string
	Note              string
	Birthday          *DateComponents
	PhoneNumbers      []LabeledValue[string]
	EmailAddresses    []LabeledValue[string]
	PostalAddresses   []LabeledValue[PostalAddress]
	URLAddresses      []LabeledValue[string]
	ContactRelations  []LabeledValue[ContactRelation]
	SocialProfiles    []LabeledValue[SocialProfile]
	InstantMessages   []LabeledValue[InstantMessage]
	Dates             []LabeledValue[DateComponents]
	ImageDataAvailable bool
	ImageData          []byte
	ThumbnailImageData []byte
}

// CreateContactInput specifies fields for a new contact.
//
// Only non-zero/non-nil fields are set on the created contact. The Contacts
// framework assigns the identifier.
type CreateContactInput struct {
	ContactType       ContactType
	NamePrefix        string
	GivenName         string
	MiddleName        string
	FamilyName        string
	PreviousFamilyName string
	NameSuffix        string
	Nickname          string
	PhoneticGivenName string
	PhoneticMiddleName string
	PhoneticFamilyName string
	OrganizationName  string
	DepartmentName    string
	JobTitle          string
	Note              string
	Birthday          *DateComponents
	PhoneNumbers      []LabeledValue[string]
	EmailAddresses    []LabeledValue[string]
	PostalAddresses   []LabeledValue[PostalAddress]
	URLAddresses      []LabeledValue[string]
	ContactRelations  []LabeledValue[ContactRelation]
	SocialProfiles    []LabeledValue[SocialProfile]
	InstantMessages   []LabeledValue[InstantMessage]
	Dates             []LabeledValue[DateComponents]
	ImageData         []byte
	// ContainerID is the container to add the contact to.
	// If empty, the default container is used.
	ContainerID string
}

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
//
// FieldName should be one of the contact field names as used by the Contacts
// framework key constants (e.g. "givenName", "familyName", "emailAddresses",
// "phoneNumbers", "organizationName").
type Filter struct {
	FieldName string
	Value     string
	Op        FilterOp
}

// ListContactsInput controls contact enumeration.
//
// Filters are ANDed together. Offset controls the starting position for
// pagination (0-based).
type ListContactsInput struct {
	Filters []Filter
	Offset  int
}

// ---------------------------------------------------------------------
// Group types
// ---------------------------------------------------------------------

// Group represents a macOS contact group.
//
// ParentGroupID is non-empty when this group is a subgroup of another group.
// It is discovered by enumerating group membership in containers.
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
	AuthorizationStatusRestricted   AuthorizationStatus = 1
	AuthorizationStatusDenied       AuthorizationStatus = 2
	AuthorizationStatusAuthorized   AuthorizationStatus = 3
)

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

// Now stub out the function signatures. The actual implementations delegate
// to the cgo bridge in bridge.go.

// CheckAuthorization returns the current authorization status for accessing
// contacts. This does not prompt the user.
func CheckAuthorization(_ context.Context) AuthorizationStatus {
	return AuthorizationStatus(checkAuthorizationStatus())
}

// RequestAuthorization requests access to contacts from the user.
// Returns the resulting authorization status and any error.
//
// On macOS 15+, this may prompt the user for limited vs full access.
func RequestAuthorization(ctx context.Context) (AuthorizationStatus, error) {
	status, errStr := requestAccess()
	if errStr != "" {
		return AuthorizationStatus(status), fmt.Errorf("contacts: authorization request failed: %s", errStr)
	}
	return AuthorizationStatus(status), nil
}

// GetContact fetches a single contact by identifier.
// Returns all available contact fields.
func GetContact(ctx context.Context, identifier string) (Contact, error) {
	if identifier == "" {
		return Contact{}, fmt.Errorf("contacts: identifier is required")
	}
	c, errStr := getContact(identifier)
	if errStr != "" {
		return Contact{}, fmt.Errorf("contacts: get contact failed: %s", errStr)
	}
	return c, nil
}

// ListContacts returns an iterator over contacts matching the given filters.
//
// The iterator yields contacts one at a time and stops early if the context
// is cancelled. Errors during enumeration are yielded as the error value and
// the iterator stops.
//
// Example:
//
//	for contact, err := range contacts.ListContacts(ctx, contacts.ListContactsInput{
//		Filters: []contacts.Filter{
//			{FieldName: "givenName", Value: "John", Op: contacts.FilterContains},
//		},
//	}) {
//		if err != nil { /* handle */ }
//		fmt.Println(contact.GivenName, contact.FamilyName)
//	}
func ListContacts(ctx context.Context, input ListContactsInput) iter.Seq2[Contact, error] {
	return func(yield func(Contact, error) bool) {
		contacts, errStr := listContacts(input.Filters)
		if errStr != "" {
			yield(Contact{}, fmt.Errorf("contacts: list contacts failed: %s", errStr))
			return
		}
		skipped := 0
		for _, c := range contacts {
			if ctx.Err() != nil {
				yield(Contact{}, ctx.Err())
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

// CreateContact creates a new contact and returns the created contact with
// its assigned identifier.
func CreateContact(ctx context.Context, input CreateContactInput) (Contact, error) {
	identifier, errStr := createContact(input)
	if errStr != "" {
		return Contact{}, fmt.Errorf("contacts: create contact failed: %s", errStr)
	}
	return GetContact(ctx, identifier)
}

// DeleteContact deletes the contact with the given identifier.
func DeleteContact(ctx context.Context, identifier string) error {
	if identifier == "" {
		return fmt.Errorf("contacts: identifier is required")
	}
	if errStr := deleteContact(identifier); errStr != "" {
		return fmt.Errorf("contacts: delete contact failed: %s", errStr)
	}
	return nil
}

// GetGroup fetches a single group by identifier, including its parent and
// subgroup relationships.
func GetGroup(ctx context.Context, identifier string) (Group, error) {
	if identifier == "" {
		return Group{}, fmt.Errorf("contacts: identifier is required")
	}
	groups, errStr := listGroups("")
	if errStr != "" {
		return Group{}, fmt.Errorf("contacts: get group failed: %s", errStr)
	}
	for _, g := range groups {
		if g.Identifier == identifier {
			return g, nil
		}
	}
	return Group{}, fmt.Errorf("contacts: group %q not found", identifier)
}

// ListGroups returns all groups, optionally filtered to a specific container.
// Each group includes its parent/child relationships.
//
// Pass containerID="" to list groups from all containers.
func ListGroups(ctx context.Context, containerID string) ([]Group, error) {
	groups, errStr := listGroups(containerID)
	if errStr != "" {
		return nil, fmt.Errorf("contacts: list groups failed: %s", errStr)
	}
	return groups, nil
}

// CreateGroup creates a new group and returns it with its assigned identifier.
func CreateGroup(ctx context.Context, input CreateGroupInput) (Group, error) {
	if input.Name == "" {
		return Group{}, fmt.Errorf("contacts: group name is required")
	}
	identifier, errStr := createGroup(input)
	if errStr != "" {
		return Group{}, fmt.Errorf("contacts: create group failed: %s", errStr)
	}
	return GetGroup(ctx, identifier)
}

// DeleteGroup deletes the group with the given identifier.
// The Contacts framework determines whether a group with contacts can be
// deleted (contacts themselves are not deleted).
func DeleteGroup(ctx context.Context, identifier string) error {
	if identifier == "" {
		return fmt.Errorf("contacts: identifier is required")
	}
	if errStr := deleteGroup(identifier); errStr != "" {
		return fmt.Errorf("contacts: delete group failed: %s", errStr)
	}
	return nil
}

// AddContactToGroup adds a contact to a group.
func AddContactToGroup(ctx context.Context, contactID, groupID string) error {
	if contactID == "" || groupID == "" {
		return fmt.Errorf("contacts: contactID and groupID are required")
	}
	if errStr := addContactToGroup(contactID, groupID); errStr != "" {
		return fmt.Errorf("contacts: add contact to group failed: %s", errStr)
	}
	return nil
}

// RemoveContactFromGroup removes a contact from a group.
//
// This uses osascript (AppleScript) to perform the removal because the
// Contacts.framework CNSaveRequest removeMember:fromGroup: method has a
// known bug on macOS 14.6+ / 15.x where the removal silently fails.
// The AppleScript "remove <person> from <group>" command works reliably.
func RemoveContactFromGroup(ctx context.Context, contactID, groupID string) error {
	if contactID == "" || groupID == "" {
		return fmt.Errorf("contacts: contactID and groupID are required")
	}
	return removeContactFromGroupViaOSAScript(ctx, contactID, groupID)
}

// removeContactFromGroupViaOSAScript uses osascript to remove a contact
// from a group, working around the CNSaveRequest removeMember:fromGroup: bug.
func removeContactFromGroupViaOSAScript(ctx context.Context, contactID, groupID string) error {
	// Sanitize identifiers to prevent AppleScript injection.
	// Contact/group IDs are UUID:type strings (e.g. "ABC-123:ABPerson").
	// We reject any identifier containing a double-quote character.
	if strings.Contains(contactID, `"`) || strings.Contains(groupID, `"`) {
		return fmt.Errorf("contacts: invalid identifier (contains quote)")
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
		return fmt.Errorf("contacts: osascript remove member failed: %s (output: %s)",
			err, strings.TrimSpace(string(out)))
	}
	return nil
}

// GetContainer fetches a single container by identifier.
func GetContainer(ctx context.Context, identifier string) (Container, error) {
	if identifier == "" {
		return Container{}, fmt.Errorf("contacts: identifier is required")
	}
	c, errStr := getContainer(identifier)
	if errStr != "" {
		return Container{}, fmt.Errorf("contacts: get container failed: %s", errStr)
	}
	return c, nil
}

// ListContainers returns all available containers.
func ListContainers(ctx context.Context) ([]Container, error) {
	containers, errStr := listContainers()
	if errStr != "" {
		return nil, fmt.Errorf("contacts: list containers failed: %s", errStr)
	}
	return containers, nil
}

// DefaultContainerID returns the identifier of the default container.
func DefaultContainerID(ctx context.Context) (string, error) {
	id, errStr := defaultContainerID()
	if errStr != "" {
		return "", fmt.Errorf("contacts: default container failed: %s", errStr)
	}
	return id, nil
}

// ListContactsInGroup returns contacts that are members of the specified group.
func ListContactsInGroup(ctx context.Context, groupID string) ([]Contact, error) {
	if groupID == "" {
		return nil, fmt.Errorf("contacts: groupID is required")
	}
	contacts, errStr := listContactsInGroup(groupID)
	if errStr != "" {
		return nil, fmt.Errorf("contacts: list contacts in group failed: %s", errStr)
	}
	return contacts, nil
}

