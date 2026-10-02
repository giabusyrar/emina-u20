package conversationflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

type memStore struct {
	flows map[string]*Flow
	n     int
}

func newMemStore() *memStore { return &memStore{flows: map[string]*Flow{}} }

func (m *memStore) Get(_ context.Context, id string) (*Flow, error) {
	f, ok := m.flows[id]
	if !ok {
		return nil, nil
	}
	c := *f
	return &c, nil
}

func (m *memStore) List(_ context.Context, brandID, appID string) ([]Flow, error) {
	var out []Flow
	for _, f := range m.flows {
		if f.BrandID == brandID && f.ApplicationID == appID {
			out = append(out, *f)
		}
	}
	return out, nil
}

func (m *memStore) Active(_ context.Context, brandID, appID, code string) (*Flow, error) {
	for _, f := range m.flows {
		if f.BrandID == brandID && f.ApplicationID == appID && f.SurveyCode == code && f.Status == StatusActive {
			c := *f
			return &c, nil
		}
	}
	return nil, nil
}

func (m *memStore) Create(_ context.Context, f *Flow) error {
	m.n++
	f.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", m.n)
	c := *f
	m.flows[f.ID] = &c
	return nil
}

func (m *memStore) Update(_ context.Context, f *Flow) error {
	c := *f
	m.flows[f.ID] = &c
	return nil
}

func newTestServer(store Store, hasFaceProfile bool) *echo.Echo {
	svc := NewService(store,
		func(_ context.Context, _, _, code string) (QuestionTypes, error) {
			if code != "skinverse_q1_q6" {
				return nil, ErrSurveyNotFound
			}
			return QuestionTypes{"wears_hijab": "boolean"}, nil
		},
		func(context.Context, string, string) (bool, error) { return hasFaceProfile, nil },
		func(_ context.Context, ref string) (string, error) {
			if ref == "wardah" || ref == "brand-uuid-wardah" {
				return "brand-uuid-wardah", nil
			}
			return "", fmt.Errorf("%w: %q", ErrUnknownBrand, ref)
		},
	)
	e := echo.New()
	NewHandler(svc).Mount(e.Group("/core/conversation-flows"))
	return e
}

const flowJSON = `{"brand_id":"wardah","application_id":"skinverse","survey_code":"skinverse_q1_q6","status":"active","locale":"id",
"persona":{"name":"Kak Gia"},
"steps":[{"engine":"score"},{"engine":"face_architecture"},{"engine":"colour","inputs":{"hijab":{"from_answer":"wears_hijab"},"hairVisible":{"value":false}}}]}`

const activePath = "/core/conversation-flows/active?brand_id=wardah&application_id=skinverse&survey_code=skinverse_q1_q6"

func do(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

type reply struct {
	Flow     Flow     `json:"flow"`
	Flows    []Flow   `json:"flows"`
	Warnings []string `json:"warnings"`
	Errors   []string `json:"errors"`
	Error    string   `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) reply {
	t.Helper()
	var r reply
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return r
}

func TestCreateWarnsWithoutFaceProfileAndServesActive(t *testing.T) {
	e := newTestServer(newMemStore(), false)
	rec := do(e, http.MethodPost, "/core/conversation-flows", flowJSON)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	r := decode(t, rec)
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "face-architecture profile") {
		t.Fatalf("warnings = %v", r.Warnings)
	}
	if r.Flow.ID == "" || r.Flow.BrandID != "brand-uuid-wardah" {
		t.Fatalf("flow = %+v", r.Flow)
	}

	rec = do(e, http.MethodGet, activePath, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("active = %d %s", rec.Code, rec.Body)
	}
	var f Flow
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	hair := f.Steps[2].Inputs["hairVisible"].Value
	if len(f.Steps) != 3 || hair == nil || *hair {
		t.Fatalf("steps = %+v", f.Steps)
	}
}

func TestCreateWithFaceProfileHasNoWarnings(t *testing.T) {
	r := decode(t, do(newTestServer(newMemStore(), true), http.MethodPost, "/core/conversation-flows", flowJSON))
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings = %v", r.Warnings)
	}
}

func TestCreateRejectsUnknownSurvey(t *testing.T) {
	body := strings.Replace(flowJSON, "skinverse_q1_q6", "nope", 1)
	rec := do(newTestServer(newMemStore(), true), http.MethodPost, "/core/conversation-flows", body)
	if rec.Code != http.StatusBadRequest || !anyContains(decode(t, rec).Errors, `survey "nope" has no active version`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestSecondActiveFlowIsRejectedButDraftIsAllowed(t *testing.T) {
	e := newTestServer(newMemStore(), true)
	do(e, http.MethodPost, "/core/conversation-flows", flowJSON)
	rec := do(e, http.MethodPost, "/core/conversation-flows", flowJSON)
	if rec.Code != http.StatusBadRequest || !anyContains(decode(t, rec).Errors, "already active") {
		t.Fatalf("second active = %d %s", rec.Code, rec.Body)
	}
	draft := strings.Replace(flowJSON, `"status":"active"`, `"status":"draft","version":2`, 1)
	if rec := do(e, http.MethodPost, "/core/conversation-flows", draft); rec.Code != http.StatusCreated {
		t.Fatalf("draft = %d %s", rec.Code, rec.Body)
	}
}

func TestUnknownBrandIs400(t *testing.T) {
	body := strings.Replace(flowJSON, `"brand_id":"wardah"`, `"brand_id":"omg"`, 1)
	if rec := do(newTestServer(newMemStore(), true), http.MethodPost, "/core/conversation-flows", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestActiveNotFoundIs404(t *testing.T) {
	if rec := do(newTestServer(newMemStore(), true), http.MethodGet, activePath, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestUpdate(t *testing.T) {
	e := newTestServer(newMemStore(), true)
	id := decode(t, do(e, http.MethodPost, "/core/conversation-flows", flowJSON)).Flow.ID

	if rec := do(e, http.MethodPut, "/core/conversation-flows/not-a-uuid", flowJSON); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-uuid id = %d", rec.Code)
	}
	if rec := do(e, http.MethodPut, "/core/conversation-flows/3f1c2b7a-0000-4000-8000-000000000000", flowJSON); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id = %d", rec.Code)
	}
	archived := strings.Replace(flowJSON, `"status":"active"`, `"status":"archived"`, 1)
	rec := do(e, http.MethodPut, "/core/conversation-flows/"+id, archived)
	if rec.Code != http.StatusOK || decode(t, rec).Flow.Status != StatusArchived {
		t.Fatalf("archive = %d %s", rec.Code, rec.Body)
	}
	if rec := do(e, http.MethodGet, activePath, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("active after archive = %d", rec.Code)
	}
}

func TestListRequiresScope(t *testing.T) {
	e := newTestServer(newMemStore(), true)
	do(e, http.MethodPost, "/core/conversation-flows", flowJSON)
	if rec := do(e, http.MethodGet, "/core/conversation-flows", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unscoped list = %d", rec.Code)
	}
	rec := do(e, http.MethodGet, "/core/conversation-flows?brand_id=wardah&application_id=skinverse", "")
	if rec.Code != http.StatusOK || len(decode(t, rec).Flows) != 1 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
}
