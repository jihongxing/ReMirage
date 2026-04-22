package personaquery

import (
	"encoding/json"
	"mirage-os/pkg/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.PersonaManifest{}, &models.V2SessionState{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedManifest(t *testing.T, db *gorm.DB, personaID string, version uint64, lifecycle string) {
	t.Helper()
	m := models.PersonaManifest{
		PersonaID:            personaID,
		Version:              version,
		Epoch:                1,
		Checksum:             "abc123",
		HandshakeProfileID:   "h1",
		PacketShapeProfileID: "p1",
		TimingProfileID:      "t1",
		BackgroundProfileID:  "b1",
		Lifecycle:            lifecycle,
		CreatedAt:            time.Now().UTC(),
	}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
}

func seedSession(t *testing.T, db *gorm.DB, sessionID, personaID string) {
	t.Helper()
	s := models.V2SessionState{
		SessionID:        sessionID,
		UserID:           "user-1",
		ClientID:         "client-1",
		GatewayID:        "gw-1",
		ServiceClass:     "Standard",
		CurrentPersonaID: personaID,
		State:            "Active",
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

// GET /api/v2/personas/{persona_id} - 正常返回最新版本
func TestGetLatestPersona(t *testing.T) {
	db := setupTestDB(t)
	seedManifest(t, db, "p1", 1, models.PersonaLifecycleActive)
	seedManifest(t, db, "p1", 2, models.PersonaLifecycleActive)

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/personas/p1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200, body=%s", w.Code, w.Body.String())
	}

	var result models.PersonaManifest
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("version=%d, want 2", result.Version)
	}
}

// GET /api/v2/personas/{persona_id} - 404
func TestGetLatestPersona_NotFound(t *testing.T) {
	db := setupTestDB(t)
	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/personas/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

// GET /api/v2/personas/{persona_id}/versions - 版本列表降序
func TestListVersions(t *testing.T) {
	db := setupTestDB(t)
	seedManifest(t, db, "p1", 1, models.PersonaLifecycleRetired)
	seedManifest(t, db, "p1", 2, models.PersonaLifecycleCooling)
	seedManifest(t, db, "p1", 3, models.PersonaLifecycleActive)

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/personas/p1/versions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200, body=%s", w.Code, w.Body.String())
	}

	var results []models.PersonaManifest
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("count=%d, want 3", len(results))
	}
	// 验证降序
	for i := 1; i < len(results); i++ {
		if results[i].Version >= results[i-1].Version {
			t.Fatalf("versions not descending: %d >= %d", results[i].Version, results[i-1].Version)
		}
	}
}

// GET /api/v2/personas/{persona_id}/versions - 404
func TestListVersions_NotFound(t *testing.T) {
	db := setupTestDB(t)
	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/personas/nonexistent/versions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

// GET /api/v2/sessions/{session_id}/persona - 正常返回
func TestGetSessionPersona(t *testing.T) {
	db := setupTestDB(t)
	seedManifest(t, db, "p1", 1, models.PersonaLifecycleActive)
	seedSession(t, db, "s1", "p1")

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sessions/s1/persona", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200, body=%s", w.Code, w.Body.String())
	}

	var result models.PersonaManifest
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.PersonaID != "p1" {
		t.Fatalf("persona_id=%s, want p1", result.PersonaID)
	}
}

// GET /api/v2/sessions/{session_id}/persona - 404
func TestGetSessionPersona_NotFound(t *testing.T) {
	db := setupTestDB(t)
	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sessions/nonexistent/persona", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

// 验证 JSON 响应时间戳格式
func TestJSONTimestampFormat(t *testing.T) {
	db := setupTestDB(t)
	seedManifest(t, db, "p1", 1, models.PersonaLifecycleActive)

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/personas/p1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var createdAt string
	if err := json.Unmarshal(raw["created_at"], &createdAt); err != nil {
		t.Fatalf("unmarshal created_at: %v", err)
	}

	if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
		// 也尝试 RFC3339Nano
		if _, err2 := time.Parse(time.RFC3339Nano, createdAt); err2 != nil {
			t.Fatalf("created_at not RFC 3339: %s", createdAt)
		}
	}
}
