#ifndef CONTACTS_BRIDGE_H
#define CONTACTS_BRIDGE_H

#include <stdint.h>

// --- String helper ---

typedef struct {
    const char *str;
    int         len;
} BridgeString;

// --- DateComponents ---
typedef struct {
    int year;
    int month;
    int day;
} CDateComponents;

// --- LabeledString ---
typedef struct {
    BridgeString identifier;
    BridgeString label;
    BridgeString value;
} CLabeledString;

// --- PostalAddress ---
typedef struct {
    BridgeString street;
    BridgeString city;
    BridgeString state;
    BridgeString postalCode;
    BridgeString country;
    BridgeString isoCountryCode;
} CPostalAddress;

typedef struct {
    BridgeString        identifier;
    BridgeString        label;
    CPostalAddress value;
} CLabeledPostalAddress;

// --- ContactRelation ---
typedef struct {
    BridgeString name;
} CContactRelation;

typedef struct {
    BridgeString          identifier;
    BridgeString          label;
    CContactRelation value;
} CLabeledContactRelation;

// --- SocialProfile ---
typedef struct {
    BridgeString urlString;
    BridgeString username;
    BridgeString service;
} CSocialProfile;

typedef struct {
    BridgeString        identifier;
    BridgeString        label;
    CSocialProfile value;
} CLabeledSocialProfile;

// --- InstantMessage ---
typedef struct {
    BridgeString instantUsername;
    BridgeString instantService;
} CInstantMessage;

typedef struct {
    BridgeString         identifier;
    BridgeString         label;
    CInstantMessage value;
} CLabeledInstantMessage;

// --- DateComponents labeled ---
typedef struct {
    BridgeString         identifier;
    BridgeString         label;
    CDateComponents value;
} CLabeledDateComponents;

// --- Contact ---
typedef struct {
    BridgeString identifier;
    int     contactType;
    BridgeString namePrefix;
    BridgeString givenName;
    BridgeString middleName;
    BridgeString familyName;
    BridgeString previousFamilyName;
    BridgeString nameSuffix;
    BridgeString nickname;
    BridgeString phoneticGivenName;
    BridgeString phoneticMiddleName;
    BridgeString phoneticFamilyName;
    BridgeString organizationName;
    BridgeString departmentName;
    BridgeString jobTitle;
    BridgeString note;

    int             hasBirthday;
    CDateComponents birthday;

    CLabeledString          *phoneNumbers;
    int                      phoneNumbersCount;
    CLabeledString          *emailAddresses;
    int                      emailAddressesCount;
    CLabeledPostalAddress   *postalAddresses;
    int                      postalAddressesCount;
    CLabeledString          *urlAddresses;
    int                      urlAddressesCount;
    CLabeledContactRelation *contactRelations;
    int                      contactRelationsCount;
    CLabeledSocialProfile   *socialProfiles;
    int                      socialProfilesCount;
    CLabeledInstantMessage  *instantMessages;
    int                      instantMessagesCount;
    CLabeledDateComponents  *dates;
    int                      datesCount;

    int      imageDataAvailable;
    void    *imageData;
    int      imageDataLen;
    void    *thumbnailImageData;
    int      thumbnailImageDataLen;
} CContact;

// --- Filter ---
typedef struct {
    BridgeString fieldName;
    BridgeString value;
    int     op;  // 0=equals, 1=contains, 2=notContains
} CFilter;

// --- Group ---
typedef struct {
    BridgeString identifier;
    BridgeString name;
    BridgeString containerID;
    BridgeString parentGroupID;
    BridgeString *subgroupIDs;
    int      subgroupIDsCount;
} CGroup;

// --- Container ---
typedef struct {
    BridgeString identifier;
    BridgeString name;
    int     containerType;
} CContainer;

// --- Result types ---
typedef struct {
    CContact *contacts;
    int       count;
    BridgeString   error;
} CContactListResult;

typedef struct {
    CContact contact;
    BridgeString  error;
} CContactResult;

typedef struct {
    BridgeString identifier;
    BridgeString error;
} CCreateResult;

typedef struct {
    BridgeString error;
} CSimpleResult;

typedef struct {
    CGroup *groups;
    int     count;
    BridgeString error;
} CGroupListResult;

typedef struct {
    CContainer *containers;
    int         count;
    BridgeString     error;
} CContainerListResult;

typedef struct {
    CContainer container;
    BridgeString    error;
} CContainerResult;

typedef struct {
    int     status;
    BridgeString error;
} CAuthResult;

typedef struct {
    BridgeString identifier;
    BridgeString error;
} CDefaultContainerResult;

// --- Bridge functions ---
int              bridge_check_authorization(void);
CAuthResult      bridge_request_access(void);
CContactResult   bridge_get_contact(BridgeString identifier);
CContactListResult bridge_list_contacts(CFilter *filters, int filterCount);
CCreateResult    bridge_create_contact(CContact input);
CSimpleResult    bridge_delete_contact(BridgeString identifier);
CGroupListResult bridge_list_groups(BridgeString containerID);
CCreateResult    bridge_create_group(BridgeString name, BridgeString containerID, BridgeString parentGroupID);
CSimpleResult    bridge_delete_group(BridgeString identifier);
CSimpleResult    bridge_add_contact_to_group(BridgeString contactID, BridgeString groupID);
CSimpleResult    bridge_remove_contact_from_group(BridgeString contactID, BridgeString groupID);
CContainerResult bridge_get_container(BridgeString identifier);
CContainerListResult bridge_list_containers(void);
CDefaultContainerResult bridge_default_container_id(void);
CContactListResult bridge_list_contacts_in_group(BridgeString groupID);

// --- Memory management ---
void bridge_free_contact(CContact *contact);
void bridge_free_contact_list(CContact *contacts, int count);
void bridge_free_group_list(CGroup *groups, int count);
void bridge_free_container_list(CContainer *containers, int count);

#endif /* CONTACTS_BRIDGE_H */