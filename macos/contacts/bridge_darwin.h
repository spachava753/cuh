#ifndef CUH_MACOS_CONTACTS_BRIDGE_DARWIN_H
#define CUH_MACOS_CONTACTS_BRIDGE_DARWIN_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

enum {
  CONTACTS_AUTH_NOT_DETERMINED = 0,
  CONTACTS_AUTH_RESTRICTED = 1,
  CONTACTS_AUTH_DENIED = 2,
  CONTACTS_AUTH_AUTHORIZED = 3,
};

enum {
  CONTACTS_ERR_NONE = 0,
  CONTACTS_ERR_PERMISSION_DENIED = 1,
  CONTACTS_ERR_NOT_FOUND = 2,
  CONTACTS_ERR_CONFLICT = 3,
  CONTACTS_ERR_VALIDATION = 4,
  CONTACTS_ERR_STORE = 5,
  CONTACTS_ERR_UNKNOWN = 99,
};

enum {
  CONTACTS_MATCH_ALL = 0,
  CONTACTS_MATCH_ANY = 1,
};

enum {
  CONTACTS_SORT_GIVEN_NAME = 0,
  CONTACTS_SORT_FAMILY_NAME = 1,
};

enum {
  CONTACTS_SORT_ASC = 0,
  CONTACTS_SORT_DESC = 1,
};

enum {
  CONTACTS_FIELD_NAMES = 1 << 0,
  CONTACTS_FIELD_ORGANIZATION = 1 << 1,
  CONTACTS_FIELD_EMAILS = 1 << 2,
  CONTACTS_FIELD_PHONES = 1 << 3,
  CONTACTS_FIELD_GROUPS = 1 << 4,
};

enum {
  CONTACTS_MUTATION_SET_ORGANIZATION = 1,
  CONTACTS_MUTATION_SET_JOB_TITLE = 2,
  CONTACTS_MUTATION_SET_GIVEN_NAME = 3,
  CONTACTS_MUTATION_SET_FAMILY_NAME = 4,
  CONTACTS_MUTATION_ADD_TO_GROUP = 5,
  CONTACTS_MUTATION_REMOVE_FROM_GROUP = 6,
  CONTACTS_MUTATION_DELETE = 7,
};

enum {
  CONTACTS_GROUPS_LIST = 1,
  CONTACTS_GROUPS_CREATE = 2,
  CONTACTS_GROUPS_RENAME = 3,
  CONTACTS_GROUPS_DELETE = 4,
};

typedef struct {
  int code;
  char *message;
} ContactsError;

typedef struct {
  char *id;
  char *container_id;
  char *account_id;
} ContactsRef;

typedef struct {
  char *label;
  char *value;
} ContactsLabeledValue;

typedef struct {
  char *id;
  char *container_id;
  char *account_id;
  char *display_name;
  char *organization;
  int64_t modified_at_unix;
} ContactsFoundRef;

typedef struct {
  ContactsFoundRef *items;
  int items_len;
  int next_offset;
} ContactsFindResult;

typedef struct {
  ContactsRef ref;
  char *given_name;
  char *family_name;
  char *middle_name;
  char *nickname;
  char *organization;
  char *job_title;
  ContactsLabeledValue *emails;
  int emails_len;
  ContactsLabeledValue *phones;
  int phones_len;
  char **group_ids;
  int group_ids_len;
  int64_t modified_at_unix;
} ContactsContact;

typedef struct {
  ContactsContact *items;
  int items_len;
} ContactsGetResult;

typedef struct {
  char *name_contains;
  char *organization_contains;
  char *email_domain;
  char **group_ids_any;
  int group_ids_any_len;
  char **ids;
  int ids_len;
  int match_policy;
  int limit;
  int offset;
  int include_meta;
  int sort_by;
  int sort_order;
} ContactsFindRequest;

typedef struct {
  ContactsRef *refs;
  int refs_len;
  uint32_t field_mask;
} ContactsGetRequest;

typedef struct {
  char *container_id;
  char *given_name;
  char *family_name;
  char *middle_name;
  char *nickname;
  char *organization;
  char *job_title;
  ContactsLabeledValue *emails;
  int emails_len;
  ContactsLabeledValue *phones;
  int phones_len;
  char **group_ids;
  int group_ids_len;
} ContactsDraft;

typedef struct {
  ContactsRef ref;
  int set_given_name;
  char *given_name;
  int set_family_name;
  char *family_name;
  int set_middle_name;
  char *middle_name;
  int set_nickname;
  char *nickname;
  int set_organization;
  char *organization;
  int set_job_title;
  char *job_title;
  int set_emails;
  ContactsLabeledValue *replace_emails;
  int replace_emails_len;
  int set_phones;
  ContactsLabeledValue *replace_phones;
  int replace_phones_len;
  char **add_group_ids;
  int add_group_ids_len;
  char **remove_group_ids;
  int remove_group_ids_len;
} ContactsPatch;

typedef struct {
  ContactsDraft *creates;
  int creates_len;
  ContactsPatch *patches;
  int patches_len;
} ContactsUpsertRequest;

typedef struct {
  ContactsRef ref;
  int succeeded;
  int created;
  int updated;
  int error_code;
  char *error_message;
} ContactsWriteResult;

typedef struct {
  ContactsWriteResult *items;
  int items_len;
} ContactsUpsertResult;

typedef struct {
  int type;
  char *value;
} ContactsMutationOp;

typedef struct {
  ContactsRef *refs;
  int refs_len;
  ContactsMutationOp *ops;
  int ops_len;
} ContactsMutateRequest;

typedef struct {
  ContactsWriteResult *items;
  int items_len;
} ContactsMutateResult;

typedef struct {
  char *id;
  char *container_id;
  char *account_id;
  char *name;
} ContactsGroup;

typedef struct {
  int action;
  char *group_id;
  char *name;
  char *container_id;
} ContactsGroupsRequest;

typedef struct {
  ContactsGroup *groups;
  int groups_len;
  ContactsWriteResult *results;
  int results_len;
} ContactsGroupsResult;

int contacts_authorization_status(void);
int contacts_request_access(ContactsError *err);

int contacts_find(const ContactsFindRequest *req, ContactsFindResult *out, ContactsError *err);
int contacts_get(const ContactsGetRequest *req, ContactsGetResult *out, ContactsError *err);
int contacts_upsert(const ContactsUpsertRequest *req, ContactsUpsertResult *out, ContactsError *err);
int contacts_mutate(const ContactsMutateRequest *req, ContactsMutateResult *out, ContactsError *err);
int contacts_groups(const ContactsGroupsRequest *req, ContactsGroupsResult *out, ContactsError *err);

void contacts_free_error(ContactsError *err);
void contacts_free_find_result(ContactsFindResult *res);
void contacts_free_get_result(ContactsGetResult *res);
void contacts_free_upsert_result(ContactsUpsertResult *res);
void contacts_free_mutate_result(ContactsMutateResult *res);
void contacts_free_groups_result(ContactsGroupsResult *res);

#ifdef __cplusplus
}
#endif

#endif
