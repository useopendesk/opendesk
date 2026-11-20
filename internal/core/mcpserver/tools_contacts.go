package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/auth"
)

// --- contacts and companies, written and read ---------------------------------------
//
// The writes are the API's own (contracts/api.md "Contacts and companies"), run as the key:
// the scope is contacts:write, labels are added or removed by name and custom fields are
// merged, so a client never wipes what it did not send. Deleting, merging and blocking stay
// out, to keep what an assistant can do small and undoable.

func contactWriteTools() []*tool {
	company := "A company by id, or by its exact name (a name needs the contacts:read scope and must match one company; use search_companies when it does not)."
	return []*tool{
		{
			Name: "create_contact", Title: "Create a contact", Scopes: []string{auth.ScopeContactsWrite},
			Description: "Create a customer (contact) with an email, a name or both. An email that already belongs to a contact is refused and the " +
				"error names that contact, so you can update it instead. Set custom fields with fields, by the keys list_custom_fields returns for entity contact.",
			Schema: obj(map[string]any{
				"email":       strN("The contact's email address.", 3, 254),
				"name":        strN("The contact's name.", 1, 200),
				"phone":       strN("A phone number.", 1, 50),
				"company":     str(company),
				"external_id": strN("Your own system's id for this person.", 1, 200),
				"labels":      arr("Label names to give the contact.", strN("", 1, 50), 50),
				"notes":       strN("Notes for the team, in plain text.", 1, 10000),
				"fields":      entityFields("contact", "Custom field values, by key."),
			}),
			Run: createContact,
		},
		{
			Name: "update_contact", Title: "Update a contact", Scopes: []string{auth.ScopeContactsWrite}, Destructive: true, Idempotent: true,
			Description: "Change a contact's details. Only the arguments you pass change; an empty string clears email, phone, external_id or notes, and " +
				"company none removes the company. Labels are added or removed by name and custom fields are merged (null clears one). Returns the updated contact.",
			Schema: obj(map[string]any{
				"contact":       str("The contact's id, or their exact email address."),
				"email":         str("A new primary email address; an empty string clears it."),
				"name":          str("A new name."),
				"phone":         str("A phone number; an empty string clears it."),
				"company":       str(company + " none removes it."),
				"external_id":   str("Your own system's id; an empty string clears it."),
				"add_labels":    arr("Label names to add.", strN("", 1, 50), 50),
				"remove_labels": arr("Label names to remove.", strN("", 1, 50), 50),
				"notes":         str("Notes for the team, replacing the old ones; an empty string clears them."),
				"fields":        entityFields("contact", "Custom field values to change, by key; the others keep theirs. null clears a field."),
			}, "contact"),
			Run: updateContact,
		},
		{
			Name: "create_company", Title: "Create a company", Scopes: []string{auth.ScopeContactsWrite},
			Description: "Create a company (an organisation that contacts belong to). A taken external_id is refused. " +
				"Link people to it with create_contact or update_contact. Custom fields use the keys list_custom_fields returns for entity company.",
			Schema: obj(map[string]any{
				"name":        strN("The company's name.", 1, 200),
				"domain":      strN("Its website domain, such as acme.com.", 1, 253),
				"external_id": strN("Your own system's id for this account.", 1, 200),
				"labels":      arr("Label names to give the company.", strN("", 1, 50), 50),
				"notes":       strN("Notes for the team, in plain text.", 1, 10000),
				"fields":      entityFields("company", "Custom field values, by key."),
			}, "name"),
			Run: createCompany,
		},
		{
			Name: "update_company", Title: "Update a company", Scopes: []string{auth.ScopeContactsWrite}, Destructive: true, Idempotent: true,
			Description: "Change a company's details. Only the arguments you pass change; an empty string clears domain, external_id or notes. Labels are added " +
				"or removed by name and custom fields are merged (null clears one). Returns the updated company.",
			Schema: obj(map[string]any{
				"company":       str("The company's id, or its exact name."),
				"name":          strN("A new name.", 1, 200),
				"domain":        str("A new domain; an empty string clears it."),
				"external_id":   str("Your own system's id; an empty string clears it."),
				"add_labels":    arr("Label names to add.", strN("", 1, 50), 50),
				"remove_labels": arr("Label names to remove.", strN("", 1, 50), 50),
				"notes":         str("Notes for the team, replacing the old ones; an empty string clears them."),
				"fields":        entityFields("company", "Custom field values to change, by key; the others keep theirs. null clears a field."),
			}, "company"),
			Run: updateCompany,
		},
		{
			Name: "search_companies", Title: "Search companies", Scopes: []string{auth.ScopeContactsRead}, ReadOnly: true, Idempotent: true,
			Description: "Find companies by name, domain or custom field value. Without a query, lists them by name.",
			Schema: obj(map[string]any{
				"query": strN("What to search for: a name, a domain or a field value.", 1, 200),
				"limit": integer("How many companies to return.", 1, 50, 10),
			}),
			Run: searchCompanies,
		},
		{
			Name: "get_company", Title: "Get a company", Scopes: []string{auth.ScopeContactsRead}, ReadOnly: true, Idempotent: true,
			Description: "Read one company by id or exact name: details, labels, custom fields, notes and how many people belong to it.",
			Schema:      obj(map[string]any{"company": str("The company's id, or its exact name.")}, "company"),
			Run:         getCompany,
		},
	}
}

// entityFields is fieldsArg for a contact's or a company's fields.
func entityFields(entity, desc string) map[string]any {
	m := fieldsArg(desc)
	m["description"] = strings.Replace(m["description"].(string), "(entity ticket)", "(entity "+entity+")", 1)
	return m
}

type apiCompany struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Domain       string         `json:"domain"`
	ExternalID   *string        `json:"external_id"`
	Notes        string         `json:"notes"`
	Tags         []tagRef       `json:"tags"`
	Fields       map[string]any `json:"fields"`
	ContactCount int            `json:"contact_count"`
	CreatedAt    time.Time      `json:"created_at"`
}

type outCompanyFull struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Domain       string         `json:"domain,omitempty"`
	ExternalID   string         `json:"external_id,omitempty"`
	Labels       []string       `json:"labels,omitempty"`
	Fields       map[string]any `json:"fields,omitempty"`
	Notes        string         `json:"notes,omitempty"`
	ContactCount int            `json:"contact_count"`
	CreatedAt    time.Time      `json:"created_at"`
}

func compactCompany(c apiCompany, full bool) outCompanyFull {
	o := outCompanyFull{ID: c.ID, Name: c.Name, Domain: c.Domain, ContactCount: c.ContactCount, CreatedAt: c.CreatedAt}
	for _, t := range c.Tags {
		o.Labels = append(o.Labels, t.Name)
	}
	if full {
		if c.ExternalID != nil {
			o.ExternalID = *c.ExternalID
		}
		if len(c.Fields) > 0 {
			o.Fields = c.Fields
		}
		o.Notes = clip(c.Notes, 2000)
	}
	return o
}

// companyID turns a company id or exact name into its id. A name is looked up with the
// list (contacts:read) and must match exactly one company, ignoring case.
func companyID(ctx context.Context, c *caller, raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if id, err := uuid.Parse(v); err == nil {
		return id.String(), nil
	}
	if v == "" || len([]rune(v)) > 200 {
		return "", invalid("company must be an id or a name")
	}
	var pg page[apiCompany]
	if err := c.get(ctx, "/v1/companies", url.Values{"q": {v}, "limit": {"50"}}, &pg); err != nil {
		if statusOf(err) == http.StatusForbidden {
			return "", invalid("looking up a company by name needs the contacts:read scope; pass the company id instead")
		}
		return "", err
	}
	var hits []apiCompany
	for _, co := range pg.Data {
		if strings.EqualFold(co.Name, v) {
			hits = append(hits, co)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0].ID, nil
	case 0:
		return "", invalid("no company is named " + strconv.Quote(clip(v, 100)) + "; search_companies finds one, create_company adds it")
	}
	return "", invalid("more than one company is named " + strconv.Quote(clip(v, 100)) + "; use its id")
}

// contactID turns a contact id or exact email into its id.
func contactID(ctx context.Context, c *caller, raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if !strings.Contains(v, "@") {
		id, err := idRef(v, "contact")
		if err != nil {
			return "", invalid("contact must be an id or an email address")
		}
		return id, nil
	}
	if id := contactByEmail(ctx, c, v); id != "" {
		return id, nil
	}
	return "", invalid("no contact has the email " + strconv.Quote(clip(v, 254)))
}

// contactByEmail is the id of the contact whose primary email is v, or "".
func contactByEmail(ctx context.Context, c *caller, v string) string {
	if len([]rune(v)) > 254 {
		return ""
	}
	var pg page[apiContact]
	if err := c.get(ctx, "/v1/contacts", url.Values{"q": {v}, "limit": {"20"}}, &pg); err != nil {
		return ""
	}
	for _, ct := range pg.Data {
		if ct.Email != nil && strings.EqualFold(*ct.Email, v) {
			return ct.ID
		}
	}
	return ""
}

// taken adds what a conflicting record is, so the client can update it instead.
func taken(err error, what, id string) error {
	if statusOf(err) != http.StatusConflict || id == "" {
		return err
	}
	return invalid(err.Error() + " (the existing " + what + " is " + id + "; use update_" + what + ")")
}

// setStr puts a text argument in the body: absent is left alone, an empty one clears. The
// API clears email and external_id with null (nullable) and phone and notes with "".
func setStr(body map[string]any, key string, v *string, max int, nullable bool) error {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if len([]rune(s)) > max {
		return invalid(key + " is too long")
	}
	if s == "" && nullable {
		body[key] = nil
	} else {
		body[key] = s
	}
	return nil
}

func createContact(ctx context.Context, c *caller, raw json.RawMessage) (any, error) {
	a, err := decode[struct {
		Email      string         `json:"email"`
		Name       string         `json:"name"`
		Phone      string         `json:"phone"`
		Company    string         `json:"company"`
		ExternalID string         `json:"external_id"`
		Labels     []string       `json:"labels"`
		Notes      string         `json:"notes"`
		Fields     map[string]any `json:"fields"`
	}](raw)
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(a.Email)
	if email == "" && strings.TrimSpace(a.Name) == "" {
		return nil, invalid("a contact needs an email or a name")
	}
	if email != "" && (!strings.Contains(email, "@") || len(email) > 254) {
		return nil, invalid("email must be an email address")
	}
	body := map[string]any{}
	if email != "" {
		body["email"] = email
	}
	for k, v := range map[string]string{"name": clip(a.Name, 200), "phone": clip(a.Phone, 50), "external_id": clip(a.ExternalID, 200), "notes": strings.TrimSpace(a.Notes)} {
		if v != "" {
			body[k] = v
		}
	}
	if strings.TrimSpace(a.Company) != "" {
		id, err := companyID(ctx, c, a.Company)
		if err != nil {
			return nil, err
		}
		body["company_id"] = id
	}
	labels, err := cleanLabels(a.Labels)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		body["add_tags"] = labels
	}
	if len(a.Fields) > 0 {
		body["fields"] = a.Fields
	}
	var ct apiContact
	if err := c.do(ctx, http.MethodPost, "/v1/contacts", nil, body, &ct); err != nil {
		var existing string
		if statusOf(err) == http.StatusConflict && email != "" {
			existing = contactByEmail(ctx, c, email)
		}
		return nil, taken(err, "contact", existing)
	}
	return compactContact(ct, true), nil
}

func updateContact(ctx context.Context, c *caller, raw json.RawMessage) (any, error) {
	a, err := decode[struct {
		Contact      string         `json:"contact"`
		Email        *string        `json:"email"`
		Name         *string        `json:"name"`
		Phone        *string        `json:"phone"`
		Company      *string        `json:"company"`
		ExternalID   *string        `json:"external_id"`
		AddLabels    []string       `json:"add_labels"`
		RemoveLabels []string       `json:"remove_labels"`
		Notes        *string        `json:"notes"`
		Fields       map[string]any `json:"fields"`
	}](raw)
	if err != nil {
		return nil, err
	}
	id, err := contactID(ctx, c, a.Contact)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if a.Email != nil {
		if e := strings.TrimSpace(*a.Email); e != "" && (!strings.Contains(e, "@") || len(e) > 254) {
			return nil, invalid("email must be an email address")
		}
	}
	for key, v := range map[string]*string{"email": a.Email, "phone": a.Phone, "external_id": a.ExternalID, "notes": a.Notes} {
		if err := setStr(body, key, v, 10000, key == "email" || key == "external_id"); err != nil {
			return nil, err
		}
	}
	if a.Name != nil {
		n := clip(*a.Name, 200)
		if n == "" {
			return nil, invalid("name cannot be empty")
		}
		body["name"] = n
	}
	if a.Company != nil {
		if v := strings.TrimSpace(*a.Company); v == "" || strings.EqualFold(v, "none") {
			body["company_id"] = nil
		} else {
			cid, err := companyID(ctx, c, v)
			if err != nil {
				return nil, err
			}
			body["company_id"] = cid
		}
	}
	add, err := cleanLabels(a.AddLabels)
	if err != nil {
		return nil, err
	}
	remove, err := cleanLabels(a.RemoveLabels)
	if err != nil {
		return nil, err
	}
	if len(add) > 0 {
		body["add_tags"] = add
	}
	if len(remove) > 0 {
		body["remove_tags"] = remove
	}
	if len(a.Fields) > 0 {
		body["fields"] = a.Fields
	}
	if len(body) == 0 {
		return nil, invalid("nothing to change: pass email, name, phone, company, external_id, notes, add_labels, remove_labels or fields")
	}
	var ct apiContact
	if err := c.do(ctx, http.MethodPatch, "/v1/contacts/"+id, nil, body, &ct); err != nil {
		var existing string
		if e, ok := body["email"].(string); ok && statusOf(err) == http.StatusConflict {
			existing = contactByEmail(ctx, c, e)
		}
		return nil, taken(err, "contact", existing)
	}
	return compactContact(ct, true), nil
}

func createCompany(ctx context.Context, c *caller, raw json.RawMessage) (any, error) {
	a, err := decode[struct {
		Name       string         `json:"name"`
		Domain     string         `json:"domain"`
		ExternalID string         `json:"external_id"`
		Labels     []string       `json:"labels"`
		Notes      string         `json:"notes"`
		Fields     map[string]any `json:"fields"`
	}](raw)
	if err != nil {
		return nil, err
	}
	name := clip(a.Name, 200)
	if name == "" {
		return nil, invalid("a company needs a name")
	}
	body := map[string]any{"name": name}
	for k, v := range map[string]string{"domain": clip(a.Domain, 253), "external_id": clip(a.ExternalID, 200), "notes": strings.TrimSpace(a.Notes)} {
		if v != "" {
			body[k] = v
		}
	}
	labels, err := cleanLabels(a.Labels)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		body["add_tags"] = labels
	}
	if len(a.Fields) > 0 {
		body["fields"] = a.Fields
	}
	var co apiCompany
	if err := c.do(ctx, http.MethodPost, "/v1/companies", nil, body, &co); err != nil {
		return nil, err
	}
	return compactCompany(co, true), nil
}

func updateCompany(ctx context.Context, c *caller, raw json.RawMessage) (any, error) {
	a, err := decode[struct {
		Company      string         `json:"company"`
		Name         *string        `json:"name"`
		Domain       *string        `json:"domain"`
		ExternalID   *string        `json:"external_id"`
		AddLabels    []string       `json:"add_labels"`
		RemoveLabels []string       `json:"remove_labels"`
		Notes        *string        `json:"notes"`
		Fields       map[string]any `json:"fields"`
	}](raw)
	if err != nil {
		return nil, err
	}
	id, err := companyID(ctx, c, a.Company)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if a.Name != nil {
		n := clip(*a.Name, 200)
		if n == "" {
			return nil, invalid("name cannot be empty")
		}
		body["name"] = n
	}
	// The API clears an external id with null and the other text fields with "".
	if a.Domain != nil {
		body["domain"] = clip(*a.Domain, 253)
	}
	if a.Notes != nil {
		body["notes"] = strings.TrimSpace(*a.Notes)
	}
	if err := setStr(body, "external_id", a.ExternalID, 200, true); err != nil {
		return nil, err
	}
	add, err := cleanLabels(a.AddLabels)
	if err != nil {
		return nil, err
	}
	remove, err := cleanLabels(a.RemoveLabels)
	if err != nil {
		return nil, err
	}
	if len(add) > 0 {
		body["add_tags"] = add
	}
	if len(remove) > 0 {
		body["remove_tags"] = remove
	}
	if len(a.Fields) > 0 {
		body["fields"] = a.Fields
	}
	if len(body) == 0 {
		return nil, invalid("nothing to change: pass name, domain, external_id, notes, add_labels, remove_labels or fields")
	}
	var co apiCompany
	if err := c.do(ctx, http.MethodPatch, "/v1/companies/"+id, nil, body, &co); err != nil {
		return nil, err
	}
	return compactCompany(co, true), nil
}

func searchCompanies(ctx context.Context, c *caller, raw json.RawMessage) (any, error) {
	a, err := decode[struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}](raw)
	if err != nil {
		return nil, err
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 10
	}
	q := url.Values{"limit": {strconv.Itoa(min(limit, 50))}}
	if s := strings.TrimSpace(a.Query); s != "" {
		q.Set("q", clip(s, 200))
	}
	var pg page[apiCompany]
	if err := c.get(ctx, "/v1/companies", q, &pg); err != nil {
		return nil, err
	}
	out := struct {
		Companies []outCompanyFull `json:"companies"`
		Count     int              `json:"count"`
		More      bool             `json:"more,omitempty"`
	}{Companies: []outCompanyFull{}, More: pg.NextCursor != nil}
	for _, co := range pg.Data {
		out.Companies = append(out.Companies, compactCompany(co, false))
	}
	out.Count = len(out.Companies)
	return out, nil
}

func getCompany(ctx context.Context, c *caller, raw json.RawMessage) (any, error) {
	a, err := decode[struct {
		Company string `json:"company"`
	}](raw)
	if err != nil {
		return nil, err
	}
	id, err := companyID(ctx, c, a.Company)
	if err != nil {
		return nil, err
	}
	var co apiCompany
	if err := c.get(ctx, "/v1/companies/"+id, nil, &co); err != nil {
		return nil, err
	}
	return struct {
		Company outCompanyFull `json:"company"`
	}{compactCompany(co, true)}, nil
}
