//go:build darwin

package contacts

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Contacts -framework Foundation
#include "bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"unsafe"
)

// --- cgo string helpers ---

func goString(cs C.BridgeString) string {
	if cs.str == nil || cs.len == 0 {
		return ""
	}
	return C.GoStringN(cs.str, cs.len)
}

func makeBridgeString(s string) C.BridgeString {
	if s == "" {
		return C.BridgeString{str: nil, len: 0}
	}
	cstr := C.CString(s) // cgo built-in: allocates C memory
	return C.BridgeString{
		str: cstr,
		len: C.int(len(s)),
	}
}

func freeBridgeString(cs C.BridgeString) {
	if cs.str != nil {
		C.free(unsafe.Pointer(cs.str))
	}
}

// --- Type conversions ---

func goContact(cc C.CContact) Contact {
	c := Contact{
		Identifier:         goString(cc.identifier),
		ContactType:        ContactType(cc.contactType),
		NamePrefix:         goString(cc.namePrefix),
		GivenName:          goString(cc.givenName),
		MiddleName:         goString(cc.middleName),
		FamilyName:         goString(cc.familyName),
		PreviousFamilyName: goString(cc.previousFamilyName),
		NameSuffix:         goString(cc.nameSuffix),
		Nickname:           goString(cc.nickname),
		PhoneticGivenName:  goString(cc.phoneticGivenName),
		PhoneticMiddleName: goString(cc.phoneticMiddleName),
		PhoneticFamilyName: goString(cc.phoneticFamilyName),
		OrganizationName:   goString(cc.organizationName),
		DepartmentName:     goString(cc.departmentName),
		JobTitle:           goString(cc.jobTitle),
		Note:               goString(cc.note),
		ImageDataAvailable: cc.imageDataAvailable != 0,
	}

	if cc.hasBirthday != 0 {
		c.Birthday = &DateComponents{
			Year:  int(cc.birthday.year),
			Month: int(cc.birthday.month),
			Day:   int(cc.birthday.day),
		}
	}

	if cc.phoneNumbersCount > 0 {
		c.PhoneNumbers = make([]LabeledValue[string], int(cc.phoneNumbersCount))
		phones := unsafe.Slice(cc.phoneNumbers, int(cc.phoneNumbersCount))
		for i, p := range phones {
			c.PhoneNumbers[i] = LabeledValue[string]{
				Identifier: goString(p.identifier),
				Label:      goString(p.label),
				Value:      goString(p.value),
			}
		}
	}

	if cc.emailAddressesCount > 0 {
		c.EmailAddresses = make([]LabeledValue[string], int(cc.emailAddressesCount))
		emails := unsafe.Slice(cc.emailAddresses, int(cc.emailAddressesCount))
		for i, e := range emails {
			c.EmailAddresses[i] = LabeledValue[string]{
				Identifier: goString(e.identifier),
				Label:      goString(e.label),
				Value:      goString(e.value),
			}
		}
	}

	if cc.postalAddressesCount > 0 {
		c.PostalAddresses = make([]LabeledValue[PostalAddress], int(cc.postalAddressesCount))
		addrs := unsafe.Slice(cc.postalAddresses, int(cc.postalAddressesCount))
		for i, a := range addrs {
			c.PostalAddresses[i] = LabeledValue[PostalAddress]{
				Identifier: goString(a.identifier),
				Label:      goString(a.label),
				Value: PostalAddress{
					Street:         goString(a.value.street),
					City:           goString(a.value.city),
					State:          goString(a.value.state),
					PostalCode:     goString(a.value.postalCode),
					Country:        goString(a.value.country),
					ISOCountryCode: goString(a.value.isoCountryCode),
				},
			}
		}
	}

	if cc.urlAddressesCount > 0 {
		c.URLAddresses = make([]LabeledValue[string], int(cc.urlAddressesCount))
		urls := unsafe.Slice(cc.urlAddresses, int(cc.urlAddressesCount))
		for i, u := range urls {
			c.URLAddresses[i] = LabeledValue[string]{
				Identifier: goString(u.identifier),
				Label:      goString(u.label),
				Value:      goString(u.value),
			}
		}
	}

	if cc.contactRelationsCount > 0 {
		c.ContactRelations = make([]LabeledValue[ContactRelation], int(cc.contactRelationsCount))
		rels := unsafe.Slice(cc.contactRelations, int(cc.contactRelationsCount))
		for i, r := range rels {
			c.ContactRelations[i] = LabeledValue[ContactRelation]{
				Identifier: goString(r.identifier),
				Label:      goString(r.label),
				Value:      ContactRelation{Name: goString(r.value.name)},
			}
		}
	}

	if cc.socialProfilesCount > 0 {
		c.SocialProfiles = make([]LabeledValue[SocialProfile], int(cc.socialProfilesCount))
		profiles := unsafe.Slice(cc.socialProfiles, int(cc.socialProfilesCount))
		for i, p := range profiles {
			c.SocialProfiles[i] = LabeledValue[SocialProfile]{
				Identifier: goString(p.identifier),
				Label:      goString(p.label),
				Value: SocialProfile{
					URLString: goString(p.value.urlString),
					Username:  goString(p.value.username),
					Service:   goString(p.value.service),
				},
			}
		}
	}

	if cc.instantMessagesCount > 0 {
		c.InstantMessages = make([]LabeledValue[InstantMessage], int(cc.instantMessagesCount))
		ims := unsafe.Slice(cc.instantMessages, int(cc.instantMessagesCount))
		for i, im := range ims {
			c.InstantMessages[i] = LabeledValue[InstantMessage]{
				Identifier: goString(im.identifier),
				Label:      goString(im.label),
				Value: InstantMessage{
					Username: goString(im.value.instantUsername),
					Service:  goString(im.value.instantService),
				},
			}
		}
	}

	if cc.datesCount > 0 {
		c.Dates = make([]LabeledValue[DateComponents], int(cc.datesCount))
		dates := unsafe.Slice(cc.dates, int(cc.datesCount))
		for i, d := range dates {
			c.Dates[i] = LabeledValue[DateComponents]{
				Identifier: goString(d.identifier),
				Label:      goString(d.label),
				Value: DateComponents{
					Year:  int(d.value.year),
					Month: int(d.value.month),
					Day:   int(d.value.day),
				},
			}
		}
	}

	if cc.imageData != nil && cc.imageDataLen > 0 {
		c.ImageData = C.GoBytes(cc.imageData, cc.imageDataLen)
	}
	if cc.thumbnailImageData != nil && cc.thumbnailImageDataLen > 0 {
		c.ThumbnailImageData = C.GoBytes(cc.thumbnailImageData, cc.thumbnailImageDataLen)
	}

	return c
}

func goGroup(cg C.CGroup) Group {
	g := Group{
		Identifier:    goString(cg.identifier),
		Name:          goString(cg.name),
		ContainerID:   goString(cg.containerID),
		ParentGroupID: goString(cg.parentGroupID),
	}
	if cg.subgroupIDsCount > 0 {
		g.SubgroupIDs = make([]string, int(cg.subgroupIDsCount))
		subs := unsafe.Slice(cg.subgroupIDs, int(cg.subgroupIDsCount))
		for i, s := range subs {
			g.SubgroupIDs[i] = goString(s)
		}
	}
	return g
}

func goContainer(cc C.CContainer) Container {
	return Container{
		Identifier:    goString(cc.identifier),
		Name:          goString(cc.name),
		ContainerType: ContainerType(cc.containerType),
	}
}

// --- Bridge function wrappers ---

func checkAuthorizationStatus() int {
	return int(C.bridge_check_authorization())
}

func requestAccess() (int, string) {
	result := C.bridge_request_access()
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	return int(result.status), errStr
}

func getContact(identifier string) (Contact, string) {
	cid := makeBridgeString(identifier)
	defer freeBridgeString(cid)

	result := C.bridge_get_contact(cid)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return Contact{}, errStr
	}

	c := goContact(result.contact)
	C.bridge_free_contact(&result.contact)
	return c, ""
}

func listContacts(filters []Filter) ([]Contact, string) {
	var cFilters *C.CFilter
	var cFilterPtrs []C.CFilter

	if len(filters) > 0 {
		cFilterPtrs = make([]C.CFilter, len(filters))
		for i, f := range filters {
			cFilterPtrs[i] = C.CFilter{
				fieldName: makeBridgeString(string(f.Field)),
				value:     makeBridgeString(f.Value),
				op:        C.int(f.Op),
			}
		}
		cFilters = &cFilterPtrs[0]
	}

	result := C.bridge_list_contacts(cFilters, C.int(len(filters)))

	// Free filter strings
	for _, cf := range cFilterPtrs {
		freeBridgeString(cf.fieldName)
		freeBridgeString(cf.value)
	}

	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return nil, errStr
	}

	contacts := make([]Contact, int(result.count))
	if result.count > 0 {
		cContacts := unsafe.Slice(result.contacts, int(result.count))
		for i, cc := range cContacts {
			contacts[i] = goContact(cc)
		}
		C.bridge_free_contact_list(result.contacts, result.count)
	}
	return contacts, ""
}

func createContact(input CreateContactInput) (string, string) {
	cc := buildCContact(input)
	defer freeCContactInput(&cc)
	containerID := makeBridgeString(input.ContainerID)
	defer freeBridgeString(containerID)

	result := C.bridge_create_contact(cc, containerID)

	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return "", errStr
	}

	id := goString(result.identifier)
	if result.identifier.str != nil {
		C.free(unsafe.Pointer(result.identifier.str))
	}
	return id, ""
}

func updateContact(input Contact) string {
	cc := buildCContactFromContact(input)
	defer freeCContactInput(&cc)

	result := C.bridge_update_contact(cc)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	return errStr
}

func buildCContactFromContact(input Contact) C.CContact {
	var cc C.CContact
	cc.identifier = makeBridgeString(input.Identifier)
	cc.contactType = C.int(input.ContactType)
	cc.namePrefix = makeBridgeString(input.NamePrefix)
	cc.givenName = makeBridgeString(input.GivenName)
	cc.middleName = makeBridgeString(input.MiddleName)
	cc.familyName = makeBridgeString(input.FamilyName)
	cc.previousFamilyName = makeBridgeString(input.PreviousFamilyName)
	cc.nameSuffix = makeBridgeString(input.NameSuffix)
	cc.nickname = makeBridgeString(input.Nickname)
	cc.phoneticGivenName = makeBridgeString(input.PhoneticGivenName)
	cc.phoneticMiddleName = makeBridgeString(input.PhoneticMiddleName)
	cc.phoneticFamilyName = makeBridgeString(input.PhoneticFamilyName)
	cc.organizationName = makeBridgeString(input.OrganizationName)
	cc.departmentName = makeBridgeString(input.DepartmentName)
	cc.jobTitle = makeBridgeString(input.JobTitle)

	if input.Birthday != nil {
		cc.hasBirthday = 1
		cc.birthday = C.CDateComponents{
			year:  C.int(input.Birthday.Year),
			month: C.int(input.Birthday.Month),
			day:   C.int(input.Birthday.Day),
		}
	}

	if len(input.PhoneNumbers) > 0 {
		phones := make([]C.CLabeledString, len(input.PhoneNumbers))
		for i, p := range input.PhoneNumbers {
			phones[i] = C.CLabeledString{
				identifier: makeBridgeString(p.Identifier),
				label:      makeBridgeString(p.Label),
				value:      makeBridgeString(p.Value),
			}
		}
		cc.phoneNumbers = &phones[0]
		cc.phoneNumbersCount = C.int(len(phones))
	}

	if len(input.EmailAddresses) > 0 {
		emails := make([]C.CLabeledString, len(input.EmailAddresses))
		for i, e := range input.EmailAddresses {
			emails[i] = C.CLabeledString{
				identifier: makeBridgeString(e.Identifier),
				label:      makeBridgeString(e.Label),
				value:      makeBridgeString(e.Value),
			}
		}
		cc.emailAddresses = &emails[0]
		cc.emailAddressesCount = C.int(len(emails))
	}

	if len(input.PostalAddresses) > 0 {
		addrs := make([]C.CLabeledPostalAddress, len(input.PostalAddresses))
		for i, a := range input.PostalAddresses {
			addrs[i] = C.CLabeledPostalAddress{
				identifier: makeBridgeString(a.Identifier),
				label:      makeBridgeString(a.Label),
				value: C.CPostalAddress{
					street:         makeBridgeString(a.Value.Street),
					city:           makeBridgeString(a.Value.City),
					state:          makeBridgeString(a.Value.State),
					postalCode:     makeBridgeString(a.Value.PostalCode),
					country:        makeBridgeString(a.Value.Country),
					isoCountryCode: makeBridgeString(a.Value.ISOCountryCode),
				},
			}
		}
		cc.postalAddresses = &addrs[0]
		cc.postalAddressesCount = C.int(len(addrs))
	}

	if len(input.URLAddresses) > 0 {
		urls := make([]C.CLabeledString, len(input.URLAddresses))
		for i, u := range input.URLAddresses {
			urls[i] = C.CLabeledString{
				identifier: makeBridgeString(u.Identifier),
				label:      makeBridgeString(u.Label),
				value:      makeBridgeString(u.Value),
			}
		}
		cc.urlAddresses = &urls[0]
		cc.urlAddressesCount = C.int(len(urls))
	}

	if len(input.ContactRelations) > 0 {
		rels := make([]C.CLabeledContactRelation, len(input.ContactRelations))
		for i, r := range input.ContactRelations {
			rels[i] = C.CLabeledContactRelation{
				identifier: makeBridgeString(r.Identifier),
				label:      makeBridgeString(r.Label),
				value:      C.CContactRelation{name: makeBridgeString(r.Value.Name)},
			}
		}
		cc.contactRelations = &rels[0]
		cc.contactRelationsCount = C.int(len(rels))
	}

	if len(input.SocialProfiles) > 0 {
		profiles := make([]C.CLabeledSocialProfile, len(input.SocialProfiles))
		for i, p := range input.SocialProfiles {
			profiles[i] = C.CLabeledSocialProfile{
				identifier: makeBridgeString(p.Identifier),
				label:      makeBridgeString(p.Label),
				value: C.CSocialProfile{
					urlString: makeBridgeString(p.Value.URLString),
					username:  makeBridgeString(p.Value.Username),
					service:   makeBridgeString(p.Value.Service),
				},
			}
		}
		cc.socialProfiles = &profiles[0]
		cc.socialProfilesCount = C.int(len(profiles))
	}

	if len(input.InstantMessages) > 0 {
		ims := make([]C.CLabeledInstantMessage, len(input.InstantMessages))
		for i, im := range input.InstantMessages {
			ims[i] = C.CLabeledInstantMessage{
				identifier: makeBridgeString(im.Identifier),
				label:      makeBridgeString(im.Label),
				value: C.CInstantMessage{
					instantUsername: makeBridgeString(im.Value.Username),
					instantService:  makeBridgeString(im.Value.Service),
				},
			}
		}
		cc.instantMessages = &ims[0]
		cc.instantMessagesCount = C.int(len(ims))
	}

	if len(input.Dates) > 0 {
		dates := make([]C.CLabeledDateComponents, len(input.Dates))
		for i, d := range input.Dates {
			dates[i] = C.CLabeledDateComponents{
				identifier: makeBridgeString(d.Identifier),
				label:      makeBridgeString(d.Label),
				value: C.CDateComponents{
					year:  C.int(d.Value.Year),
					month: C.int(d.Value.Month),
					day:   C.int(d.Value.Day),
				},
			}
		}
		cc.dates = &dates[0]
		cc.datesCount = C.int(len(dates))
	}

	if len(input.ImageData) > 0 {
		cc.imageData = C.CBytes(input.ImageData)
		cc.imageDataLen = C.int(len(input.ImageData))
	}

	return cc
}

func buildCContact(input CreateContactInput) C.CContact {
	var cc C.CContact
	cc.contactType = C.int(input.ContactType)
	cc.namePrefix = makeBridgeString(input.NamePrefix)
	cc.givenName = makeBridgeString(input.GivenName)
	cc.middleName = makeBridgeString(input.MiddleName)
	cc.familyName = makeBridgeString(input.FamilyName)
	cc.previousFamilyName = makeBridgeString(input.PreviousFamilyName)
	cc.nameSuffix = makeBridgeString(input.NameSuffix)
	cc.nickname = makeBridgeString(input.Nickname)
	cc.phoneticGivenName = makeBridgeString(input.PhoneticGivenName)
	cc.phoneticMiddleName = makeBridgeString(input.PhoneticMiddleName)
	cc.phoneticFamilyName = makeBridgeString(input.PhoneticFamilyName)
	cc.organizationName = makeBridgeString(input.OrganizationName)
	cc.departmentName = makeBridgeString(input.DepartmentName)
	cc.jobTitle = makeBridgeString(input.JobTitle)
	cc.note = makeBridgeString(input.Note)

	if input.Birthday != nil {
		cc.hasBirthday = 1
		cc.birthday = C.CDateComponents{
			year:  C.int(input.Birthday.Year),
			month: C.int(input.Birthday.Month),
			day:   C.int(input.Birthday.Day),
		}
	}

	if len(input.PhoneNumbers) > 0 {
		phones := make([]C.CLabeledString, len(input.PhoneNumbers))
		for i, p := range input.PhoneNumbers {
			phones[i] = C.CLabeledString{
				identifier: makeBridgeString(p.Identifier),
				label:      makeBridgeString(p.Label),
				value:      makeBridgeString(p.Value),
			}
		}
		cc.phoneNumbers = &phones[0]
		cc.phoneNumbersCount = C.int(len(phones))
	}

	if len(input.EmailAddresses) > 0 {
		emails := make([]C.CLabeledString, len(input.EmailAddresses))
		for i, e := range input.EmailAddresses {
			emails[i] = C.CLabeledString{
				identifier: makeBridgeString(e.Identifier),
				label:      makeBridgeString(e.Label),
				value:      makeBridgeString(e.Value),
			}
		}
		cc.emailAddresses = &emails[0]
		cc.emailAddressesCount = C.int(len(emails))
	}

	if len(input.PostalAddresses) > 0 {
		addrs := make([]C.CLabeledPostalAddress, len(input.PostalAddresses))
		for i, a := range input.PostalAddresses {
			addrs[i] = C.CLabeledPostalAddress{
				identifier: makeBridgeString(a.Identifier),
				label:      makeBridgeString(a.Label),
				value: C.CPostalAddress{
					street:         makeBridgeString(a.Value.Street),
					city:           makeBridgeString(a.Value.City),
					state:          makeBridgeString(a.Value.State),
					postalCode:     makeBridgeString(a.Value.PostalCode),
					country:        makeBridgeString(a.Value.Country),
					isoCountryCode: makeBridgeString(a.Value.ISOCountryCode),
				},
			}
		}
		cc.postalAddresses = &addrs[0]
		cc.postalAddressesCount = C.int(len(addrs))
	}

	if len(input.URLAddresses) > 0 {
		urls := make([]C.CLabeledString, len(input.URLAddresses))
		for i, u := range input.URLAddresses {
			urls[i] = C.CLabeledString{
				identifier: makeBridgeString(u.Identifier),
				label:      makeBridgeString(u.Label),
				value:      makeBridgeString(u.Value),
			}
		}
		cc.urlAddresses = &urls[0]
		cc.urlAddressesCount = C.int(len(urls))
	}

	if len(input.ContactRelations) > 0 {
		rels := make([]C.CLabeledContactRelation, len(input.ContactRelations))
		for i, r := range input.ContactRelations {
			rels[i] = C.CLabeledContactRelation{
				identifier: makeBridgeString(r.Identifier),
				label:      makeBridgeString(r.Label),
				value:      C.CContactRelation{name: makeBridgeString(r.Value.Name)},
			}
		}
		cc.contactRelations = &rels[0]
		cc.contactRelationsCount = C.int(len(rels))
	}

	if len(input.SocialProfiles) > 0 {
		profiles := make([]C.CLabeledSocialProfile, len(input.SocialProfiles))
		for i, p := range input.SocialProfiles {
			profiles[i] = C.CLabeledSocialProfile{
				identifier: makeBridgeString(p.Identifier),
				label:      makeBridgeString(p.Label),
				value: C.CSocialProfile{
					urlString: makeBridgeString(p.Value.URLString),
					username:  makeBridgeString(p.Value.Username),
					service:   makeBridgeString(p.Value.Service),
				},
			}
		}
		cc.socialProfiles = &profiles[0]
		cc.socialProfilesCount = C.int(len(profiles))
	}

	if len(input.InstantMessages) > 0 {
		ims := make([]C.CLabeledInstantMessage, len(input.InstantMessages))
		for i, im := range input.InstantMessages {
			ims[i] = C.CLabeledInstantMessage{
				identifier: makeBridgeString(im.Identifier),
				label:      makeBridgeString(im.Label),
				value: C.CInstantMessage{
					instantUsername: makeBridgeString(im.Value.Username),
					instantService:  makeBridgeString(im.Value.Service),
				},
			}
		}
		cc.instantMessages = &ims[0]
		cc.instantMessagesCount = C.int(len(ims))
	}

	if len(input.Dates) > 0 {
		dates := make([]C.CLabeledDateComponents, len(input.Dates))
		for i, d := range input.Dates {
			dates[i] = C.CLabeledDateComponents{
				identifier: makeBridgeString(d.Identifier),
				label:      makeBridgeString(d.Label),
				value: C.CDateComponents{
					year:  C.int(d.Value.Year),
					month: C.int(d.Value.Month),
					day:   C.int(d.Value.Day),
				},
			}
		}
		cc.dates = &dates[0]
		cc.datesCount = C.int(len(dates))
	}

	if len(input.ImageData) > 0 {
		cc.imageData = C.CBytes(input.ImageData)
		cc.imageDataLen = C.int(len(input.ImageData))
	}

	return cc
}

func freeCContactInput(cc *C.CContact) {
	freeBridgeString(cc.identifier)
	freeBridgeString(cc.namePrefix)
	freeBridgeString(cc.givenName)
	freeBridgeString(cc.middleName)
	freeBridgeString(cc.familyName)
	freeBridgeString(cc.previousFamilyName)
	freeBridgeString(cc.nameSuffix)
	freeBridgeString(cc.nickname)
	freeBridgeString(cc.phoneticGivenName)
	freeBridgeString(cc.phoneticMiddleName)
	freeBridgeString(cc.phoneticFamilyName)
	freeBridgeString(cc.organizationName)
	freeBridgeString(cc.departmentName)
	freeBridgeString(cc.jobTitle)
	freeBridgeString(cc.note)

	if cc.phoneNumbersCount > 0 && cc.phoneNumbers != nil {
		phones := unsafe.Slice(cc.phoneNumbers, int(cc.phoneNumbersCount))
		for _, p := range phones {
			freeBridgeString(p.identifier)
			freeBridgeString(p.label)
			freeBridgeString(p.value)
		}
	}

	if cc.emailAddressesCount > 0 && cc.emailAddresses != nil {
		emails := unsafe.Slice(cc.emailAddresses, int(cc.emailAddressesCount))
		for _, e := range emails {
			freeBridgeString(e.identifier)
			freeBridgeString(e.label)
			freeBridgeString(e.value)
		}
	}

	if cc.postalAddressesCount > 0 && cc.postalAddresses != nil {
		addrs := unsafe.Slice(cc.postalAddresses, int(cc.postalAddressesCount))
		for _, a := range addrs {
			freeBridgeString(a.identifier)
			freeBridgeString(a.label)
			freeBridgeString(a.value.street)
			freeBridgeString(a.value.city)
			freeBridgeString(a.value.state)
			freeBridgeString(a.value.postalCode)
			freeBridgeString(a.value.country)
			freeBridgeString(a.value.isoCountryCode)
		}
	}

	if cc.urlAddressesCount > 0 && cc.urlAddresses != nil {
		urls := unsafe.Slice(cc.urlAddresses, int(cc.urlAddressesCount))
		for _, u := range urls {
			freeBridgeString(u.identifier)
			freeBridgeString(u.label)
			freeBridgeString(u.value)
		}
	}

	if cc.contactRelationsCount > 0 && cc.contactRelations != nil {
		rels := unsafe.Slice(cc.contactRelations, int(cc.contactRelationsCount))
		for _, r := range rels {
			freeBridgeString(r.identifier)
			freeBridgeString(r.label)
			freeBridgeString(r.value.name)
		}
	}

	if cc.socialProfilesCount > 0 && cc.socialProfiles != nil {
		profiles := unsafe.Slice(cc.socialProfiles, int(cc.socialProfilesCount))
		for _, p := range profiles {
			freeBridgeString(p.identifier)
			freeBridgeString(p.label)
			freeBridgeString(p.value.urlString)
			freeBridgeString(p.value.username)
			freeBridgeString(p.value.service)
		}
	}

	if cc.instantMessagesCount > 0 && cc.instantMessages != nil {
		ims := unsafe.Slice(cc.instantMessages, int(cc.instantMessagesCount))
		for _, im := range ims {
			freeBridgeString(im.identifier)
			freeBridgeString(im.label)
			freeBridgeString(im.value.instantUsername)
			freeBridgeString(im.value.instantService)
		}
	}

	if cc.datesCount > 0 && cc.dates != nil {
		dates := unsafe.Slice(cc.dates, int(cc.datesCount))
		for _, d := range dates {
			freeBridgeString(d.identifier)
			freeBridgeString(d.label)
		}
	}

	if cc.imageData != nil {
		C.free(cc.imageData)
	}
}

func deleteContact(identifier string) string {
	cid := makeBridgeString(identifier)
	defer freeBridgeString(cid)

	result := C.bridge_delete_contact(cid)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	return errStr
}

func listGroups(containerID string, includeHierarchy bool) ([]Group, string) {
	cid := makeBridgeString(containerID)
	defer freeBridgeString(cid)

	include := C.int(0)
	if includeHierarchy {
		include = 1
	}
	result := C.bridge_list_groups(cid, include)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return nil, errStr
	}

	groups := make([]Group, int(result.count))
	if result.count > 0 {
		cGroups := unsafe.Slice(result.groups, int(result.count))
		for i, cg := range cGroups {
			groups[i] = goGroup(cg)
		}
		C.bridge_free_group_list(result.groups, result.count)
	}
	return groups, ""
}

func createGroup(input CreateGroupInput) (string, string) {
	cname := makeBridgeString(input.Name)
	defer freeBridgeString(cname)
	ccid := makeBridgeString(input.ContainerID)
	defer freeBridgeString(ccid)
	cpid := makeBridgeString(input.ParentGroupID)
	defer freeBridgeString(cpid)

	result := C.bridge_create_group(cname, ccid, cpid)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return "", errStr
	}

	id := goString(result.identifier)
	if result.identifier.str != nil {
		C.free(unsafe.Pointer(result.identifier.str))
	}
	return id, ""
}

func updateGroup(identifier string, name *string, parentGroupID *string) string {
	cid := makeBridgeString(identifier)
	defer freeBridgeString(cid)

	hasName := C.int(0)
	cname := C.BridgeString{str: nil, len: 0}
	if name != nil {
		hasName = 1
		cname = makeBridgeString(*name)
		defer freeBridgeString(cname)
	}

	hasParent := C.int(0)
	cpid := C.BridgeString{str: nil, len: 0}
	if parentGroupID != nil {
		hasParent = 1
		cpid = makeBridgeString(*parentGroupID)
		defer freeBridgeString(cpid)
	}

	result := C.bridge_update_group(cid, cname, hasName, cpid, hasParent)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	return errStr
}

func deleteGroup(identifier string) string {
	cid := makeBridgeString(identifier)
	defer freeBridgeString(cid)

	result := C.bridge_delete_group(cid)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	return errStr
}

func addContactToGroup(contactID, groupID string) string {
	ccid := makeBridgeString(contactID)
	defer freeBridgeString(ccid)
	cgid := makeBridgeString(groupID)
	defer freeBridgeString(cgid)

	result := C.bridge_add_contact_to_group(ccid, cgid)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	return errStr
}

func getContainer(identifier string) (Container, string) {
	cid := makeBridgeString(identifier)
	defer freeBridgeString(cid)

	result := C.bridge_get_container(cid)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return Container{}, errStr
	}

	c := goContainer(result.container)
	if result.container.identifier.str != nil {
		C.free(unsafe.Pointer(result.container.identifier.str))
	}
	if result.container.name.str != nil {
		C.free(unsafe.Pointer(result.container.name.str))
	}
	return c, ""
}

func listContainers() ([]Container, string) {
	result := C.bridge_list_containers()
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return nil, errStr
	}

	containers := make([]Container, int(result.count))
	if result.count > 0 {
		cContainers := unsafe.Slice(result.containers, int(result.count))
		for i, cc := range cContainers {
			containers[i] = goContainer(cc)
		}
		C.bridge_free_container_list(result.containers, result.count)
	}
	return containers, ""
}

func defaultContainerID() (string, string) {
	result := C.bridge_default_container_id()
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return "", errStr
	}

	id := goString(result.identifier)
	if result.identifier.str != nil {
		C.free(unsafe.Pointer(result.identifier.str))
	}
	return id, ""
}

func listContactsInGroup(groupID string) ([]Contact, string) {
	cgid := makeBridgeString(groupID)
	defer freeBridgeString(cgid)

	result := C.bridge_list_contacts_in_group(cgid)
	errStr := goString(result.error)
	if result.error.str != nil {
		C.free(unsafe.Pointer(result.error.str))
	}
	if errStr != "" {
		return nil, errStr
	}

	contacts := make([]Contact, int(result.count))
	if result.count > 0 {
		cContacts := unsafe.Slice(result.contacts, int(result.count))
		for i, cc := range cContacts {
			contacts[i] = goContact(cc)
		}
		C.bridge_free_contact_list(result.contacts, result.count)
	}
	return contacts, ""
}
