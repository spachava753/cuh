#import <Contacts/Contacts.h>
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>
#include "bridge.h"

// --- Helpers ---

static BridgeString cstring_from_nsstring(NSString *str) {
    BridgeString cs;
    if (str == nil) {
        cs.str = NULL;
        cs.len = 0;
        return cs;
    }
    const char *utf8 = [str UTF8String];
    int len = (int)strlen(utf8);
    char *copy = (char *)malloc(len + 1);
    memcpy(copy, utf8, len + 1);
    cs.str = copy;
    cs.len = len;
    return cs;
}

static BridgeString cstring_empty(void) {
    BridgeString cs;
    cs.str = NULL;
    cs.len = 0;
    return cs;
}

static NSString *nsstring_from_cstring(BridgeString cs) {
    if (cs.str == NULL || cs.len == 0) {
        return @"";
    }
    return [[NSString alloc] initWithBytes:cs.str length:cs.len encoding:NSUTF8StringEncoding];
}

static BridgeString cstring_from_error(NSError *error) {
    if (error == nil) {
        return cstring_empty();
    }
    return cstring_from_nsstring([error localizedDescription]);
}

static void free_cstring(BridgeString *cs) {
    if (cs->str != NULL) {
        free((void *)cs->str);
        cs->str = NULL;
        cs->len = 0;
    }
}

// --- Convert CNContact to CContact ---

static CLabeledString convert_labeled_string(CNLabeledValue<NSString *> *lv) {
    CLabeledString cls;
    cls.identifier = cstring_from_nsstring(lv.identifier);
    cls.label = cstring_from_nsstring(lv.label ? [CNLabeledValue localizedStringForLabel:lv.label] : nil);
    cls.value = cstring_from_nsstring(lv.value);
    return cls;
}

static CLabeledPostalAddress convert_labeled_postal(CNLabeledValue<CNPostalAddress *> *lv) {
    CLabeledPostalAddress cla;
    cla.identifier = cstring_from_nsstring(lv.identifier);
    cla.label = cstring_from_nsstring(lv.label ? [CNLabeledValue localizedStringForLabel:lv.label] : nil);
    CNPostalAddress *addr = lv.value;
    cla.value.street = cstring_from_nsstring(addr.street);
    cla.value.city = cstring_from_nsstring(addr.city);
    cla.value.state = cstring_from_nsstring(addr.state);
    cla.value.postalCode = cstring_from_nsstring(addr.postalCode);
    cla.value.country = cstring_from_nsstring(addr.country);
    cla.value.isoCountryCode = cstring_from_nsstring(addr.ISOCountryCode);
    return cla;
}

static CLabeledContactRelation convert_labeled_relation(CNLabeledValue<CNContactRelation *> *lv) {
    CLabeledContactRelation clr;
    clr.identifier = cstring_from_nsstring(lv.identifier);
    clr.label = cstring_from_nsstring(lv.label ? [CNLabeledValue localizedStringForLabel:lv.label] : nil);
    clr.value.name = cstring_from_nsstring(lv.value.name);
    return clr;
}

static CLabeledSocialProfile convert_labeled_social(CNLabeledValue<CNSocialProfile *> *lv) {
    CLabeledSocialProfile cls;
    cls.identifier = cstring_from_nsstring(lv.identifier);
    cls.label = cstring_from_nsstring(lv.label ? [CNLabeledValue localizedStringForLabel:lv.label] : nil);
    CNSocialProfile *sp = lv.value;
    cls.value.urlString = cstring_from_nsstring(sp.urlString);
    cls.value.username = cstring_from_nsstring(sp.username);
    cls.value.service = cstring_from_nsstring(sp.service);
    return cls;
}

static CLabeledInstantMessage convert_labeled_im(CNLabeledValue<CNInstantMessageAddress *> *lv) {
    CLabeledInstantMessage cli;
    cli.identifier = cstring_from_nsstring(lv.identifier);
    cli.label = cstring_from_nsstring(lv.label ? [CNLabeledValue localizedStringForLabel:lv.label] : nil);
    CNInstantMessageAddress *im = lv.value;
    cli.value.instantUsername = cstring_from_nsstring(im.username);
    cli.value.instantService = cstring_from_nsstring(im.service);
    return cli;
}

static CDateComponents convert_date_components(NSDateComponents *dc) {
    CDateComponents cdc;
    cdc.year = (dc.year != NSDateComponentUndefined) ? (int)dc.year : 0;
    cdc.month = (dc.month != NSDateComponentUndefined) ? (int)dc.month : 0;
    cdc.day = (dc.day != NSDateComponentUndefined) ? (int)dc.day : 0;
    return cdc;
}

static CLabeledDateComponents convert_labeled_date(CNLabeledValue<NSDateComponents *> *lv) {
    CLabeledDateComponents cld;
    cld.identifier = cstring_from_nsstring(lv.identifier);
    cld.label = cstring_from_nsstring(lv.label ? [CNLabeledValue localizedStringForLabel:lv.label] : nil);
    cld.value = convert_date_components(lv.value);
    return cld;
}

static NSArray<id<CNKeyDescriptor>> *allContactKeys(void) {
    return @[
        CNContactIdentifierKey,
        CNContactTypeKey,
        CNContactNamePrefixKey,
        CNContactGivenNameKey,
        CNContactMiddleNameKey,
        CNContactFamilyNameKey,
        CNContactPreviousFamilyNameKey,
        CNContactNameSuffixKey,
        CNContactNicknameKey,
        CNContactPhoneticGivenNameKey,
        CNContactPhoneticMiddleNameKey,
        CNContactPhoneticFamilyNameKey,
        CNContactOrganizationNameKey,
        CNContactDepartmentNameKey,
        CNContactJobTitleKey,
        CNContactBirthdayKey,
        CNContactPhoneNumbersKey,
        CNContactEmailAddressesKey,
        CNContactPostalAddressesKey,
        CNContactUrlAddressesKey,
        CNContactRelationsKey,
        CNContactSocialProfilesKey,
        CNContactInstantMessageAddressesKey,
        CNContactDatesKey,
        CNContactImageDataAvailableKey,
        CNContactImageDataKey,
        CNContactThumbnailImageDataKey,
    ];
}

static CContact convert_contact(CNContactStore *store, CNContact *contact, NSError **error) {
    CContact cc;
    memset(&cc, 0, sizeof(CContact));

    cc.identifier = cstring_from_nsstring(contact.identifier);

    NSArray<CNContainer *> *containers = [store containersMatchingPredicate:[CNContainer predicateForContainerOfContactWithIdentifier:contact.identifier] error:error];
    if (error != NULL && *error != nil) {
        return cc;
    }
    if (containers.count > 0 && containers[0].identifier != nil) {
        cc.containerID = cstring_from_nsstring(containers[0].identifier);
    }

    cc.contactType = (int)contact.contactType;

    // Guard each field access with isKeyAvailable to avoid exceptions
    if ([contact isKeyAvailable:CNContactNamePrefixKey])
        cc.namePrefix = cstring_from_nsstring(contact.namePrefix);
    if ([contact isKeyAvailable:CNContactGivenNameKey])
        cc.givenName = cstring_from_nsstring(contact.givenName);
    if ([contact isKeyAvailable:CNContactMiddleNameKey])
        cc.middleName = cstring_from_nsstring(contact.middleName);
    if ([contact isKeyAvailable:CNContactFamilyNameKey])
        cc.familyName = cstring_from_nsstring(contact.familyName);
    if ([contact isKeyAvailable:CNContactPreviousFamilyNameKey])
        cc.previousFamilyName = cstring_from_nsstring(contact.previousFamilyName);
    if ([contact isKeyAvailable:CNContactNameSuffixKey])
        cc.nameSuffix = cstring_from_nsstring(contact.nameSuffix);
    if ([contact isKeyAvailable:CNContactNicknameKey])
        cc.nickname = cstring_from_nsstring(contact.nickname);
    if ([contact isKeyAvailable:CNContactPhoneticGivenNameKey])
        cc.phoneticGivenName = cstring_from_nsstring(contact.phoneticGivenName);
    if ([contact isKeyAvailable:CNContactPhoneticMiddleNameKey])
        cc.phoneticMiddleName = cstring_from_nsstring(contact.phoneticMiddleName);
    if ([contact isKeyAvailable:CNContactPhoneticFamilyNameKey])
        cc.phoneticFamilyName = cstring_from_nsstring(contact.phoneticFamilyName);
    if ([contact isKeyAvailable:CNContactOrganizationNameKey])
        cc.organizationName = cstring_from_nsstring(contact.organizationName);
    if ([contact isKeyAvailable:CNContactDepartmentNameKey])
        cc.departmentName = cstring_from_nsstring(contact.departmentName);
    if ([contact isKeyAvailable:CNContactJobTitleKey])
        cc.jobTitle = cstring_from_nsstring(contact.jobTitle);
    if ([contact isKeyAvailable:CNContactNoteKey])
        cc.note = cstring_from_nsstring(contact.note);

    // Birthday
    if ([contact isKeyAvailable:CNContactBirthdayKey] && contact.birthday != nil) {
        cc.hasBirthday = 1;
        cc.birthday = convert_date_components(contact.birthday);
    }

    // Phone numbers
    if ([contact isKeyAvailable:CNContactPhoneNumbersKey]) {
        NSArray<CNLabeledValue<CNPhoneNumber *> *> *phones = contact.phoneNumbers;
        cc.phoneNumbersCount = (int)phones.count;
        if (cc.phoneNumbersCount > 0) {
            cc.phoneNumbers = (CLabeledString *)malloc(sizeof(CLabeledString) * cc.phoneNumbersCount);
            for (int i = 0; i < cc.phoneNumbersCount; i++) {
                CNLabeledValue<CNPhoneNumber *> *lv = phones[i];
                cc.phoneNumbers[i].identifier = cstring_from_nsstring(lv.identifier);
                cc.phoneNumbers[i].label = cstring_from_nsstring(lv.label ? [CNLabeledValue localizedStringForLabel:lv.label] : nil);
                cc.phoneNumbers[i].value = cstring_from_nsstring(lv.value.stringValue);
            }
        }
    }

    // Email addresses
    if ([contact isKeyAvailable:CNContactEmailAddressesKey]) {
        NSArray<CNLabeledValue<NSString *> *> *emails = contact.emailAddresses;
        cc.emailAddressesCount = (int)emails.count;
        if (cc.emailAddressesCount > 0) {
            cc.emailAddresses = (CLabeledString *)malloc(sizeof(CLabeledString) * cc.emailAddressesCount);
            for (int i = 0; i < cc.emailAddressesCount; i++) {
                cc.emailAddresses[i] = convert_labeled_string(emails[i]);
            }
        }
    }

    // Postal addresses
    if ([contact isKeyAvailable:CNContactPostalAddressesKey]) {
        NSArray<CNLabeledValue<CNPostalAddress *> *> *addrs = contact.postalAddresses;
        cc.postalAddressesCount = (int)addrs.count;
        if (cc.postalAddressesCount > 0) {
            cc.postalAddresses = (CLabeledPostalAddress *)malloc(sizeof(CLabeledPostalAddress) * cc.postalAddressesCount);
            for (int i = 0; i < cc.postalAddressesCount; i++) {
                cc.postalAddresses[i] = convert_labeled_postal(addrs[i]);
            }
        }
    }

    // URL addresses
    if ([contact isKeyAvailable:CNContactUrlAddressesKey]) {
        NSArray<CNLabeledValue<NSString *> *> *urls = contact.urlAddresses;
        cc.urlAddressesCount = (int)urls.count;
        if (cc.urlAddressesCount > 0) {
            cc.urlAddresses = (CLabeledString *)malloc(sizeof(CLabeledString) * cc.urlAddressesCount);
            for (int i = 0; i < cc.urlAddressesCount; i++) {
                cc.urlAddresses[i] = convert_labeled_string(urls[i]);
            }
        }
    }

    // Contact relations
    if ([contact isKeyAvailable:CNContactRelationsKey]) {
        NSArray<CNLabeledValue<CNContactRelation *> *> *rels = contact.contactRelations;
        cc.contactRelationsCount = (int)rels.count;
        if (cc.contactRelationsCount > 0) {
            cc.contactRelations = (CLabeledContactRelation *)malloc(sizeof(CLabeledContactRelation) * cc.contactRelationsCount);
            for (int i = 0; i < cc.contactRelationsCount; i++) {
                cc.contactRelations[i] = convert_labeled_relation(rels[i]);
            }
        }
    }

    // Social profiles
    if ([contact isKeyAvailable:CNContactSocialProfilesKey]) {
        NSArray<CNLabeledValue<CNSocialProfile *> *> *profiles = contact.socialProfiles;
        cc.socialProfilesCount = (int)profiles.count;
        if (cc.socialProfilesCount > 0) {
            cc.socialProfiles = (CLabeledSocialProfile *)malloc(sizeof(CLabeledSocialProfile) * cc.socialProfilesCount);
            for (int i = 0; i < cc.socialProfilesCount; i++) {
                cc.socialProfiles[i] = convert_labeled_social(profiles[i]);
            }
        }
    }

    // Instant messages
    if ([contact isKeyAvailable:CNContactInstantMessageAddressesKey]) {
        NSArray<CNLabeledValue<CNInstantMessageAddress *> *> *ims = contact.instantMessageAddresses;
        cc.instantMessagesCount = (int)ims.count;
        if (cc.instantMessagesCount > 0) {
            cc.instantMessages = (CLabeledInstantMessage *)malloc(sizeof(CLabeledInstantMessage) * cc.instantMessagesCount);
            for (int i = 0; i < cc.instantMessagesCount; i++) {
                cc.instantMessages[i] = convert_labeled_im(ims[i]);
            }
        }
    }

    // Dates
    if ([contact isKeyAvailable:CNContactDatesKey]) {
        NSArray<CNLabeledValue<NSDateComponents *> *> *dates = contact.dates;
        cc.datesCount = (int)dates.count;
        if (cc.datesCount > 0) {
            cc.dates = (CLabeledDateComponents *)malloc(sizeof(CLabeledDateComponents) * cc.datesCount);
            for (int i = 0; i < cc.datesCount; i++) {
                cc.dates[i] = convert_labeled_date(dates[i]);
            }
        }
    }

    // Image data
    if ([contact isKeyAvailable:CNContactImageDataAvailableKey]) {
        cc.imageDataAvailable = contact.imageDataAvailable ? 1 : 0;
    }
    if ([contact isKeyAvailable:CNContactImageDataKey] && contact.imageData != nil) {
        NSData *data = contact.imageData;
        cc.imageDataLen = (int)data.length;
        cc.imageData = malloc(data.length);
        memcpy(cc.imageData, data.bytes, data.length);
    }
    if ([contact isKeyAvailable:CNContactThumbnailImageDataKey] && contact.thumbnailImageData != nil) {
        NSData *data = contact.thumbnailImageData;
        cc.thumbnailImageDataLen = (int)data.length;
        cc.thumbnailImageData = malloc(data.length);
        memcpy(cc.thumbnailImageData, data.bytes, data.length);
    }

    return cc;
}

// --- Apply CreateContactInput to CNMutableContact ---

static void apply_input_to_mutable(CNMutableContact *mc, CContact input) {
    mc.contactType = (CNContactType)input.contactType;

    NSString *s;
    s = nsstring_from_cstring(input.namePrefix);
    if (s.length > 0) mc.namePrefix = s;
    s = nsstring_from_cstring(input.givenName);
    if (s.length > 0) mc.givenName = s;
    s = nsstring_from_cstring(input.middleName);
    if (s.length > 0) mc.middleName = s;
    s = nsstring_from_cstring(input.familyName);
    if (s.length > 0) mc.familyName = s;
    s = nsstring_from_cstring(input.previousFamilyName);
    if (s.length > 0) mc.previousFamilyName = s;
    s = nsstring_from_cstring(input.nameSuffix);
    if (s.length > 0) mc.nameSuffix = s;
    s = nsstring_from_cstring(input.nickname);
    if (s.length > 0) mc.nickname = s;
    s = nsstring_from_cstring(input.phoneticGivenName);
    if (s.length > 0) mc.phoneticGivenName = s;
    s = nsstring_from_cstring(input.phoneticMiddleName);
    if (s.length > 0) mc.phoneticMiddleName = s;
    s = nsstring_from_cstring(input.phoneticFamilyName);
    if (s.length > 0) mc.phoneticFamilyName = s;
    s = nsstring_from_cstring(input.organizationName);
    if (s.length > 0) mc.organizationName = s;
    s = nsstring_from_cstring(input.departmentName);
    if (s.length > 0) mc.departmentName = s;
    s = nsstring_from_cstring(input.jobTitle);
    if (s.length > 0) mc.jobTitle = s;
    s = nsstring_from_cstring(input.note);
    if (s.length > 0) mc.note = s;

    // Birthday
    if (input.hasBirthday) {
        NSDateComponents *dc = [[NSDateComponents alloc] init];
        if (input.birthday.year > 0) dc.year = input.birthday.year;
        if (input.birthday.month > 0) dc.month = input.birthday.month;
        if (input.birthday.day > 0) dc.day = input.birthday.day;
        mc.birthday = dc;
    }

    // Phone numbers
    if (input.phoneNumbersCount > 0) {
        NSMutableArray<CNLabeledValue<CNPhoneNumber *> *> *phones = [NSMutableArray array];
        for (int i = 0; i < input.phoneNumbersCount; i++) {
            NSString *label = nsstring_from_cstring(input.phoneNumbers[i].label);
            NSString *value = nsstring_from_cstring(input.phoneNumbers[i].value);
            CNPhoneNumber *pn = [CNPhoneNumber phoneNumberWithStringValue:value];
            // Map common label strings to CNLabel constants
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if ([label isEqualToString:@"mobile"] || [label isEqualToString:@"Mobile"]) cnLabel = CNLabelPhoneNumberMobile;
            else if ([label isEqualToString:@"main"] || [label isEqualToString:@"Main"]) cnLabel = CNLabelPhoneNumberMain;
            else if ([label isEqualToString:@"iPhone"]) cnLabel = CNLabelPhoneNumberiPhone;
            else if (label.length > 0) cnLabel = label;
            [phones addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:pn]];
        }
        mc.phoneNumbers = phones;
    }

    // Email addresses
    if (input.emailAddressesCount > 0) {
        NSMutableArray<CNLabeledValue<NSString *> *> *emails = [NSMutableArray array];
        for (int i = 0; i < input.emailAddressesCount; i++) {
            NSString *label = nsstring_from_cstring(input.emailAddresses[i].label);
            NSString *value = nsstring_from_cstring(input.emailAddresses[i].value);
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if (label.length > 0) cnLabel = label;
            [emails addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:value]];
        }
        mc.emailAddresses = emails;
    }

    // Postal addresses
    if (input.postalAddressesCount > 0) {
        NSMutableArray<CNLabeledValue<CNPostalAddress *> *> *addrs = [NSMutableArray array];
        for (int i = 0; i < input.postalAddressesCount; i++) {
            CNMutablePostalAddress *pa = [[CNMutablePostalAddress alloc] init];
            pa.street = nsstring_from_cstring(input.postalAddresses[i].value.street);
            pa.city = nsstring_from_cstring(input.postalAddresses[i].value.city);
            pa.state = nsstring_from_cstring(input.postalAddresses[i].value.state);
            pa.postalCode = nsstring_from_cstring(input.postalAddresses[i].value.postalCode);
            pa.country = nsstring_from_cstring(input.postalAddresses[i].value.country);
            pa.ISOCountryCode = nsstring_from_cstring(input.postalAddresses[i].value.isoCountryCode);
            NSString *label = nsstring_from_cstring(input.postalAddresses[i].label);
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if (label.length > 0) cnLabel = label;
            [addrs addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:pa]];
        }
        mc.postalAddresses = addrs;
    }

    // URL addresses
    if (input.urlAddressesCount > 0) {
        NSMutableArray<CNLabeledValue<NSString *> *> *urls = [NSMutableArray array];
        for (int i = 0; i < input.urlAddressesCount; i++) {
            NSString *label = nsstring_from_cstring(input.urlAddresses[i].label);
            NSString *value = nsstring_from_cstring(input.urlAddresses[i].value);
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if (label.length > 0) cnLabel = label;
            [urls addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:value]];
        }
        mc.urlAddresses = urls;
    }

    // Contact relations
    if (input.contactRelationsCount > 0) {
        NSMutableArray<CNLabeledValue<CNContactRelation *> *> *rels = [NSMutableArray array];
        for (int i = 0; i < input.contactRelationsCount; i++) {
            NSString *label = nsstring_from_cstring(input.contactRelations[i].label);
            NSString *name = nsstring_from_cstring(input.contactRelations[i].value.name);
            CNContactRelation *rel = [CNContactRelation contactRelationWithName:name];
            [rels addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:rel]];
        }
        mc.contactRelations = rels;
    }

    // Social profiles
    if (input.socialProfilesCount > 0) {
        NSMutableArray<CNLabeledValue<CNSocialProfile *> *> *profiles = [NSMutableArray array];
        for (int i = 0; i < input.socialProfilesCount; i++) {
            NSString *label = nsstring_from_cstring(input.socialProfiles[i].label);
            NSString *urlStr = nsstring_from_cstring(input.socialProfiles[i].value.urlString);
            NSString *username = nsstring_from_cstring(input.socialProfiles[i].value.username);
            NSString *service = nsstring_from_cstring(input.socialProfiles[i].value.service);
            CNSocialProfile *sp = [[CNSocialProfile alloc] initWithUrlString:urlStr username:username userIdentifier:nil service:service];
            [profiles addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:sp]];
        }
        mc.socialProfiles = profiles;
    }

    // Instant messages
    if (input.instantMessagesCount > 0) {
        NSMutableArray<CNLabeledValue<CNInstantMessageAddress *> *> *ims = [NSMutableArray array];
        for (int i = 0; i < input.instantMessagesCount; i++) {
            NSString *label = nsstring_from_cstring(input.instantMessages[i].label);
            NSString *username = nsstring_from_cstring(input.instantMessages[i].value.instantUsername);
            NSString *service = nsstring_from_cstring(input.instantMessages[i].value.instantService);
            CNInstantMessageAddress *im = [[CNInstantMessageAddress alloc] initWithUsername:username service:service];
            [ims addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:im]];
        }
        mc.instantMessageAddresses = ims;
    }

    // Dates
    if (input.datesCount > 0) {
        NSMutableArray<CNLabeledValue<NSDateComponents *> *> *dates = [NSMutableArray array];
        for (int i = 0; i < input.datesCount; i++) {
            NSString *label = nsstring_from_cstring(input.dates[i].label);
            NSDateComponents *dc = [[NSDateComponents alloc] init];
            if (input.dates[i].value.year > 0) dc.year = input.dates[i].value.year;
            if (input.dates[i].value.month > 0) dc.month = input.dates[i].value.month;
            if (input.dates[i].value.day > 0) dc.day = input.dates[i].value.day;
            [dates addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:dc]];
        }
        mc.dates = dates;
    }

    // Image data
    if (input.imageData != NULL && input.imageDataLen > 0) {
        mc.imageData = [NSData dataWithBytes:input.imageData length:input.imageDataLen];
    }
}

static void apply_full_input_to_mutable(CNMutableContact *mc, CContact input) {
    mc.contactType = (CNContactType)input.contactType;

    mc.namePrefix = nsstring_from_cstring(input.namePrefix);
    mc.givenName = nsstring_from_cstring(input.givenName);
    mc.middleName = nsstring_from_cstring(input.middleName);
    mc.familyName = nsstring_from_cstring(input.familyName);
    mc.previousFamilyName = nsstring_from_cstring(input.previousFamilyName);
    mc.nameSuffix = nsstring_from_cstring(input.nameSuffix);
    mc.nickname = nsstring_from_cstring(input.nickname);
    mc.phoneticGivenName = nsstring_from_cstring(input.phoneticGivenName);
    mc.phoneticMiddleName = nsstring_from_cstring(input.phoneticMiddleName);
    mc.phoneticFamilyName = nsstring_from_cstring(input.phoneticFamilyName);
    mc.organizationName = nsstring_from_cstring(input.organizationName);
    mc.departmentName = nsstring_from_cstring(input.departmentName);
    mc.jobTitle = nsstring_from_cstring(input.jobTitle);

    // Intentionally skip Note updates due entitlement constraints.

    if (input.hasBirthday) {
        NSDateComponents *dc = [[NSDateComponents alloc] init];
        if (input.birthday.year > 0) dc.year = input.birthday.year;
        if (input.birthday.month > 0) dc.month = input.birthday.month;
        if (input.birthday.day > 0) dc.day = input.birthday.day;
        mc.birthday = dc;
    } else {
        mc.birthday = nil;
    }

    if (input.phoneNumbersCount > 0) {
        NSMutableArray<CNLabeledValue<CNPhoneNumber *> *> *phones = [NSMutableArray array];
        for (int i = 0; i < input.phoneNumbersCount; i++) {
            NSString *label = nsstring_from_cstring(input.phoneNumbers[i].label);
            NSString *value = nsstring_from_cstring(input.phoneNumbers[i].value);
            CNPhoneNumber *pn = [CNPhoneNumber phoneNumberWithStringValue:value];
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if ([label isEqualToString:@"mobile"] || [label isEqualToString:@"Mobile"]) cnLabel = CNLabelPhoneNumberMobile;
            else if ([label isEqualToString:@"main"] || [label isEqualToString:@"Main"]) cnLabel = CNLabelPhoneNumberMain;
            else if ([label isEqualToString:@"iPhone"]) cnLabel = CNLabelPhoneNumberiPhone;
            else if (label.length > 0) cnLabel = label;
            [phones addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:pn]];
        }
        mc.phoneNumbers = phones;
    } else {
        mc.phoneNumbers = @[];
    }

    if (input.emailAddressesCount > 0) {
        NSMutableArray<CNLabeledValue<NSString *> *> *emails = [NSMutableArray array];
        for (int i = 0; i < input.emailAddressesCount; i++) {
            NSString *label = nsstring_from_cstring(input.emailAddresses[i].label);
            NSString *value = nsstring_from_cstring(input.emailAddresses[i].value);
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if (label.length > 0) cnLabel = label;
            [emails addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:value]];
        }
        mc.emailAddresses = emails;
    } else {
        mc.emailAddresses = @[];
    }

    if (input.postalAddressesCount > 0) {
        NSMutableArray<CNLabeledValue<CNPostalAddress *> *> *addrs = [NSMutableArray array];
        for (int i = 0; i < input.postalAddressesCount; i++) {
            CNMutablePostalAddress *pa = [[CNMutablePostalAddress alloc] init];
            pa.street = nsstring_from_cstring(input.postalAddresses[i].value.street);
            pa.city = nsstring_from_cstring(input.postalAddresses[i].value.city);
            pa.state = nsstring_from_cstring(input.postalAddresses[i].value.state);
            pa.postalCode = nsstring_from_cstring(input.postalAddresses[i].value.postalCode);
            pa.country = nsstring_from_cstring(input.postalAddresses[i].value.country);
            pa.ISOCountryCode = nsstring_from_cstring(input.postalAddresses[i].value.isoCountryCode);
            NSString *label = nsstring_from_cstring(input.postalAddresses[i].label);
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if (label.length > 0) cnLabel = label;
            [addrs addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:pa]];
        }
        mc.postalAddresses = addrs;
    } else {
        mc.postalAddresses = @[];
    }

    if (input.urlAddressesCount > 0) {
        NSMutableArray<CNLabeledValue<NSString *> *> *urls = [NSMutableArray array];
        for (int i = 0; i < input.urlAddressesCount; i++) {
            NSString *label = nsstring_from_cstring(input.urlAddresses[i].label);
            NSString *value = nsstring_from_cstring(input.urlAddresses[i].value);
            NSString *cnLabel = nil;
            if ([label isEqualToString:@"home"] || [label isEqualToString:@"Home"]) cnLabel = CNLabelHome;
            else if ([label isEqualToString:@"work"] || [label isEqualToString:@"Work"]) cnLabel = CNLabelWork;
            else if (label.length > 0) cnLabel = label;
            [urls addObject:[CNLabeledValue labeledValueWithLabel:cnLabel value:value]];
        }
        mc.urlAddresses = urls;
    } else {
        mc.urlAddresses = @[];
    }

    if (input.contactRelationsCount > 0) {
        NSMutableArray<CNLabeledValue<CNContactRelation *> *> *rels = [NSMutableArray array];
        for (int i = 0; i < input.contactRelationsCount; i++) {
            NSString *label = nsstring_from_cstring(input.contactRelations[i].label);
            NSString *name = nsstring_from_cstring(input.contactRelations[i].value.name);
            CNContactRelation *rel = [CNContactRelation contactRelationWithName:name];
            [rels addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:rel]];
        }
        mc.contactRelations = rels;
    } else {
        mc.contactRelations = @[];
    }

    if (input.socialProfilesCount > 0) {
        NSMutableArray<CNLabeledValue<CNSocialProfile *> *> *profiles = [NSMutableArray array];
        for (int i = 0; i < input.socialProfilesCount; i++) {
            NSString *label = nsstring_from_cstring(input.socialProfiles[i].label);
            NSString *urlStr = nsstring_from_cstring(input.socialProfiles[i].value.urlString);
            NSString *username = nsstring_from_cstring(input.socialProfiles[i].value.username);
            NSString *service = nsstring_from_cstring(input.socialProfiles[i].value.service);
            CNSocialProfile *sp = [[CNSocialProfile alloc] initWithUrlString:urlStr username:username userIdentifier:nil service:service];
            [profiles addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:sp]];
        }
        mc.socialProfiles = profiles;
    } else {
        mc.socialProfiles = @[];
    }

    if (input.instantMessagesCount > 0) {
        NSMutableArray<CNLabeledValue<CNInstantMessageAddress *> *> *ims = [NSMutableArray array];
        for (int i = 0; i < input.instantMessagesCount; i++) {
            NSString *label = nsstring_from_cstring(input.instantMessages[i].label);
            NSString *username = nsstring_from_cstring(input.instantMessages[i].value.instantUsername);
            NSString *service = nsstring_from_cstring(input.instantMessages[i].value.instantService);
            CNInstantMessageAddress *im = [[CNInstantMessageAddress alloc] initWithUsername:username service:service];
            [ims addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:im]];
        }
        mc.instantMessageAddresses = ims;
    } else {
        mc.instantMessageAddresses = @[];
    }

    if (input.datesCount > 0) {
        NSMutableArray<CNLabeledValue<NSDateComponents *> *> *dates = [NSMutableArray array];
        for (int i = 0; i < input.datesCount; i++) {
            NSString *label = nsstring_from_cstring(input.dates[i].label);
            NSDateComponents *dc = [[NSDateComponents alloc] init];
            if (input.dates[i].value.year > 0) dc.year = input.dates[i].value.year;
            if (input.dates[i].value.month > 0) dc.month = input.dates[i].value.month;
            if (input.dates[i].value.day > 0) dc.day = input.dates[i].value.day;
            [dates addObject:[CNLabeledValue labeledValueWithLabel:(label.length > 0 ? label : nil) value:dc]];
        }
        mc.dates = dates;
    } else {
        mc.dates = @[];
    }

    if (input.imageData != NULL && input.imageDataLen > 0) {
        mc.imageData = [NSData dataWithBytes:input.imageData length:input.imageDataLen];
    } else {
        mc.imageData = nil;
    }
}

// --- Filter helpers ---

static BOOL string_matches_filter(NSString *fieldValue, NSString *filterValue, int op) {
    if (fieldValue == nil) fieldValue = @"";
    if (filterValue == nil) filterValue = @"";
    switch (op) {
        case 0: // equals
            return [fieldValue caseInsensitiveCompare:filterValue] == NSOrderedSame;
        case 1: // contains
            return [fieldValue rangeOfString:filterValue options:NSCaseInsensitiveSearch].location != NSNotFound;
        case 2: // notContains
            return [fieldValue rangeOfString:filterValue options:NSCaseInsensitiveSearch].location == NSNotFound;
        default:
            return YES;
    }
}

static BOOL contact_matches_filter(CNContact *contact, CFilter filter) {
    NSString *fieldName = nsstring_from_cstring(filter.fieldName);
    NSString *filterValue = nsstring_from_cstring(filter.value);
    int op = filter.op;

    // Single-value string fields
    if ([fieldName isEqualToString:@"givenName"] && [contact isKeyAvailable:CNContactGivenNameKey]) {
        return string_matches_filter(contact.givenName, filterValue, op);
    }
    if ([fieldName isEqualToString:@"familyName"] && [contact isKeyAvailable:CNContactFamilyNameKey]) {
        return string_matches_filter(contact.familyName, filterValue, op);
    }
    if ([fieldName isEqualToString:@"middleName"] && [contact isKeyAvailable:CNContactMiddleNameKey]) {
        return string_matches_filter(contact.middleName, filterValue, op);
    }
    if ([fieldName isEqualToString:@"organizationName"] && [contact isKeyAvailable:CNContactOrganizationNameKey]) {
        return string_matches_filter(contact.organizationName, filterValue, op);
    }
    if ([fieldName isEqualToString:@"departmentName"] && [contact isKeyAvailable:CNContactDepartmentNameKey]) {
        return string_matches_filter(contact.departmentName, filterValue, op);
    }
    if ([fieldName isEqualToString:@"jobTitle"] && [contact isKeyAvailable:CNContactJobTitleKey]) {
        return string_matches_filter(contact.jobTitle, filterValue, op);
    }
    if ([fieldName isEqualToString:@"nickname"] && [contact isKeyAvailable:CNContactNicknameKey]) {
        return string_matches_filter(contact.nickname, filterValue, op);
    }
    if ([fieldName isEqualToString:@"namePrefix"] && [contact isKeyAvailable:CNContactNamePrefixKey]) {
        return string_matches_filter(contact.namePrefix, filterValue, op);
    }
    if ([fieldName isEqualToString:@"nameSuffix"] && [contact isKeyAvailable:CNContactNameSuffixKey]) {
        return string_matches_filter(contact.nameSuffix, filterValue, op);
    }
    if ([fieldName isEqualToString:@"note"] && [contact isKeyAvailable:CNContactNoteKey]) {
        return string_matches_filter(contact.note, filterValue, op);
    }

    // Multi-value fields: match if ANY value matches
    if ([fieldName isEqualToString:@"emailAddresses"] && [contact isKeyAvailable:CNContactEmailAddressesKey]) {
        for (CNLabeledValue<NSString *> *lv in contact.emailAddresses) {
            if (string_matches_filter(lv.value, filterValue, op)) return YES;
        }
        // For notContains, all must not contain
        if (op == 2) {
            for (CNLabeledValue<NSString *> *lv in contact.emailAddresses) {
                if (!string_matches_filter(lv.value, filterValue, op)) return NO;
            }
            return YES;
        }
        return NO;
    }
    if ([fieldName isEqualToString:@"phoneNumbers"] && [contact isKeyAvailable:CNContactPhoneNumbersKey]) {
        for (CNLabeledValue<CNPhoneNumber *> *lv in contact.phoneNumbers) {
            if (string_matches_filter(lv.value.stringValue, filterValue, op)) return YES;
        }
        if (op == 2) {
            for (CNLabeledValue<CNPhoneNumber *> *lv in contact.phoneNumbers) {
                if (!string_matches_filter(lv.value.stringValue, filterValue, op)) return NO;
            }
            return YES;
        }
        return NO;
    }

    // Unknown field name — don't filter (match everything)
    return YES;
}

static BOOL contact_matches_all_filters(CNContact *contact, CFilter *filters, int filterCount) {
    for (int i = 0; i < filterCount; i++) {
        if (!contact_matches_filter(contact, filters[i])) {
            return NO;
        }
    }
    return YES;
}

// --- Bridge function implementations ---

int bridge_check_authorization(void) {
    return (int)[CNContactStore authorizationStatusForEntityType:CNEntityTypeContacts];
}

CAuthResult bridge_request_access(void) {
    CAuthResult result;
    memset(&result, 0, sizeof(CAuthResult));

    CNContactStore *store = [[CNContactStore alloc] init];
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    __block BOOL granted = NO;
    __block NSError *accessError = nil;

    [store requestAccessForEntityType:CNEntityTypeContacts completionHandler:^(BOOL g, NSError *error) {
        granted = g;
        accessError = error;
        dispatch_semaphore_signal(sem);
    }];

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);

    result.status = (int)[CNContactStore authorizationStatusForEntityType:CNEntityTypeContacts];
    if (accessError != nil) {
        result.error = cstring_from_error(accessError);
    }
    return result;
}

CContactResult bridge_get_contact(BridgeString identifier) {
    CContactResult result;
    memset(&result, 0, sizeof(CContactResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *ident = nsstring_from_cstring(identifier);
        NSError *error = nil;

        CNContact *contact = [store unifiedContactWithIdentifier:ident keysToFetch:allContactKeys() error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        if (contact == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"contact %@ not found", ident]);
            return result;
        }
        result.contact = convert_contact(store, contact, &error);
        if (error != nil) {
            result.error = cstring_from_error(error);
            bridge_free_contact(&result.contact);
            memset(&result.contact, 0, sizeof(CContact));
            return result;
        }
    }
    return result;
}

CContactListResult bridge_list_contacts(CFilter *filters, int filterCount) {
    CContactListResult result;
    memset(&result, 0, sizeof(CContactListResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        CNContactFetchRequest *request = [[CNContactFetchRequest alloc] initWithKeysToFetch:allContactKeys()];
        request.sortOrder = CNContactSortOrderGivenName;

        NSMutableArray<CNContact *> *matched = [NSMutableArray array];
        NSError *error = nil;

        BOOL success = [store enumerateContactsWithFetchRequest:request error:&error usingBlock:^(CNContact * _Nonnull contact, BOOL * _Nonnull stop) {
            if (filterCount == 0 || contact_matches_all_filters(contact, filters, filterCount)) {
                [matched addObject:contact];
            }
        }];

        if (!success || error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }

        result.count = (int)matched.count;
        if (result.count > 0) {
            result.contacts = (CContact *)malloc(sizeof(CContact) * result.count);
            for (int i = 0; i < result.count; i++) {
                result.contacts[i] = convert_contact(store, matched[i], &error);
                if (error != nil) {
                    result.error = cstring_from_error(error);
                    for (int j = 0; j <= i; j++) {
                        bridge_free_contact(&result.contacts[j]);
                    }
                    free(result.contacts);
                    result.contacts = NULL;
                    result.count = 0;
                    return result;
                }
            }
        }
    }
    return result;
}

CCreateResult bridge_create_contact(CContact input, BridgeString containerID) {
    CCreateResult result;
    memset(&result, 0, sizeof(CCreateResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        CNMutableContact *mc = [[CNMutableContact alloc] init];
        apply_input_to_mutable(mc, input);

        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        NSString *cid = nsstring_from_cstring(containerID);
        [saveRequest addContact:mc toContainerWithIdentifier:(cid.length > 0 ? cid : nil)];

        NSError *error = nil;
        if (![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
        result.identifier = cstring_from_nsstring(mc.identifier);
    }
    return result;
}

CSimpleResult bridge_update_contact(CContact input) {
    CSimpleResult result;
    memset(&result, 0, sizeof(CSimpleResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *ident = nsstring_from_cstring(input.identifier);
        if (ident.length == 0) {
            result.error = cstring_from_nsstring(@"identifier is required");
            return result;
        }

        NSError *error = nil;
        CNContact *contact = [store unifiedContactWithIdentifier:ident keysToFetch:allContactKeys() error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        if (contact == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"contact %@ not found", ident]);
            return result;
        }

        CNMutableContact *mc = [contact mutableCopy];
        apply_full_input_to_mutable(mc, input);

        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        [saveRequest updateContact:mc];

        if (![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
    }
    return result;
}

CSimpleResult bridge_delete_contact(BridgeString identifier) {
    CSimpleResult result;
    memset(&result, 0, sizeof(CSimpleResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *ident = nsstring_from_cstring(identifier);
        NSError *error = nil;

        CNContact *contact = [store unifiedContactWithIdentifier:ident keysToFetch:@[CNContactIdentifierKey] error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        if (contact == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"contact %@ not found", ident]);
            return result;
        }

        CNMutableContact *mc = [contact mutableCopy];
        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        [saveRequest deleteContact:mc];

        if (![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
    }
    return result;
}

CGroupListResult bridge_list_groups(BridgeString containerID, int includeHierarchy) {
    CGroupListResult result;
    memset(&result, 0, sizeof(CGroupListResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSError *error = nil;

        NSString *cid = nsstring_from_cstring(containerID);
        NSPredicate *predicate = nil;
        if (cid.length > 0) {
            predicate = [CNGroup predicateForGroupsInContainerWithIdentifier:cid];
        }

        NSArray<CNGroup *> *groups = [store groupsMatchingPredicate:predicate error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }

        NSMutableDictionary<NSString *, NSString *> *groupToContainer = [NSMutableDictionary dictionary];
        for (CNGroup *group in groups) {
            NSArray<CNContainer *> *containers = [store containersMatchingPredicate:[CNContainer predicateForContainerOfGroupWithIdentifier:group.identifier] error:&error];
            if (error != nil) {
                result.error = cstring_from_error(error);
                return result;
            }
            NSString *groupContainerID = @"";
            if (containers.count > 0 && containers[0].identifier != nil) {
                groupContainerID = containers[0].identifier;
            }
            groupToContainer[group.identifier] = groupContainerID;
        }

        NSMutableDictionary<NSString *, NSString *> *parentByChild = [NSMutableDictionary dictionary];
        NSMutableDictionary<NSString *, NSMutableArray<NSString *> *> *childrenByParent = [NSMutableDictionary dictionary];

        if (includeHierarchy != 0) {
            for (CNGroup *group in groups) {
                NSArray<CNGroup *> *subgroups = [store groupsMatchingPredicate:[CNGroup predicateForSubgroupsInGroupWithIdentifier:group.identifier] error:&error];
                if (error != nil) {
                    result.error = cstring_from_error(error);
                    return result;
                }

                NSMutableArray<NSString *> *childIDs = [NSMutableArray arrayWithCapacity:subgroups.count];
                for (CNGroup *subgroup in subgroups) {
                    [childIDs addObject:subgroup.identifier];
                    parentByChild[subgroup.identifier] = group.identifier;
                }
                childrenByParent[group.identifier] = childIDs;
            }
        }

        result.count = (int)groups.count;
        if (result.count > 0) {
            result.groups = (CGroup *)malloc(sizeof(CGroup) * result.count);
            for (int i = 0; i < result.count; i++) {
                CNGroup *g = groups[i];
                result.groups[i].identifier = cstring_from_nsstring(g.identifier);
                result.groups[i].name = cstring_from_nsstring(g.name);
                NSString *resolvedContainerID = groupToContainer[g.identifier];
                result.groups[i].containerID = cstring_from_nsstring(resolvedContainerID ?: @"");

                NSString *parentID = parentByChild[g.identifier];
                result.groups[i].parentGroupID = cstring_from_nsstring(parentID ?: @"");

                NSArray<NSString *> *subgroupIDs = childrenByParent[g.identifier];
                int subgroupCount = (int)subgroupIDs.count;
                result.groups[i].subgroupIDsCount = subgroupCount;
                if (subgroupCount > 0) {
                    result.groups[i].subgroupIDs = (BridgeString *)malloc(sizeof(BridgeString) * subgroupCount);
                    for (int j = 0; j < subgroupCount; j++) {
                        result.groups[i].subgroupIDs[j] = cstring_from_nsstring(subgroupIDs[j]);
                    }
                } else {
                    result.groups[i].subgroupIDs = NULL;
                }
            }
        }
    }
    return result;
}

CCreateResult bridge_create_group(BridgeString name, BridgeString containerID, BridgeString parentGroupID) {
    CCreateResult result;
    memset(&result, 0, sizeof(CCreateResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        CNMutableGroup *mg = [[CNMutableGroup alloc] init];
        mg.name = nsstring_from_cstring(name);

        NSString *cid = nsstring_from_cstring(containerID);
        NSString *pid = nsstring_from_cstring(parentGroupID);

        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        [saveRequest addGroup:mg toContainerWithIdentifier:(cid.length > 0 ? cid : nil)];

        // If parent group specified, add as subgroup
        if (pid.length > 0) {
            NSError *error = nil;
            // Find the parent group
            NSArray<CNGroup *> *allGroups = [store groupsMatchingPredicate:nil error:&error];
            if (error != nil) {
                result.error = cstring_from_error(error);
                return result;
            }
            CNGroup *parentGroup = nil;
            for (CNGroup *g in allGroups) {
                if ([g.identifier isEqualToString:pid]) {
                    parentGroup = g;
                    break;
                }
            }
            if (parentGroup == nil) {
                result.error = cstring_from_nsstring([NSString stringWithFormat:@"parent group %@ not found", pid]);
                return result;
            }
            [saveRequest addSubgroup:mg toGroup:parentGroup];
        }

        NSError *error = nil;
        if (![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
        result.identifier = cstring_from_nsstring(mg.identifier);
    }
    return result;
}

CSimpleResult bridge_update_group(BridgeString identifier, BridgeString name, int hasName, BridgeString parentGroupID, int hasParentGroupID) {
    CSimpleResult result;
    memset(&result, 0, sizeof(CSimpleResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *ident = nsstring_from_cstring(identifier);
        NSString *groupName = nsstring_from_cstring(name);
        NSString *parentID = nsstring_from_cstring(parentGroupID);

        if (ident.length == 0) {
            result.error = cstring_from_nsstring(@"identifier is required");
            return result;
        }
        if (hasParentGroupID != 0 && [ident isEqualToString:parentID]) {
            result.error = cstring_from_nsstring(@"parentGroupID cannot equal identifier");
            return result;
        }

        NSError *error = nil;
        NSArray<CNGroup *> *allGroups = [store groupsMatchingPredicate:nil error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }

        CNGroup *targetGroup = nil;
        CNGroup *newParentGroup = nil;
        for (CNGroup *g in allGroups) {
            if ([g.identifier isEqualToString:ident]) {
                targetGroup = g;
            }
            if (hasParentGroupID != 0 && parentID.length > 0 && [g.identifier isEqualToString:parentID]) {
                newParentGroup = g;
            }
        }
        if (targetGroup == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"group %@ not found", ident]);
            return result;
        }
        if (hasParentGroupID != 0 && parentID.length > 0 && newParentGroup == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"parent group %@ not found", parentID]);
            return result;
        }

        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        BOOL hasMutations = NO;

        if (hasName != 0) {
            CNMutableGroup *mutableGroup = [targetGroup mutableCopy];
            mutableGroup.name = groupName;
            [saveRequest updateGroup:mutableGroup];
            hasMutations = YES;
        }

        if (hasParentGroupID != 0) {
            NSMutableArray<CNGroup *> *currentParents = [NSMutableArray array];
            for (CNGroup *candidateParent in allGroups) {
                NSArray<CNGroup *> *subgroups = [store groupsMatchingPredicate:[CNGroup predicateForSubgroupsInGroupWithIdentifier:candidateParent.identifier] error:&error];
                if (error != nil) {
                    result.error = cstring_from_error(error);
                    return result;
                }
                for (CNGroup *subgroup in subgroups) {
                    if ([subgroup.identifier isEqualToString:ident]) {
                        [currentParents addObject:candidateParent];
                        break;
                    }
                }
            }

            if (parentID.length == 0) {
                for (CNGroup *existingParent in currentParents) {
                    [saveRequest removeSubgroup:targetGroup fromGroup:existingParent];
                    hasMutations = YES;
                }
            } else {
                BOOL alreadyChild = NO;
                for (CNGroup *existingParent in currentParents) {
                    if ([existingParent.identifier isEqualToString:parentID]) {
                        alreadyChild = YES;
                        continue;
                    }
                    [saveRequest removeSubgroup:targetGroup fromGroup:existingParent];
                    hasMutations = YES;
                }
                if (!alreadyChild) {
                    [saveRequest addSubgroup:targetGroup toGroup:newParentGroup];
                    hasMutations = YES;
                }
            }
        }

        if (hasMutations && ![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
    }
    return result;
}

CSimpleResult bridge_delete_group(BridgeString identifier) {
    CSimpleResult result;
    memset(&result, 0, sizeof(CSimpleResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *ident = nsstring_from_cstring(identifier);
        NSError *error = nil;

        NSArray<CNGroup *> *allGroups = [store groupsMatchingPredicate:nil error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }

        CNGroup *targetGroup = nil;
        for (CNGroup *g in allGroups) {
            if ([g.identifier isEqualToString:ident]) {
                targetGroup = g;
                break;
            }
        }
        if (targetGroup == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"group %@ not found", ident]);
            return result;
        }

        CNMutableGroup *mg = [targetGroup mutableCopy];
        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        [saveRequest deleteGroup:mg];

        if (![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
    }
    return result;
}

CSimpleResult bridge_add_contact_to_group(BridgeString contactID, BridgeString groupID) {
    CSimpleResult result;
    memset(&result, 0, sizeof(CSimpleResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *cid = nsstring_from_cstring(contactID);
        NSString *gid = nsstring_from_cstring(groupID);
        NSError *error = nil;

        // Fetch contact
        CNContact *contact = [store unifiedContactWithIdentifier:cid keysToFetch:@[CNContactIdentifierKey] error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        if (contact == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"contact %@ not found", cid]);
            return result;
        }

        // Fetch group
        NSArray<CNGroup *> *allGroups = [store groupsMatchingPredicate:nil error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        CNGroup *targetGroup = nil;
        for (CNGroup *g in allGroups) {
            if ([g.identifier isEqualToString:gid]) {
                targetGroup = g;
                break;
            }
        }
        if (targetGroup == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"group %@ not found", gid]);
            return result;
        }

        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        [saveRequest addMember:contact toGroup:targetGroup];

        if (![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
    }
    return result;
}

CSimpleResult bridge_remove_contact_from_group(BridgeString contactID, BridgeString groupID) {
    CSimpleResult result;
    memset(&result, 0, sizeof(CSimpleResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *cid = nsstring_from_cstring(contactID);
        NSString *gid = nsstring_from_cstring(groupID);
        NSError *error = nil;

        // Fetch contact
        CNContact *contact = [store unifiedContactWithIdentifier:cid keysToFetch:@[CNContactIdentifierKey] error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        if (contact == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"contact %@ not found", cid]);
            return result;
        }

        // Fetch group
        NSArray<CNGroup *> *allGroups = [store groupsMatchingPredicate:nil error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        CNGroup *targetGroup = nil;
        for (CNGroup *g in allGroups) {
            if ([g.identifier isEqualToString:gid]) {
                targetGroup = g;
                break;
            }
        }
        if (targetGroup == nil) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"group %@ not found", gid]);
            return result;
        }

        CNSaveRequest *saveRequest = [[CNSaveRequest alloc] init];
        [saveRequest removeMember:contact fromGroup:targetGroup];

        if (![store executeSaveRequest:saveRequest error:&error]) {
            result.error = cstring_from_error(error);
            return result;
        }
    }
    return result;
}

CContainerResult bridge_get_container(BridgeString identifier) {
    CContainerResult result;
    memset(&result, 0, sizeof(CContainerResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *ident = nsstring_from_cstring(identifier);
        NSError *error = nil;

        NSArray<CNContainer *> *containers = [store containersMatchingPredicate:[CNContainer predicateForContainersWithIdentifiers:@[ident]] error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }
        if (containers.count == 0) {
            result.error = cstring_from_nsstring([NSString stringWithFormat:@"container %@ not found", ident]);
            return result;
        }

        CNContainer *c = containers[0];
        result.container.identifier = cstring_from_nsstring(c.identifier);
        result.container.name = cstring_from_nsstring(c.name);
        result.container.containerType = (int)c.type;
    }
    return result;
}

CContainerListResult bridge_list_containers(void) {
    CContainerListResult result;
    memset(&result, 0, sizeof(CContainerListResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSError *error = nil;

        NSArray<CNContainer *> *containers = [store containersMatchingPredicate:nil error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }

        result.count = (int)containers.count;
        if (result.count > 0) {
            result.containers = (CContainer *)malloc(sizeof(CContainer) * result.count);
            for (int i = 0; i < result.count; i++) {
                CNContainer *c = containers[i];
                result.containers[i].identifier = cstring_from_nsstring(c.identifier);
                result.containers[i].name = cstring_from_nsstring(c.name);
                result.containers[i].containerType = (int)c.type;
            }
        }
    }
    return result;
}

CDefaultContainerResult bridge_default_container_id(void) {
    CDefaultContainerResult result;
    memset(&result, 0, sizeof(CDefaultContainerResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *ident = store.defaultContainerIdentifier;
        if (ident == nil || ident.length == 0) {
            result.error = cstring_from_nsstring(@"no default container");
            return result;
        }
        result.identifier = cstring_from_nsstring(ident);
    }
    return result;
}

CContactListResult bridge_list_contacts_in_group(BridgeString groupID) {
    CContactListResult result;
    memset(&result, 0, sizeof(CContactListResult));

    @autoreleasepool {
        CNContactStore *store = [[CNContactStore alloc] init];
        NSString *gid = nsstring_from_cstring(groupID);
        NSError *error = nil;

        NSPredicate *predicate = [CNContact predicateForContactsInGroupWithIdentifier:gid];
        NSArray<CNContact *> *contacts = [store unifiedContactsMatchingPredicate:predicate keysToFetch:allContactKeys() error:&error];
        if (error != nil) {
            result.error = cstring_from_error(error);
            return result;
        }

        result.count = (int)contacts.count;
        if (result.count > 0) {
            result.contacts = (CContact *)malloc(sizeof(CContact) * result.count);
            for (int i = 0; i < result.count; i++) {
                result.contacts[i] = convert_contact(store, contacts[i], &error);
                if (error != nil) {
                    result.error = cstring_from_error(error);
                    for (int j = 0; j <= i; j++) {
                        bridge_free_contact(&result.contacts[j]);
                    }
                    free(result.contacts);
                    result.contacts = NULL;
                    result.count = 0;
                    return result;
                }
            }
        }
    }
    return result;
}

// --- Memory management ---

static void free_labeled_string(CLabeledString *ls) {
    free_cstring(&ls->identifier);
    free_cstring(&ls->label);
    free_cstring(&ls->value);
}

static void free_labeled_postal(CLabeledPostalAddress *lp) {
    free_cstring(&lp->identifier);
    free_cstring(&lp->label);
    free_cstring(&lp->value.street);
    free_cstring(&lp->value.city);
    free_cstring(&lp->value.state);
    free_cstring(&lp->value.postalCode);
    free_cstring(&lp->value.country);
    free_cstring(&lp->value.isoCountryCode);
}

static void free_labeled_relation(CLabeledContactRelation *lr) {
    free_cstring(&lr->identifier);
    free_cstring(&lr->label);
    free_cstring(&lr->value.name);
}

static void free_labeled_social(CLabeledSocialProfile *ls) {
    free_cstring(&ls->identifier);
    free_cstring(&ls->label);
    free_cstring(&ls->value.urlString);
    free_cstring(&ls->value.username);
    free_cstring(&ls->value.service);
}

static void free_labeled_im(CLabeledInstantMessage *li) {
    free_cstring(&li->identifier);
    free_cstring(&li->label);
    free_cstring(&li->value.instantUsername);
    free_cstring(&li->value.instantService);
}

static void free_labeled_date(CLabeledDateComponents *ld) {
    free_cstring(&ld->identifier);
    free_cstring(&ld->label);
}

void bridge_free_contact(CContact *contact) {
    if (contact == NULL) return;
    free_cstring(&contact->identifier);
    free_cstring(&contact->containerID);
    free_cstring(&contact->namePrefix);
    free_cstring(&contact->givenName);
    free_cstring(&contact->middleName);
    free_cstring(&contact->familyName);
    free_cstring(&contact->previousFamilyName);
    free_cstring(&contact->nameSuffix);
    free_cstring(&contact->nickname);
    free_cstring(&contact->phoneticGivenName);
    free_cstring(&contact->phoneticMiddleName);
    free_cstring(&contact->phoneticFamilyName);
    free_cstring(&contact->organizationName);
    free_cstring(&contact->departmentName);
    free_cstring(&contact->jobTitle);
    free_cstring(&contact->note);

    for (int i = 0; i < contact->phoneNumbersCount; i++) {
        free_labeled_string(&contact->phoneNumbers[i]);
    }
    if (contact->phoneNumbers) free(contact->phoneNumbers);

    for (int i = 0; i < contact->emailAddressesCount; i++) {
        free_labeled_string(&contact->emailAddresses[i]);
    }
    if (contact->emailAddresses) free(contact->emailAddresses);

    for (int i = 0; i < contact->postalAddressesCount; i++) {
        free_labeled_postal(&contact->postalAddresses[i]);
    }
    if (contact->postalAddresses) free(contact->postalAddresses);

    for (int i = 0; i < contact->urlAddressesCount; i++) {
        free_labeled_string(&contact->urlAddresses[i]);
    }
    if (contact->urlAddresses) free(contact->urlAddresses);

    for (int i = 0; i < contact->contactRelationsCount; i++) {
        free_labeled_relation(&contact->contactRelations[i]);
    }
    if (contact->contactRelations) free(contact->contactRelations);

    for (int i = 0; i < contact->socialProfilesCount; i++) {
        free_labeled_social(&contact->socialProfiles[i]);
    }
    if (contact->socialProfiles) free(contact->socialProfiles);

    for (int i = 0; i < contact->instantMessagesCount; i++) {
        free_labeled_im(&contact->instantMessages[i]);
    }
    if (contact->instantMessages) free(contact->instantMessages);

    for (int i = 0; i < contact->datesCount; i++) {
        free_labeled_date(&contact->dates[i]);
    }
    if (contact->dates) free(contact->dates);

    if (contact->imageData) free(contact->imageData);
    if (contact->thumbnailImageData) free(contact->thumbnailImageData);
}

void bridge_free_contact_list(CContact *contacts, int count) {
    if (contacts == NULL) return;
    for (int i = 0; i < count; i++) {
        bridge_free_contact(&contacts[i]);
    }
    free(contacts);
}

void bridge_free_group_list(CGroup *groups, int count) {
    if (groups == NULL) return;
    for (int i = 0; i < count; i++) {
        free_cstring(&groups[i].identifier);
        free_cstring(&groups[i].name);
        free_cstring(&groups[i].containerID);
        free_cstring(&groups[i].parentGroupID);
        for (int j = 0; j < groups[i].subgroupIDsCount; j++) {
            free_cstring(&groups[i].subgroupIDs[j]);
        }
        if (groups[i].subgroupIDs) free(groups[i].subgroupIDs);
    }
    free(groups);
}

void bridge_free_container_list(CContainer *containers, int count) {
    if (containers == NULL) return;
    for (int i = 0; i < count; i++) {
        free_cstring(&containers[i].identifier);
        free_cstring(&containers[i].name);
    }
    free(containers);
}