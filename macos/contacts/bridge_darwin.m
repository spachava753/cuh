#import "bridge_darwin.h"

#import <Contacts/Contacts.h>
#import <Foundation/Foundation.h>
#import <dispatch/dispatch.h>

#include <stdlib.h>
#include <string.h>

static NSString *contacts_nsstring(const char *s) {
  if (s == NULL) {
    return nil;
  }
  return [NSString stringWithUTF8String:s];
}

static char *contacts_strdup_ns(NSString *s) {
  if (s == nil) {
    return NULL;
  }
  const char *utf8 = [s UTF8String];
  if (utf8 == NULL) {
    return NULL;
  }
  size_t n = strlen(utf8);
  char *out = (char *)malloc(n + 1);
  if (out == NULL) {
    return NULL;
  }
  memcpy(out, utf8, n + 1);
  return out;
}

static void contacts_reset_error(ContactsError *err) {
  if (err == NULL) {
    return;
  }
  err->code = CONTACTS_ERR_NONE;
  err->message = NULL;
}

void contacts_free_error(ContactsError *err) {
  if (err == NULL) {
    return;
  }
  if (err->message != NULL) {
    free(err->message);
    err->message = NULL;
  }
  err->code = CONTACTS_ERR_NONE;
}

static void contacts_set_error(ContactsError *err, int code, NSString *message) {
  if (err == NULL) {
    return;
  }
  contacts_free_error(err);
  err->code = code;
  err->message = contacts_strdup_ns(message ?: @"");
}

static int contacts_map_nserror_code(NSError *error) {
  if (error == nil) {
    return CONTACTS_ERR_UNKNOWN;
  }
  if (![error.domain isEqualToString:CNErrorDomain]) {
    return CONTACTS_ERR_STORE;
  }

  switch (error.code) {
#ifdef CNErrorCodeAuthorizationDenied
    case CNErrorCodeAuthorizationDenied:
      return CONTACTS_ERR_PERMISSION_DENIED;
#endif
#ifdef CNErrorCodeRecordDoesNotExist
    case CNErrorCodeRecordDoesNotExist:
      return CONTACTS_ERR_NOT_FOUND;
#endif
#ifdef CNErrorCodeValidationError
    case CNErrorCodeValidationError:
      return CONTACTS_ERR_VALIDATION;
#endif
#ifdef CNErrorCodeValidationMultipleErrors
    case CNErrorCodeValidationMultipleErrors:
      return CONTACTS_ERR_VALIDATION;
#endif
#ifdef CNErrorCodeNoAccessibleWritableContainers
    case CNErrorCodeNoAccessibleWritableContainers:
      return CONTACTS_ERR_CONFLICT;
#endif
#ifdef CNErrorCodeParentContainerNotWritable
    case CNErrorCodeParentContainerNotWritable:
      return CONTACTS_ERR_CONFLICT;
#endif
#ifdef CNErrorCodePolicyViolation
    case CNErrorCodePolicyViolation:
      return CONTACTS_ERR_VALIDATION;
#endif
#ifdef CNErrorCodeUnauthorizedKeys
    case CNErrorCodeUnauthorizedKeys:
      return CONTACTS_ERR_PERMISSION_DENIED;
#endif
    default:
      break;
  }

  return CONTACTS_ERR_STORE;
}

static NSString *contacts_string_or_empty(NSString *s) {
  return s ?: @"";
}

static NSString *contacts_escape_applescript_string(NSString *value) {
  NSString *escaped = contacts_string_or_empty(value);
  escaped = [escaped stringByReplacingOccurrencesOfString:@"\\" withString:@"\\\\"];
  escaped = [escaped stringByReplacingOccurrencesOfString:@"\"" withString:@"\\\""];
  return escaped;
}

static BOOL contacts_run_applescript_source(NSString *source, NSString **errorMessage) {
  NSDictionary *scriptError = nil;
  NSAppleScript *appleScript = [[NSAppleScript alloc] initWithSource:source];
  NSAppleEventDescriptor *result = [appleScript executeAndReturnError:&scriptError];
  if (result != nil) {
    return YES;
  }

  NSString *message = scriptError[NSAppleScriptErrorMessage];
  if (message.length == 0) {
    NSNumber *code = scriptError[NSAppleScriptErrorNumber];
    if (code != nil) {
      message = [NSString stringWithFormat:@"AppleScript error %@", code];
    }
  }
  if (message.length == 0) {
    message = @"AppleScript execution failed";
  }
  if (errorMessage != NULL) {
    *errorMessage = message;
  }
  return NO;
}

// We intentionally use AppleScript for membership removal because
// removeMember:fromGroup: can report success without persisting removal on some
// macOS Contacts backends. The Contacts app scripting path has been more
// reliable for this operation; we still verify persisted state afterward.
static BOOL contacts_remove_membership_via_applescript(NSString *groupID,
                                                        NSString *groupName,
                                                        NSString *contactID,
                                                        NSString *givenName,
                                                        NSString *familyName,
                                                        NSString **errorMessage) {
  if (contactID.length == 0) {
    if (errorMessage != NULL) {
      *errorMessage = @"missing contact identifier";
    }
    return NO;
  }

  NSString *escapedContactID = contacts_escape_applescript_string(contactID);
  NSString *escapedGroupID = contacts_escape_applescript_string(groupID);
  NSString *escapedGroupName = contacts_escape_applescript_string(groupName);

  if (escapedGroupID.length > 0) {
    NSString *byIDs = [NSString stringWithFormat:
      @"tell application \"Contacts\"\n"
       "set targetGroup to first group whose id is \"%@\"\n"
       "set targetPerson to person id \"%@\"\n"
       "remove targetPerson from targetGroup\n"
       "save\n"
       "end tell\n",
      escapedGroupID,
      escapedContactID];
    if (contacts_run_applescript_source(byIDs, errorMessage)) {
      return YES;
    }
  }

  if (escapedGroupName.length > 0) {
    NSString *byGroupNameAndID = [NSString stringWithFormat:
      @"tell application \"Contacts\"\n"
       "set targetGroup to first group whose name is \"%@\"\n"
       "set targetPerson to person id \"%@\"\n"
       "remove targetPerson from targetGroup\n"
       "save\n"
       "end tell\n",
      escapedGroupName,
      escapedContactID];
    if (contacts_run_applescript_source(byGroupNameAndID, errorMessage)) {
      return YES;
    }
  }

  if (escapedGroupName.length > 0 && (givenName.length > 0 || familyName.length > 0)) {
    NSString *selector = nil;
    if (givenName.length > 0 && familyName.length > 0) {
      selector = [NSString stringWithFormat:@"first person whose first name is \"%@\" and last name is \"%@\"",
                                          contacts_escape_applescript_string(givenName),
                                          contacts_escape_applescript_string(familyName)];
    } else if (givenName.length > 0) {
      selector = [NSString stringWithFormat:@"first person whose first name is \"%@\"",
                                          contacts_escape_applescript_string(givenName)];
    } else {
      selector = [NSString stringWithFormat:@"first person whose last name is \"%@\"",
                                          contacts_escape_applescript_string(familyName)];
    }

    NSString *byGroupAndName = [NSString stringWithFormat:
      @"tell application \"Contacts\"\n"
       "set targetGroup to first group whose name is \"%@\"\n"
       "set targetPerson to %@\n"
       "remove targetPerson from targetGroup\n"
       "save\n"
       "end tell\n",
      escapedGroupName,
      selector];
    if (contacts_run_applescript_source(byGroupAndName, errorMessage)) {
      return YES;
    }
  }

  return NO;
}

static void contacts_free_ref(ContactsRef *ref) {
  if (ref == NULL) {
    return;
  }
  if (ref->id != NULL) {
    free(ref->id);
    ref->id = NULL;
  }
  if (ref->container_id != NULL) {
    free(ref->container_id);
    ref->container_id = NULL;
  }
  if (ref->account_id != NULL) {
    free(ref->account_id);
    ref->account_id = NULL;
  }
}

static void contacts_free_labeled_values(ContactsLabeledValue *values, int len) {
  if (values == NULL) {
    return;
  }
  for (int i = 0; i < len; i++) {
    if (values[i].label != NULL) {
      free(values[i].label);
      values[i].label = NULL;
    }
    if (values[i].value != NULL) {
      free(values[i].value);
      values[i].value = NULL;
    }
  }
  free(values);
}

static void contacts_fill_ref_info(CNContactStore *store, NSString *contactID, ContactsRef *outRef) {
  if (outRef == NULL) {
    return;
  }
  outRef->id = contacts_strdup_ns(contactID ?: @"");
  outRef->container_id = NULL;
  outRef->account_id = NULL;

  if (store == nil || contactID.length == 0) {
    return;
  }

  NSError *err = nil;
  NSArray *containers = [store containersMatchingPredicate:[CNContainer predicateForContainerOfContactWithIdentifier:contactID]
                                                     error:&err];
  if (containers.count == 0 || err != nil) {
    return;
  }

  CNContainer *container = containers.firstObject;
  outRef->container_id = contacts_strdup_ns(container.identifier ?: @"");
  // Contacts.framework does not expose a stable account identifier for all backends.
  // Use container identifier as a stable account-scoped identifier in v1.
  outRef->account_id = contacts_strdup_ns(container.identifier ?: @"");
}

static NSString *contacts_compose_display_name(CNContact *contact) {
  if (contact == nil) {
    return @"";
  }
  NSMutableArray *parts = [NSMutableArray arrayWithCapacity:2];
  if (contact.givenName.length > 0) {
    [parts addObject:contact.givenName];
  }
  if (contact.familyName.length > 0) {
    [parts addObject:contact.familyName];
  }
  if (parts.count == 0 && contact.organizationName.length > 0) {
    return contact.organizationName;
  }
  return [parts componentsJoinedByString:@" "];
}

static BOOL contacts_string_contains(NSString *haystack, NSString *needle) {
  if (needle.length == 0) {
    return YES;
  }
  if (haystack.length == 0) {
    return NO;
  }
  NSRange r = [haystack rangeOfString:needle options:(NSCaseInsensitiveSearch | NSDiacriticInsensitiveSearch)];
  return r.location != NSNotFound;
}

static NSArray *contacts_strings_from_c_array(char **items, int len) {
  if (items == NULL || len <= 0) {
    return @[];
  }
  NSMutableArray *out = [NSMutableArray arrayWithCapacity:(NSUInteger)len];
  for (int i = 0; i < len; i++) {
    NSString *s = contacts_nsstring(items[i]);
    if (s.length > 0) {
      [out addObject:s];
    }
  }
  return out;
}

static NSSet *contacts_group_member_set(CNContactStore *store, char **groupIDs, int groupIDsLen, NSError **errorOut) {
  if (groupIDs == NULL || groupIDsLen == 0) {
    return nil;
  }

  NSMutableSet *ids = [NSMutableSet set];
  for (int i = 0; i < groupIDsLen; i++) {
    NSString *groupID = contacts_nsstring(groupIDs[i]);
    if (groupID.length == 0) {
      continue;
    }
    NSError *groupErr = nil;
    NSArray *members = [store unifiedContactsMatchingPredicate:[CNContact predicateForContactsInGroupWithIdentifier:groupID]
                                                    keysToFetch:@[CNContactIdentifierKey]
                                                          error:&groupErr];
    if (groupErr != nil) {
      if (errorOut != NULL) {
        *errorOut = groupErr;
      }
      return nil;
    }
    for (CNContact *member in members) {
      if (member.identifier.length > 0) {
        [ids addObject:member.identifier];
      }
    }
  }
  return ids;
}

static BOOL contacts_contact_matches(CNContact *contact,
                                     const ContactsFindRequest *req,
                                     NSSet *groupMemberIDs,
                                     NSSet *idSet) {
  int clauses = 0;
  int matched = 0;

  NSString *nameNeedle = contacts_nsstring(req->name_contains);
  if (nameNeedle.length > 0) {
    clauses++;
    NSMutableArray *parts = [NSMutableArray arrayWithCapacity:4];
    if (contact.givenName.length > 0) {
      [parts addObject:contact.givenName];
    }
    if (contact.middleName.length > 0) {
      [parts addObject:contact.middleName];
    }
    if (contact.familyName.length > 0) {
      [parts addObject:contact.familyName];
    }
    if (contact.nickname.length > 0) {
      [parts addObject:contact.nickname];
    }
    NSString *joined = [parts componentsJoinedByString:@" "];
    if (contacts_string_contains(joined, nameNeedle)) {
      matched++;
    }
  }

  NSString *orgNeedle = contacts_nsstring(req->organization_contains);
  if (orgNeedle.length > 0) {
    clauses++;
    if (contacts_string_contains(contact.organizationName, orgNeedle)) {
      matched++;
    }
  }

  NSString *emailDomain = contacts_nsstring(req->email_domain);
  if (emailDomain.length > 0) {
    clauses++;
    NSString *needle = [emailDomain lowercaseString];
    BOOL domainMatch = NO;
    for (CNLabeledValue *lv in contact.emailAddresses) {
      NSString *email = [[lv value] lowercaseString];
      NSRange at = [email rangeOfString:@"@" options:NSBackwardsSearch];
      if (at.location == NSNotFound) {
        continue;
      }
      NSString *domain = [email substringFromIndex:(at.location + 1)];
      if ([domain isEqualToString:needle]) {
        domainMatch = YES;
        break;
      }
    }
    if (domainMatch) {
      matched++;
    }
  }


  if (idSet != nil && idSet.count > 0) {
    clauses++;
    if ([idSet containsObject:contact.identifier]) {
      matched++;
    }
  }

  if (groupMemberIDs != nil && groupMemberIDs.count > 0) {
    clauses++;
    if ([groupMemberIDs containsObject:contact.identifier]) {
      matched++;
    }
  }

  if (clauses == 0) {
    return YES;
  }

  if (req->match_policy == CONTACTS_MATCH_ANY) {
    return matched > 0;
  }
  return matched == clauses;
}

static NSArray *contacts_find_keys(const ContactsFindRequest *req) {
  NSMutableArray *keys = [NSMutableArray arrayWithCapacity:8];
  [keys addObject:CNContactIdentifierKey];
  [keys addObject:CNContactGivenNameKey];
  [keys addObject:CNContactMiddleNameKey];
  [keys addObject:CNContactFamilyNameKey];
  [keys addObject:CNContactNicknameKey];

  if (req->organization_contains != NULL || req->include_meta != 0) {
    [keys addObject:CNContactOrganizationNameKey];
  }
  if (req->email_domain != NULL) {
    [keys addObject:CNContactEmailAddressesKey];
  }

  return keys;
}

static int contacts_sort_compare(CNContact *a, CNContact *b, int sortBy, int sortOrder) {
  NSString *leftPrimary = @"";
  NSString *rightPrimary = @"";
  NSString *leftSecondary = @"";
  NSString *rightSecondary = @"";

  if (sortBy == CONTACTS_SORT_FAMILY_NAME) {
    leftPrimary = [a.familyName lowercaseString];
    rightPrimary = [b.familyName lowercaseString];
    leftSecondary = [a.givenName lowercaseString];
    rightSecondary = [b.givenName lowercaseString];
  } else {
    leftPrimary = [a.givenName lowercaseString];
    rightPrimary = [b.givenName lowercaseString];
    leftSecondary = [a.familyName lowercaseString];
    rightSecondary = [b.familyName lowercaseString];
  }

  NSComparisonResult primary = [leftPrimary compare:rightPrimary options:NSCaseInsensitiveSearch];
  NSComparisonResult secondary = [leftSecondary compare:rightSecondary options:NSCaseInsensitiveSearch];

  int out = 0;
  if (primary == NSOrderedAscending) {
    out = -1;
  } else if (primary == NSOrderedDescending) {
    out = 1;
  } else if (secondary == NSOrderedAscending) {
    out = -1;
  } else if (secondary == NSOrderedDescending) {
    out = 1;
  }

  if (sortOrder == CONTACTS_SORT_DESC) {
    out *= -1;
  }
  return out;
}

int contacts_authorization_status(void) {
  CNAuthorizationStatus status = [CNContactStore authorizationStatusForEntityType:CNEntityTypeContacts];
  switch (status) {
    case CNAuthorizationStatusAuthorized:
      return CONTACTS_AUTH_AUTHORIZED;
    case CNAuthorizationStatusDenied:
      return CONTACTS_AUTH_DENIED;
    case CNAuthorizationStatusRestricted:
      return CONTACTS_AUTH_RESTRICTED;
    case CNAuthorizationStatusNotDetermined:
    default:
      return CONTACTS_AUTH_NOT_DETERMINED;
  }
}

int contacts_request_access(ContactsError *err) {
  contacts_reset_error(err);

  CNContactStore *store = [[CNContactStore alloc] init];
  __block BOOL granted = NO;
  __block NSError *requestError = nil;

  dispatch_semaphore_t sem = dispatch_semaphore_create(0);
  [store requestAccessForEntityType:CNEntityTypeContacts
                  completionHandler:^(BOOL accessGranted, NSError *_Nullable accessError) {
                    granted = accessGranted;
                    requestError = accessError;
                    dispatch_semaphore_signal(sem);
                  }];
  dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);

  if (!granted) {
    NSString *msg = requestError.localizedDescription;
    if (msg.length == 0) {
      msg = @"contacts access denied";
    }
    contacts_set_error(err, CONTACTS_ERR_PERMISSION_DENIED, msg);
    return 0;
  }
  return 1;
}

int contacts_find(const ContactsFindRequest *req, ContactsFindResult *out, ContactsError *err) {
  contacts_reset_error(err);
  if (out == NULL || req == NULL) {
    contacts_set_error(err, CONTACTS_ERR_VALIDATION, @"invalid find request");
    return 0;
  }

  out->items = NULL;
  out->items_len = 0;
  out->next_offset = -1;

  if (contacts_authorization_status() != CONTACTS_AUTH_AUTHORIZED) {
    contacts_set_error(err, CONTACTS_ERR_PERMISSION_DENIED, @"contacts access not authorized");
    return 0;
  }

  CNContactStore *store = [[CNContactStore alloc] init];

  NSError *filterErr = nil;
  NSSet *groupMemberIDs = contacts_group_member_set(store, req->group_ids_any, req->group_ids_any_len, &filterErr);
  if (filterErr != nil) {
    contacts_set_error(err, contacts_map_nserror_code(filterErr), filterErr.localizedDescription);
    return 0;
  }

  NSSet *idSet = nil;
  if (req->ids_len > 0) {
    idSet = [NSSet setWithArray:contacts_strings_from_c_array(req->ids, req->ids_len)];
  }

  NSArray *keys = contacts_find_keys(req);
  CNContactFetchRequest *fetchReq = [[CNContactFetchRequest alloc] initWithKeysToFetch:keys];

  NSMutableArray *matches = [NSMutableArray array];
  NSError *fetchErr = nil;
  BOOL ok = [store enumerateContactsWithFetchRequest:fetchReq
                                               error:&fetchErr
                                          usingBlock:^(CNContact *contact, BOOL *stop) {
                                            if (contacts_contact_matches(contact, req, groupMemberIDs, idSet)) {
                                              [matches addObject:contact];
                                            }
                                          }];
  if (!ok || fetchErr != nil) {
    contacts_set_error(err, contacts_map_nserror_code(fetchErr), fetchErr.localizedDescription);
    return 0;
  }

  [matches sortUsingComparator:^NSComparisonResult(CNContact *a, CNContact *b) {
    int cmp = contacts_sort_compare(a, b, req->sort_by, req->sort_order);
    if (cmp < 0) {
      return NSOrderedAscending;
    }
    if (cmp > 0) {
      return NSOrderedDescending;
    }
    return NSOrderedSame;
  }];

  int offset = req->offset;
  if (offset < 0) {
    offset = 0;
  }
  int limit = req->limit;
  if (limit <= 0) {
    limit = 50;
  }

  int total = (int)matches.count;
  if (offset >= total) {
    out->items = NULL;
    out->items_len = 0;
    out->next_offset = -1;
    return 1;
  }

  int end = offset + limit;
  if (end > total) {
    end = total;
  }

  int count = end - offset;
  out->items = (ContactsFoundRef *)calloc((size_t)count, sizeof(ContactsFoundRef));
  out->items_len = count;
  out->next_offset = (end < total) ? end : -1;

  for (int i = 0; i < count; i++) {
    CNContact *contact = matches[(NSUInteger)(offset + i)];
    ContactsFoundRef *dst = &out->items[i];
    dst->id = contacts_strdup_ns(contact.identifier ?: @"");

    ContactsRef ref = {0};
    contacts_fill_ref_info(store, contact.identifier, &ref);
    dst->container_id = ref.container_id;
    dst->account_id = ref.account_id;
    if (ref.id != NULL) {
      free(ref.id);
    }

    NSString *displayName = contacts_compose_display_name(contact);
    dst->display_name = contacts_strdup_ns(displayName);
    if ([contact isKeyAvailable:CNContactOrganizationNameKey]) {
      dst->organization = contacts_strdup_ns(contacts_string_or_empty(contact.organizationName));
    } else {
      dst->organization = contacts_strdup_ns(@"");
    }
    dst->modified_at_unix = 0;
  }

  return 1;
}

static NSDictionary *contacts_group_memberships_for_ids(CNContactStore *store, NSSet *ids, NSError **errorOut) {
  if (ids == nil || ids.count == 0) {
    return @{};
  }

  NSError *groupErr = nil;
  NSArray *groups = [store groupsMatchingPredicate:nil error:&groupErr];
  if (groupErr != nil) {
    if (errorOut != NULL) {
      *errorOut = groupErr;
    }
    return nil;
  }

  NSMutableDictionary *result = [NSMutableDictionary dictionaryWithCapacity:ids.count];
  for (NSString *contactID in ids) {
    result[contactID] = [NSMutableArray array];
  }

  for (CNGroup *group in groups) {
    NSError *membersErr = nil;
    NSArray *members = [store unifiedContactsMatchingPredicate:[CNContact predicateForContactsInGroupWithIdentifier:group.identifier]
                                                    keysToFetch:@[CNContactIdentifierKey]
                                                          error:&membersErr];
    if (membersErr != nil) {
      if (errorOut != NULL) {
        *errorOut = membersErr;
      }
      return nil;
    }

    for (CNContact *member in members) {
      NSMutableArray *groupIDs = result[member.identifier];
      if (groupIDs != nil && group.identifier.length > 0) {
        [groupIDs addObject:group.identifier];
      }
    }
  }

  return result;
}

static NSArray *contacts_get_keys(uint32_t fieldMask) {
  NSMutableArray *keys = [NSMutableArray arrayWithCapacity:10];
  [keys addObject:CNContactIdentifierKey];

  BOOL includeAll = (fieldMask == 0);
  if (includeAll || (fieldMask & CONTACTS_FIELD_NAMES) != 0) {
    [keys addObject:CNContactGivenNameKey];
    [keys addObject:CNContactMiddleNameKey];
    [keys addObject:CNContactFamilyNameKey];
    [keys addObject:CNContactNicknameKey];
  }
  if (includeAll || (fieldMask & CONTACTS_FIELD_ORGANIZATION) != 0) {
    [keys addObject:CNContactOrganizationNameKey];
    [keys addObject:CNContactJobTitleKey];
  }
  if (includeAll || (fieldMask & CONTACTS_FIELD_EMAILS) != 0) {
    [keys addObject:CNContactEmailAddressesKey];
  }
  if (includeAll || (fieldMask & CONTACTS_FIELD_PHONES) != 0) {
    [keys addObject:CNContactPhoneNumbersKey];
  }
  return keys;
}

static void contacts_fill_labeled_email_values(NSArray *values, ContactsLabeledValue **outValues, int *outLen) {
  if (outValues == NULL || outLen == NULL) {
    return;
  }
  *outValues = NULL;
  *outLen = 0;
  if (values.count == 0) {
    return;
  }

  ContactsLabeledValue *result = (ContactsLabeledValue *)calloc((size_t)values.count, sizeof(ContactsLabeledValue));
  for (NSUInteger i = 0; i < values.count; i++) {
    CNLabeledValue *lv = values[i];
    NSString *label = lv.label;
    if (label.length > 0) {
      label = [CNLabeledValue localizedStringForLabel:label] ?: label;
    }
    result[i].label = contacts_strdup_ns(label ?: @"");
    NSString *value = [lv value];
    result[i].value = contacts_strdup_ns(value ?: @"");
  }

  *outValues = result;
  *outLen = (int)values.count;
}

static void contacts_fill_labeled_phone_values(NSArray *values, ContactsLabeledValue **outValues, int *outLen) {
  if (outValues == NULL || outLen == NULL) {
    return;
  }
  *outValues = NULL;
  *outLen = 0;
  if (values.count == 0) {
    return;
  }

  ContactsLabeledValue *result = (ContactsLabeledValue *)calloc((size_t)values.count, sizeof(ContactsLabeledValue));
  for (NSUInteger i = 0; i < values.count; i++) {
    CNLabeledValue *lv = values[i];
    NSString *label = lv.label;
    if (label.length > 0) {
      label = [CNLabeledValue localizedStringForLabel:label] ?: label;
    }
    result[i].label = contacts_strdup_ns(label ?: @"");

    CNPhoneNumber *number = [lv value];
    result[i].value = contacts_strdup_ns(number.stringValue ?: @"");
  }

  *outValues = result;
  *outLen = (int)values.count;
}

int contacts_get(const ContactsGetRequest *req, ContactsGetResult *out, ContactsError *err) {
  contacts_reset_error(err);
  if (req == NULL || out == NULL) {
    contacts_set_error(err, CONTACTS_ERR_VALIDATION, @"invalid get request");
    return 0;
  }

  out->items = NULL;
  out->items_len = 0;

  if (contacts_authorization_status() != CONTACTS_AUTH_AUTHORIZED) {
    contacts_set_error(err, CONTACTS_ERR_PERMISSION_DENIED, @"contacts access not authorized");
    return 0;
  }

  if (req->refs_len <= 0 || req->refs == NULL) {
    return 1;
  }

  NSMutableArray *ids = [NSMutableArray arrayWithCapacity:(NSUInteger)req->refs_len];
  for (int i = 0; i < req->refs_len; i++) {
    NSString *id = contacts_nsstring(req->refs[i].id);
    if (id.length > 0) {
      [ids addObject:id];
    }
  }
  if (ids.count == 0) {
    return 1;
  }

  CNContactStore *store = [[CNContactStore alloc] init];
  NSError *fetchErr = nil;
  NSArray *contacts = [store unifiedContactsMatchingPredicate:[CNContact predicateForContactsWithIdentifiers:ids]
                                                   keysToFetch:contacts_get_keys(req->field_mask)
                                                         error:&fetchErr];
  if (fetchErr != nil) {
    contacts_set_error(err, contacts_map_nserror_code(fetchErr), fetchErr.localizedDescription);
    return 0;
  }

  NSSet *idSet = [NSSet setWithArray:ids];
  NSDictionary *groupMap = @{};
  if (req->field_mask == 0 || (req->field_mask & CONTACTS_FIELD_GROUPS) != 0) {
    NSError *groupErr = nil;
    groupMap = contacts_group_memberships_for_ids(store, idSet, &groupErr);
    if (groupErr != nil) {
      contacts_set_error(err, contacts_map_nserror_code(groupErr), groupErr.localizedDescription);
      return 0;
    }
  }

  out->items = (ContactsContact *)calloc((size_t)contacts.count, sizeof(ContactsContact));
  out->items_len = (int)contacts.count;

  for (NSUInteger i = 0; i < contacts.count; i++) {
    CNContact *src = contacts[i];
    ContactsContact *dst = &out->items[i];

    contacts_fill_ref_info(store, src.identifier, &dst->ref);

    if ([src isKeyAvailable:CNContactGivenNameKey]) {
      dst->given_name = contacts_strdup_ns(contacts_string_or_empty(src.givenName));
    } else {
      dst->given_name = contacts_strdup_ns(@"");
    }
    if ([src isKeyAvailable:CNContactFamilyNameKey]) {
      dst->family_name = contacts_strdup_ns(contacts_string_or_empty(src.familyName));
    } else {
      dst->family_name = contacts_strdup_ns(@"");
    }
    if ([src isKeyAvailable:CNContactMiddleNameKey]) {
      dst->middle_name = contacts_strdup_ns(contacts_string_or_empty(src.middleName));
    } else {
      dst->middle_name = contacts_strdup_ns(@"");
    }
    if ([src isKeyAvailable:CNContactNicknameKey]) {
      dst->nickname = contacts_strdup_ns(contacts_string_or_empty(src.nickname));
    } else {
      dst->nickname = contacts_strdup_ns(@"");
    }
    if ([src isKeyAvailable:CNContactOrganizationNameKey]) {
      dst->organization = contacts_strdup_ns(contacts_string_or_empty(src.organizationName));
    } else {
      dst->organization = contacts_strdup_ns(@"");
    }
    if ([src isKeyAvailable:CNContactJobTitleKey]) {
      dst->job_title = contacts_strdup_ns(contacts_string_or_empty(src.jobTitle));
    } else {
      dst->job_title = contacts_strdup_ns(@"");
    }
    dst->modified_at_unix = 0;

    if ([src isKeyAvailable:CNContactEmailAddressesKey]) {
      contacts_fill_labeled_email_values(src.emailAddresses, &dst->emails, &dst->emails_len);
    }
    if ([src isKeyAvailable:CNContactPhoneNumbersKey]) {
      contacts_fill_labeled_phone_values(src.phoneNumbers, &dst->phones, &dst->phones_len);
    }

    NSArray *groupIDs = groupMap[src.identifier];
    if (groupIDs.count > 0) {
      dst->group_ids = (char **)calloc(groupIDs.count, sizeof(char *));
      dst->group_ids_len = (int)groupIDs.count;
      for (NSUInteger gi = 0; gi < groupIDs.count; gi++) {
        dst->group_ids[gi] = contacts_strdup_ns(groupIDs[gi]);
      }
    }
  }

  return 1;
}

static NSArray *contacts_emails_from_c(const ContactsLabeledValue *values, int len) {
  if (values == NULL || len <= 0) {
    return @[];
  }
  NSMutableArray *out = [NSMutableArray arrayWithCapacity:(NSUInteger)len];
  for (int i = 0; i < len; i++) {
    NSString *value = contacts_nsstring(values[i].value);
    if (value.length == 0) {
      continue;
    }
    NSString *label = contacts_nsstring(values[i].label);
    if (label.length == 0) {
      label = CNLabelOther;
    }
    [out addObject:[CNLabeledValue labeledValueWithLabel:label value:value]];
  }
  return out;
}

static NSArray *contacts_phones_from_c(const ContactsLabeledValue *values, int len) {
  if (values == NULL || len <= 0) {
    return @[];
  }
  NSMutableArray *out = [NSMutableArray arrayWithCapacity:(NSUInteger)len];
  for (int i = 0; i < len; i++) {
    NSString *number = contacts_nsstring(values[i].value);
    if (number.length == 0) {
      continue;
    }
    NSString *label = contacts_nsstring(values[i].label);
    if (label.length == 0) {
      label = CNLabelPhoneNumberMobile;
    }
    CNPhoneNumber *pn = [CNPhoneNumber phoneNumberWithStringValue:number];
    [out addObject:[CNLabeledValue labeledValueWithLabel:label value:pn]];
  }
  return out;
}

static CNGroup *contacts_group_by_id(CNContactStore *store, NSString *groupID, NSError **errorOut) {
  if (groupID.length == 0) {
    return nil;
  }
  NSError *err = nil;
  NSArray *groups = [store groupsMatchingPredicate:[CNGroup predicateForGroupsWithIdentifiers:@[groupID]] error:&err];
  if (errorOut != NULL) {
    *errorOut = err;
  }
  if (err != nil || groups.count == 0) {
    return nil;
  }
  return groups.firstObject;
}

static CNContact *contacts_contact_by_id(CNContactStore *store,
                                         NSString *contactID,
                                         NSArray *keys,
                                         NSError **errorOut) {
  if (contactID.length == 0) {
    return nil;
  }
  NSError *err = nil;
  NSArray *contacts = [store unifiedContactsMatchingPredicate:[CNContact predicateForContactsWithIdentifiers:@[contactID]]
                                                   keysToFetch:keys
                                                         error:&err];
  if (errorOut != NULL) {
    *errorOut = err;
  }
  if (err != nil || contacts.count == 0) {
    return nil;
  }
  return contacts.firstObject;
}

static CNContact *contacts_group_member_contact(CNContactStore *store,
                                                NSString *groupID,
                                                NSString *contactID,
                                                NSError **errorOut) {
  if (groupID.length == 0 || contactID.length == 0) {
    return nil;
  }

  NSError *err = nil;
  NSArray *members = [store unifiedContactsMatchingPredicate:[CNContact predicateForContactsInGroupWithIdentifier:groupID]
                                                  keysToFetch:@[CNContactIdentifierKey]
                                                        error:&err];
  if (errorOut != NULL) {
    *errorOut = err;
  }
  if (err != nil) {
    return nil;
  }

  for (CNContact *member in members) {
    if ([member.identifier isEqualToString:contactID]) {
      return member;
    }
  }
  return nil;
}

static BOOL contacts_group_contains_contact(CNContactStore *store,
                                            NSString *groupID,
                                            NSString *contactID,
                                            NSError **errorOut) {
  if (groupID.length == 0 || contactID.length == 0) {
    return NO;
  }

  NSError *err = nil;
  NSArray *members = [store unifiedContactsMatchingPredicate:[CNContact predicateForContactsInGroupWithIdentifier:groupID]
                                                  keysToFetch:@[CNContactIdentifierKey]
                                                        error:&err];
  if (errorOut != NULL) {
    *errorOut = err;
  }
  if (err != nil) {
    return NO;
  }

  for (CNContact *member in members) {
    if ([member.identifier isEqualToString:contactID]) {
      return YES;
    }
  }
  return NO;
}

static BOOL contacts_verify_membership_state(CNContactStore *store,
                                             NSString *contactID,
                                             NSArray *mustHaveGroups,
                                             NSArray *mustNotHaveGroups,
                                             int *errorCode,
                                             NSString **errorMessage) {
  if (errorCode != NULL) {
    *errorCode = CONTACTS_ERR_NONE;
  }
  if (errorMessage != NULL) {
    *errorMessage = nil;
  }

  for (NSString *groupID in mustHaveGroups) {
    NSError *err = nil;
    BOOL hasMember = contacts_group_contains_contact(store, groupID, contactID, &err);
    if (err != nil) {
      if (errorCode != NULL) {
        *errorCode = contacts_map_nserror_code(err);
      }
      if (errorMessage != NULL) {
        *errorMessage = err.localizedDescription;
      }
      return NO;
    }
    if (!hasMember) {
      if (errorCode != NULL) {
        *errorCode = CONTACTS_ERR_CONFLICT;
      }
      if (errorMessage != NULL) {
        *errorMessage = [NSString stringWithFormat:@"membership add did not persist for group %@", groupID];
      }
      return NO;
    }
  }

  for (NSString *groupID in mustNotHaveGroups) {
    NSError *err = nil;
    BOOL hasMember = contacts_group_contains_contact(store, groupID, contactID, &err);
    if (err != nil) {
      if (errorCode != NULL) {
        *errorCode = contacts_map_nserror_code(err);
      }
      if (errorMessage != NULL) {
        *errorMessage = err.localizedDescription;
      }
      return NO;
    }
    if (hasMember) {
      if (errorCode != NULL) {
        *errorCode = CONTACTS_ERR_CONFLICT;
      }
      if (errorMessage != NULL) {
        *errorMessage = [NSString stringWithFormat:@"membership remove did not persist for group %@", groupID];
      }
      return NO;
    }
  }

  return YES;
}

static ContactsWriteResult contacts_make_error_write_result(ContactsRef ref, int code, NSString *message) {
  ContactsWriteResult out = {0};
  out.succeeded = 0;
  out.created = 0;
  out.updated = 0;
  out.error_code = code;
  out.error_message = contacts_strdup_ns(message ?: @"");

  out.ref.id = contacts_strdup_ns(contacts_nsstring(ref.id) ?: @"");
  out.ref.container_id = contacts_strdup_ns(contacts_nsstring(ref.container_id) ?: @"");
  out.ref.account_id = contacts_strdup_ns(contacts_nsstring(ref.account_id) ?: @"");
  return out;
}

static ContactsWriteResult contacts_make_success_result_for_contact(CNContactStore *store,
                                                                    NSString *contactID,
                                                                    int created,
                                                                    int updated) {
  ContactsWriteResult out = {0};
  out.succeeded = 1;
  out.created = created;
  out.updated = updated;
  out.error_code = CONTACTS_ERR_NONE;
  out.error_message = NULL;
  contacts_fill_ref_info(store, contactID, &out.ref);
  return out;
}


static ContactsWriteResult contacts_apply_create(CNContactStore *store, const ContactsDraft *draft) {
  ContactsWriteResult result = {0};


  CNMutableContact *contact = [[CNMutableContact alloc] init];
  contact.givenName = contacts_string_or_empty(contacts_nsstring(draft->given_name));
  contact.familyName = contacts_string_or_empty(contacts_nsstring(draft->family_name));
  contact.middleName = contacts_string_or_empty(contacts_nsstring(draft->middle_name));
  contact.nickname = contacts_string_or_empty(contacts_nsstring(draft->nickname));
  contact.organizationName = contacts_string_or_empty(contacts_nsstring(draft->organization));
  contact.jobTitle = contacts_string_or_empty(contacts_nsstring(draft->job_title));
  contact.emailAddresses = contacts_emails_from_c(draft->emails, draft->emails_len);
  contact.phoneNumbers = contacts_phones_from_c(draft->phones, draft->phones_len);

  CNSaveRequest *save = [[CNSaveRequest alloc] init];
  NSString *containerID = contacts_nsstring(draft->container_id);
  if (containerID.length == 0) {
    containerID = nil;
  }
  [save addContact:contact toContainerWithIdentifier:containerID];

  NSError *saveErr = nil;
  if (![store executeSaveRequest:save error:&saveErr]) {
    return contacts_make_error_write_result((ContactsRef){0}, contacts_map_nserror_code(saveErr), saveErr.localizedDescription);
  }

  result = contacts_make_success_result_for_contact(store, contact.identifier, 1, 0);

  if (draft->group_ids_len > 0 && draft->group_ids != NULL) {
    NSError *fetchErr = nil;
    CNContact *savedContact = contacts_contact_by_id(store, contact.identifier, @[CNContactIdentifierKey], &fetchErr);
    if (fetchErr != nil || savedContact == nil) {
      if (fetchErr == nil) {
        fetchErr = [NSError errorWithDomain:CNErrorDomain code:CONTACTS_ERR_NOT_FOUND userInfo:nil];
      }
      result.succeeded = 0;
      result.error_code = contacts_map_nserror_code(fetchErr);
      result.error_message = contacts_strdup_ns(fetchErr.localizedDescription ?: @"unable to load created contact");
      return result;
    }

    CNSaveRequest *groupSave = [[CNSaveRequest alloc] init];
    NSMutableArray *addedGroupIDs = [NSMutableArray array];
    BOOL hasMembershipOps = NO;
    for (int i = 0; i < draft->group_ids_len; i++) {
      NSString *groupID = contacts_nsstring(draft->group_ids[i]);
      if (groupID.length == 0) {
        continue;
      }
      NSError *groupErr = nil;
      CNGroup *group = contacts_group_by_id(store, groupID, &groupErr);
      if (groupErr != nil || group == nil) {
        NSString *msg = groupErr.localizedDescription;
        if (msg.length == 0) {
          msg = [NSString stringWithFormat:@"group %s not found", draft->group_ids[i] ?: ""];
        }
        result.succeeded = 0;
        result.error_code = CONTACTS_ERR_NOT_FOUND;
        result.error_message = contacts_strdup_ns(msg);
        return result;
      }
      [groupSave addMember:savedContact toGroup:group];
      [addedGroupIDs addObject:groupID];
      hasMembershipOps = YES;
    }

    if (hasMembershipOps) {
      NSError *membershipErr = nil;
      if (![store executeSaveRequest:groupSave error:&membershipErr]) {
        result.succeeded = 0;
        result.error_code = contacts_map_nserror_code(membershipErr);
        result.error_message = contacts_strdup_ns(membershipErr.localizedDescription);
        return result;
      }

      int verifyCode = CONTACTS_ERR_NONE;
      NSString *verifyMessage = nil;
      if (!contacts_verify_membership_state(store, contact.identifier, addedGroupIDs, @[], &verifyCode, &verifyMessage)) {
        result.succeeded = 0;
        result.error_code = verifyCode;
        result.error_message = contacts_strdup_ns(verifyMessage ?: @"membership add did not persist");
        return result;
      }

      result.updated = 1;
    }
  }

  return result;
}

static NSArray *contacts_patch_keys(void) {
  return @[
    CNContactIdentifierKey,
    CNContactGivenNameKey,
    CNContactMiddleNameKey,
    CNContactFamilyNameKey,
    CNContactNicknameKey,
    CNContactOrganizationNameKey,
    CNContactJobTitleKey,
    CNContactEmailAddressesKey,
    CNContactPhoneNumbersKey,
  ];
}

static ContactsWriteResult contacts_apply_patch(CNContactStore *store, const ContactsPatch *patch) {
  NSString *contactID = contacts_nsstring(patch->ref.id);
  if (contactID.length == 0) {
    return contacts_make_error_write_result(patch->ref, CONTACTS_ERR_VALIDATION, @"patch ref.id is required");
  }

  NSError *fetchErr = nil;
  CNContact *existing = contacts_contact_by_id(store, contactID, contacts_patch_keys(), &fetchErr);
  if (fetchErr != nil) {
    return contacts_make_error_write_result(patch->ref, contacts_map_nserror_code(fetchErr), fetchErr.localizedDescription);
  }
  if (existing == nil) {
    return contacts_make_error_write_result(patch->ref, CONTACTS_ERR_NOT_FOUND, @"contact not found");
  }

  CNMutableContact *mutable = [existing mutableCopy];
  BOOL hasFieldUpdates = NO;

  if (patch->set_given_name != 0) {
    mutable.givenName = contacts_string_or_empty(contacts_nsstring(patch->given_name));
    hasFieldUpdates = YES;
  }
  if (patch->set_family_name != 0) {
    mutable.familyName = contacts_string_or_empty(contacts_nsstring(patch->family_name));
    hasFieldUpdates = YES;
  }
  if (patch->set_middle_name != 0) {
    mutable.middleName = contacts_string_or_empty(contacts_nsstring(patch->middle_name));
    hasFieldUpdates = YES;
  }
  if (patch->set_nickname != 0) {
    mutable.nickname = contacts_string_or_empty(contacts_nsstring(patch->nickname));
    hasFieldUpdates = YES;
  }
  if (patch->set_organization != 0) {
    mutable.organizationName = contacts_string_or_empty(contacts_nsstring(patch->organization));
    hasFieldUpdates = YES;
  }
  if (patch->set_job_title != 0) {
    mutable.jobTitle = contacts_string_or_empty(contacts_nsstring(patch->job_title));
    hasFieldUpdates = YES;
  }
  if (patch->set_emails != 0) {
    mutable.emailAddresses = contacts_emails_from_c(patch->replace_emails, patch->replace_emails_len);
    hasFieldUpdates = YES;
  }
  if (patch->set_phones != 0) {
    mutable.phoneNumbers = contacts_phones_from_c(patch->replace_phones, patch->replace_phones_len);
    hasFieldUpdates = YES;
  }

  BOOL hasMembershipOps = (patch->add_group_ids_len > 0) || (patch->remove_group_ids_len > 0);

  if (hasFieldUpdates) {
    CNSaveRequest *save = [[CNSaveRequest alloc] init];
    [save updateContact:mutable];
    NSError *updateErr = nil;
    if (![store executeSaveRequest:save error:&updateErr]) {
      return contacts_make_error_write_result(patch->ref, contacts_map_nserror_code(updateErr), updateErr.localizedDescription);
    }
  }

  if (hasMembershipOps) {
    CNSaveRequest *membership = [[CNSaveRequest alloc] init];
    NSMutableArray *addedGroupIDs = [NSMutableArray array];
    NSMutableArray *removedGroupIDs = [NSMutableArray array];
    NSMutableArray *removedGroupNames = [NSMutableArray array];

    for (int i = 0; i < patch->add_group_ids_len; i++) {
      NSString *groupID = contacts_nsstring(patch->add_group_ids[i]);
      if (groupID.length == 0) {
        continue;
      }
      NSError *groupErr = nil;
      CNGroup *group = contacts_group_by_id(store, groupID, &groupErr);
      if (groupErr != nil || group == nil) {
        NSString *msg = groupErr.localizedDescription;
        if (msg.length == 0) {
          msg = [NSString stringWithFormat:@"group %s not found", patch->add_group_ids[i] ?: ""];
        }
        return contacts_make_error_write_result(patch->ref, CONTACTS_ERR_NOT_FOUND, msg);
      }
      [membership addMember:mutable toGroup:group];
      [addedGroupIDs addObject:groupID];
    }

    for (int i = 0; i < patch->remove_group_ids_len; i++) {
      NSString *groupID = contacts_nsstring(patch->remove_group_ids[i]);
      if (groupID.length == 0) {
        continue;
      }
      NSError *groupErr = nil;
      CNGroup *group = contacts_group_by_id(store, groupID, &groupErr);
      if (groupErr != nil || group == nil) {
        NSString *msg = groupErr.localizedDescription;
        if (msg.length == 0) {
          msg = [NSString stringWithFormat:@"group %s not found", patch->remove_group_ids[i] ?: ""];
        }
        return contacts_make_error_write_result(patch->ref, CONTACTS_ERR_NOT_FOUND, msg);
      }
      [removedGroupIDs addObject:groupID];
      [removedGroupNames addObject:contacts_string_or_empty(group.name)];
    }

    if (addedGroupIDs.count > 0) {
      NSError *membershipErr = nil;
      if (![store executeSaveRequest:membership error:&membershipErr]) {
        return contacts_make_error_write_result(patch->ref, contacts_map_nserror_code(membershipErr), membershipErr.localizedDescription);
      }
    }

    for (NSUInteger i = 0; i < removedGroupIDs.count; i++) {
      NSString *removeGroupID = removedGroupIDs[i];
      NSString *removeGroupName = removedGroupNames[i];
      NSString *scriptErr = nil;
      if (!contacts_remove_membership_via_applescript(removeGroupID, removeGroupName, contactID, existing.givenName, existing.familyName, &scriptErr)) {
        NSString *message = [NSString stringWithFormat:@"AppleScript remove failed for group %@: %@", removeGroupID, scriptErr ?: @"unknown error"];
        return contacts_make_error_write_result(patch->ref, CONTACTS_ERR_STORE, message);
      }
    }

    int verifyCode = CONTACTS_ERR_NONE;
    NSString *verifyMessage = nil;
    if (!contacts_verify_membership_state(store, contactID, addedGroupIDs, removedGroupIDs, &verifyCode, &verifyMessage)) {
      return contacts_make_error_write_result(patch->ref, verifyCode, verifyMessage ?: @"membership update did not persist");
    }
  }

  return contacts_make_success_result_for_contact(store, contactID, 0, hasFieldUpdates || hasMembershipOps ? 1 : 0);
}

int contacts_upsert(const ContactsUpsertRequest *req, ContactsUpsertResult *out, ContactsError *err) {
  contacts_reset_error(err);
  if (req == NULL || out == NULL) {
    contacts_set_error(err, CONTACTS_ERR_VALIDATION, @"invalid upsert request");
    return 0;
  }

  out->items = NULL;
  out->items_len = 0;

  if (contacts_authorization_status() != CONTACTS_AUTH_AUTHORIZED) {
    contacts_set_error(err, CONTACTS_ERR_PERMISSION_DENIED, @"contacts access not authorized");
    return 0;
  }

  int total = req->creates_len + req->patches_len;
  if (total <= 0) {
    return 1;
  }

  out->items = (ContactsWriteResult *)calloc((size_t)total, sizeof(ContactsWriteResult));
  out->items_len = total;

  CNContactStore *store = [[CNContactStore alloc] init];

  int idx = 0;
  for (int i = 0; i < req->creates_len; i++) {
    out->items[idx++] = contacts_apply_create(store, &req->creates[i]);
  }
  for (int i = 0; i < req->patches_len; i++) {
    out->items[idx++] = contacts_apply_patch(store, &req->patches[i]);
  }

  return 1;
}

int contacts_mutate(const ContactsMutateRequest *req, ContactsMutateResult *out, ContactsError *err) {
  contacts_reset_error(err);
  if (req == NULL || out == NULL) {
    contacts_set_error(err, CONTACTS_ERR_VALIDATION, @"invalid mutate request");
    return 0;
  }

  out->items = NULL;
  out->items_len = 0;

  if (contacts_authorization_status() != CONTACTS_AUTH_AUTHORIZED) {
    contacts_set_error(err, CONTACTS_ERR_PERMISSION_DENIED, @"contacts access not authorized");
    return 0;
  }

  if (req->refs_len <= 0 || req->refs == NULL) {
    return 1;
  }

  out->items = (ContactsWriteResult *)calloc((size_t)req->refs_len, sizeof(ContactsWriteResult));
  out->items_len = req->refs_len;

  CNContactStore *store = [[CNContactStore alloc] init];

  for (int i = 0; i < req->refs_len; i++) {
    ContactsRef ref = req->refs[i];
    NSString *contactID = contacts_nsstring(ref.id);
    if (contactID.length == 0) {
      out->items[i] = contacts_make_error_write_result(ref, CONTACTS_ERR_VALIDATION, @"mutation ref.id is required");
      continue;
    }

    BOOL deleteOp = NO;
    NSString *setOrg = nil;
    NSString *setJob = nil;
    NSString *setGiven = nil;
    NSString *setFamily = nil;
    NSMutableArray *addGroups = [NSMutableArray array];
    NSMutableArray *removeGroups = [NSMutableArray array];

    for (int oi = 0; oi < req->ops_len; oi++) {
      ContactsMutationOp op = req->ops[oi];
      NSString *value = contacts_nsstring(op.value);
      switch (op.type) {
        case CONTACTS_MUTATION_SET_ORGANIZATION:
          setOrg = value ?: @"";
          break;
        case CONTACTS_MUTATION_SET_JOB_TITLE:
          setJob = value ?: @"";
          break;
        case CONTACTS_MUTATION_SET_GIVEN_NAME:
          setGiven = value ?: @"";
          break;
        case CONTACTS_MUTATION_SET_FAMILY_NAME:
          setFamily = value ?: @"";
          break;
        case CONTACTS_MUTATION_ADD_TO_GROUP:
          if (value.length > 0) {
            [addGroups addObject:value];
          }
          break;
        case CONTACTS_MUTATION_REMOVE_FROM_GROUP:
          if (value.length > 0) {
            [removeGroups addObject:value];
          }
          break;
        case CONTACTS_MUTATION_DELETE:
          deleteOp = YES;
          break;
        default:
          break;
      }
    }

    NSError *fetchErr = nil;
    CNContact *existing = contacts_contact_by_id(store, contactID, contacts_patch_keys(), &fetchErr);
    if (fetchErr != nil) {
      out->items[i] = contacts_make_error_write_result(ref, contacts_map_nserror_code(fetchErr), fetchErr.localizedDescription);
      continue;
    }
    if (existing == nil) {
      out->items[i] = contacts_make_error_write_result(ref, CONTACTS_ERR_NOT_FOUND, @"contact not found");
      continue;
    }

    if (deleteOp) {
      CNSaveRequest *deleteReq = [[CNSaveRequest alloc] init];
      [deleteReq deleteContact:[existing mutableCopy]];
      NSError *deleteErr = nil;
      if (![store executeSaveRequest:deleteReq error:&deleteErr]) {
        out->items[i] = contacts_make_error_write_result(ref, contacts_map_nserror_code(deleteErr), deleteErr.localizedDescription);
      } else {
        out->items[i] = contacts_make_success_result_for_contact(store, contactID, 0, 1);
      }
      continue;
    }

    CNMutableContact *mutable = [existing mutableCopy];
    BOOL hasFieldUpdates = NO;
    if (setOrg != nil) {
      mutable.organizationName = setOrg;
      hasFieldUpdates = YES;
    }
    if (setJob != nil) {
      mutable.jobTitle = setJob;
      hasFieldUpdates = YES;
    }
    if (setGiven != nil) {
      mutable.givenName = setGiven;
      hasFieldUpdates = YES;
    }
    if (setFamily != nil) {
      mutable.familyName = setFamily;
      hasFieldUpdates = YES;
    }

    if (hasFieldUpdates) {
      CNSaveRequest *updateReq = [[CNSaveRequest alloc] init];
      [updateReq updateContact:mutable];
      NSError *updateErr = nil;
      if (![store executeSaveRequest:updateReq error:&updateErr]) {
        out->items[i] = contacts_make_error_write_result(ref, contacts_map_nserror_code(updateErr), updateErr.localizedDescription);
        continue;
      }
    }

    if (addGroups.count > 0 || removeGroups.count > 0) {
      CNSaveRequest *membershipReq = [[CNSaveRequest alloc] init];
      BOOL membershipError = NO;
      NSString *membershipMessage = nil;
      NSMutableArray *addedGroupIDs = [NSMutableArray array];
      NSMutableArray *removedGroupIDs = [NSMutableArray array];
      NSMutableArray *removedGroupNames = [NSMutableArray array];

      for (NSString *groupID in addGroups) {
        NSError *groupErr = nil;
        CNGroup *group = contacts_group_by_id(store, groupID, &groupErr);
        if (groupErr != nil || group == nil) {
          membershipError = YES;
          membershipMessage = groupErr.localizedDescription;
          if (membershipMessage.length == 0) {
            membershipMessage = [NSString stringWithFormat:@"group %@ not found", groupID];
          }
          break;
        }
        [membershipReq addMember:mutable toGroup:group];
        [addedGroupIDs addObject:groupID];
      }

      if (!membershipError) {
        for (NSString *groupID in removeGroups) {
          NSError *groupErr = nil;
          CNGroup *group = contacts_group_by_id(store, groupID, &groupErr);
          if (groupErr != nil || group == nil) {
            membershipError = YES;
            membershipMessage = groupErr.localizedDescription;
            if (membershipMessage.length == 0) {
              membershipMessage = [NSString stringWithFormat:@"group %@ not found", groupID];
            }
            break;
          }

          [removedGroupIDs addObject:groupID];
          [removedGroupNames addObject:contacts_string_or_empty(group.name)];
        }
      }

      if (membershipError) {
        out->items[i] = contacts_make_error_write_result(ref, CONTACTS_ERR_NOT_FOUND, membershipMessage);
        continue;
      }

      if (addedGroupIDs.count > 0) {
        NSError *membershipErr = nil;
        if (![store executeSaveRequest:membershipReq error:&membershipErr]) {
          out->items[i] = contacts_make_error_write_result(ref, contacts_map_nserror_code(membershipErr), membershipErr.localizedDescription);
          continue;
        }
      }

      for (NSUInteger gi = 0; gi < removedGroupIDs.count; gi++) {
        NSString *removeGroupID = removedGroupIDs[gi];
        NSString *removeGroupName = removedGroupNames[gi];
        NSString *scriptErr = nil;
        if (!contacts_remove_membership_via_applescript(removeGroupID, removeGroupName, contactID, existing.givenName, existing.familyName, &scriptErr)) {
          membershipError = YES;
          membershipMessage = [NSString stringWithFormat:@"AppleScript remove failed for group %@: %@", removeGroupID, scriptErr ?: @"unknown error"];
          break;
        }
      }

      if (membershipError) {
        out->items[i] = contacts_make_error_write_result(ref, CONTACTS_ERR_STORE, membershipMessage);
        continue;
      }

      int verifyCode = CONTACTS_ERR_NONE;
      NSString *verifyMessage = nil;
      if (!contacts_verify_membership_state(store, contactID, addedGroupIDs, removedGroupIDs, &verifyCode, &verifyMessage)) {
        out->items[i] = contacts_make_error_write_result(ref, verifyCode, verifyMessage ?: @"membership update did not persist");
        continue;
      }
    }

    out->items[i] = contacts_make_success_result_for_contact(store, contactID, 0, 1);
  }

  return 1;
}

static int contacts_fill_group(CNContactStore *store, CNGroup *group, ContactsGroup *outGroup) {
  if (store == nil || group == nil || outGroup == NULL) {
    return 0;
  }
  outGroup->id = contacts_strdup_ns(group.identifier ?: @"");
  outGroup->name = contacts_strdup_ns(group.name ?: @"");
  outGroup->container_id = NULL;
  outGroup->account_id = NULL;

  NSError *containerErr = nil;
  NSArray *containers = [store containersMatchingPredicate:[CNContainer predicateForContainerOfGroupWithIdentifier:group.identifier]
                                                     error:&containerErr];
  if (containerErr == nil && containers.count > 0) {
    CNContainer *container = containers.firstObject;
    outGroup->container_id = contacts_strdup_ns(container.identifier ?: @"");
    outGroup->account_id = contacts_strdup_ns(container.identifier ?: @"");
  }
  return 1;
}

static int contacts_list_groups(CNContactStore *store, ContactsGroupsResult *out, ContactsError *err) {
  NSError *listErr = nil;
  NSArray *groups = [store groupsMatchingPredicate:nil error:&listErr];
  if (listErr != nil) {
    contacts_set_error(err, contacts_map_nserror_code(listErr), listErr.localizedDescription);
    return 0;
  }

  out->groups = NULL;
  out->groups_len = 0;
  if (groups.count == 0) {
    return 1;
  }

  out->groups = (ContactsGroup *)calloc((size_t)groups.count, sizeof(ContactsGroup));
  out->groups_len = (int)groups.count;
  for (NSUInteger i = 0; i < groups.count; i++) {
    contacts_fill_group(store, groups[i], &out->groups[i]);
  }
  return 1;
}

int contacts_groups(const ContactsGroupsRequest *req, ContactsGroupsResult *out, ContactsError *err) {
  contacts_reset_error(err);
  if (req == NULL || out == NULL) {
    contacts_set_error(err, CONTACTS_ERR_VALIDATION, @"invalid groups request");
    return 0;
  }

  out->groups = NULL;
  out->groups_len = 0;
  out->results = NULL;
  out->results_len = 0;

  if (contacts_authorization_status() != CONTACTS_AUTH_AUTHORIZED) {
    contacts_set_error(err, CONTACTS_ERR_PERMISSION_DENIED, @"contacts access not authorized");
    return 0;
  }

  CNContactStore *store = [[CNContactStore alloc] init];

  if (req->action == CONTACTS_GROUPS_CREATE || req->action == CONTACTS_GROUPS_RENAME || req->action == CONTACTS_GROUPS_DELETE) {
    out->results = (ContactsWriteResult *)calloc(1, sizeof(ContactsWriteResult));
    out->results_len = 1;
    ContactsWriteResult *result = &out->results[0];

    if (req->action == CONTACTS_GROUPS_CREATE) {
      NSString *name = contacts_nsstring(req->name);
      if (name.length == 0) {
        *result = contacts_make_error_write_result((ContactsRef){0}, CONTACTS_ERR_VALIDATION, @"group name is required");
      } else {
        CNMutableGroup *group = [[CNMutableGroup alloc] init];
        group.name = name;
        CNSaveRequest *save = [[CNSaveRequest alloc] init];
        NSString *containerID = contacts_nsstring(req->container_id);
        if (containerID.length == 0) {
          containerID = nil;
        }
        [save addGroup:group toContainerWithIdentifier:containerID];
        NSError *saveErr = nil;
        if (![store executeSaveRequest:save error:&saveErr]) {
          *result = contacts_make_error_write_result((ContactsRef){0}, contacts_map_nserror_code(saveErr), saveErr.localizedDescription);
        } else {
          ContactsWriteResult okResult = contacts_make_success_result_for_contact(store, group.identifier, 1, 0);
          *result = okResult;
        }
      }
    }

    if (req->action == CONTACTS_GROUPS_RENAME) {
      NSString *groupID = contacts_nsstring(req->group_id);
      NSString *name = contacts_nsstring(req->name);
      if (groupID.length == 0 || name.length == 0) {
        *result = contacts_make_error_write_result((ContactsRef){0}, CONTACTS_ERR_VALIDATION, @"group_id and name are required");
      } else {
        NSError *groupErr = nil;
        CNGroup *group = contacts_group_by_id(store, groupID, &groupErr);
        if (groupErr != nil || group == nil) {
          NSString *msg = groupErr.localizedDescription;
          if (msg.length == 0) {
            msg = @"group not found";
          }
          *result = contacts_make_error_write_result((ContactsRef){0}, CONTACTS_ERR_NOT_FOUND, msg);
        } else {
          CNMutableGroup *mutable = [group mutableCopy];
          mutable.name = name;
          CNSaveRequest *save = [[CNSaveRequest alloc] init];
          [save updateGroup:mutable];
          NSError *saveErr = nil;
          if (![store executeSaveRequest:save error:&saveErr]) {
            *result = contacts_make_error_write_result((ContactsRef){0}, contacts_map_nserror_code(saveErr), saveErr.localizedDescription);
          } else {
            ContactsWriteResult okResult = contacts_make_success_result_for_contact(store, groupID, 0, 1);
            *result = okResult;
          }
        }
      }
    }

    if (req->action == CONTACTS_GROUPS_DELETE) {
      NSString *groupID = contacts_nsstring(req->group_id);
      if (groupID.length == 0) {
        *result = contacts_make_error_write_result((ContactsRef){0}, CONTACTS_ERR_VALIDATION, @"group_id is required");
      } else {
        NSError *groupErr = nil;
        CNGroup *group = contacts_group_by_id(store, groupID, &groupErr);
        if (groupErr != nil || group == nil) {
          NSString *msg = groupErr.localizedDescription;
          if (msg.length == 0) {
            msg = @"group not found";
          }
          *result = contacts_make_error_write_result((ContactsRef){0}, CONTACTS_ERR_NOT_FOUND, msg);
        } else {
          CNSaveRequest *save = [[CNSaveRequest alloc] init];
          [save deleteGroup:[group mutableCopy]];
          NSError *saveErr = nil;
          if (![store executeSaveRequest:save error:&saveErr]) {
            *result = contacts_make_error_write_result((ContactsRef){0}, contacts_map_nserror_code(saveErr), saveErr.localizedDescription);
          } else {
            ContactsWriteResult okResult = contacts_make_success_result_for_contact(store, groupID, 0, 1);
            *result = okResult;
          }
        }
      }
    }
  }

  if (!contacts_list_groups(store, out, err)) {
    return 0;
  }

  return 1;
}

void contacts_free_find_result(ContactsFindResult *res) {
  if (res == NULL) {
    return;
  }
  if (res->items != NULL) {
    for (int i = 0; i < res->items_len; i++) {
      ContactsFoundRef *item = &res->items[i];
      if (item->id != NULL) {
        free(item->id);
      }
      if (item->container_id != NULL) {
        free(item->container_id);
      }
      if (item->account_id != NULL) {
        free(item->account_id);
      }
      if (item->display_name != NULL) {
        free(item->display_name);
      }
      if (item->organization != NULL) {
        free(item->organization);
      }
    }
    free(res->items);
  }
  res->items = NULL;
  res->items_len = 0;
  res->next_offset = -1;
}

void contacts_free_get_result(ContactsGetResult *res) {
  if (res == NULL) {
    return;
  }
  if (res->items != NULL) {
    for (int i = 0; i < res->items_len; i++) {
      ContactsContact *item = &res->items[i];
      contacts_free_ref(&item->ref);
      if (item->given_name != NULL) {
        free(item->given_name);
      }
      if (item->family_name != NULL) {
        free(item->family_name);
      }
      if (item->middle_name != NULL) {
        free(item->middle_name);
      }
      if (item->nickname != NULL) {
        free(item->nickname);
      }
      if (item->organization != NULL) {
        free(item->organization);
      }
      if (item->job_title != NULL) {
        free(item->job_title);
      }
      contacts_free_labeled_values(item->emails, item->emails_len);
      contacts_free_labeled_values(item->phones, item->phones_len);
      if (item->group_ids != NULL) {
        for (int gi = 0; gi < item->group_ids_len; gi++) {
          if (item->group_ids[gi] != NULL) {
            free(item->group_ids[gi]);
          }
        }
        free(item->group_ids);
      }
    }
    free(res->items);
  }

  res->items = NULL;
  res->items_len = 0;
}

static void contacts_free_write_results(ContactsWriteResult *items, int len) {
  if (items == NULL) {
    return;
  }
  for (int i = 0; i < len; i++) {
    contacts_free_ref(&items[i].ref);
    if (items[i].error_message != NULL) {
      free(items[i].error_message);
      items[i].error_message = NULL;
    }
  }
  free(items);
}

void contacts_free_upsert_result(ContactsUpsertResult *res) {
  if (res == NULL) {
    return;
  }
  contacts_free_write_results(res->items, res->items_len);
  res->items = NULL;
  res->items_len = 0;
}

void contacts_free_mutate_result(ContactsMutateResult *res) {
  if (res == NULL) {
    return;
  }
  contacts_free_write_results(res->items, res->items_len);
  res->items = NULL;
  res->items_len = 0;
}

void contacts_free_groups_result(ContactsGroupsResult *res) {
  if (res == NULL) {
    return;
  }

  if (res->groups != NULL) {
    for (int i = 0; i < res->groups_len; i++) {
      ContactsGroup *group = &res->groups[i];
      if (group->id != NULL) {
        free(group->id);
      }
      if (group->container_id != NULL) {
        free(group->container_id);
      }
      if (group->account_id != NULL) {
        free(group->account_id);
      }
      if (group->name != NULL) {
        free(group->name);
      }
    }
    free(res->groups);
  }
  res->groups = NULL;
  res->groups_len = 0;

  contacts_free_write_results(res->results, res->results_len);
  res->results = NULL;
  res->results_len = 0;
}
