package statequery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mirage-os/pkg/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.V2LinkState{}, &models.V2SessionState{}, &models.V2ControlState{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func setupTestServer(db *gorm.DB) *http.ServeMux {
	mux := http.NewServeMux()
	h := NewHandler(db)
	h.RegisterRoutes(mux)
	return mux
}

func seedLink(t *testing.T, db *gorm.DB, linkID, gatewayID, phase string) {
	now := time.Now()
	link := models.V2LinkState{
		LinkID:        linkID,
		TransportType: "quic",
		GatewayID:     gatewayID,
		Phase:         phase,
		Available:     phase == "Active",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("seed link failed: %v", err)
	}
}

func seedSession(t *testing.T, db *gorm.DB, sessionID, userID, gatewayID, linkID, state string) {
	now := time.Now()
	ss := models.V2SessionState{
		SessionID:     sessionID,
		UserID:        userID,
		ClientID:      "client-1",
		GatewayID:     gatewayID,
		ServiceClass:  "Standard",
		Priority:      50,
		CurrentLinkID: linkID,
		State:         state,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := db.Create(&ss).Error; err != nil {
		t.Fatalf("seed session failed: %v", err)
	}
}

func seedControl(t *testing.T, db *gorm.DB, gatewayID string) {
	cs := models.V2ControlState{
		GatewayID:     gatewayID,
		Epoch:         5,
		ControlHealth: "Healthy",
		UpdatedAt:     time.Now(),
	}
	if err := db.Create(&cs).Error; err != nil {
		t.Fatalf("seed control failed: %v", err)
	}
}

// === Link 端点测试 ===

func TestGetLinks(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)
	seedLink(t, db, "link-1", "gw-1", "Active")
	seedLink(t, db, "link-2", "gw-1", "Probing")

	req := httptest.NewRequest("GET", "/api/v2/links?gateway_id=gw-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var links []models.V2LinkState
	json.NewDecoder(w.Body).Decode(&links)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
}

func TestGetLinkDetail(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)
	seedLink(t, db, "link-1", "gw-1", "Active")

	req := httptest.NewRequest("GET", "/api/v2/links/link-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetLinkDetail404(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)

	req := httptest.NewRequest("GET", "/api/v2/links/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// === Session 端点测试 ===

func TestGetSessions(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)
	seedSession(t, db, "sess-1", "user-1", "gw-1", "link-1", "Active")
	seedSession(t, db, "sess-2", "user-2", "gw-1", "link-1", "Degraded")

	req := httptest.NewRequest("GET", "/api/v2/sessions?gateway_id=gw-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var sessions []models.V2SessionState
	json.NewDecoder(w.Body).Decode(&sessions)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestGetSessionsFilterByState(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)
	seedSession(t, db, "sess-1", "user-1", "gw-1", "link-1", "Active")
	seedSession(t, db, "sess-2", "user-2", "gw-1", "link-1", "Degraded")

	req := httptest.NewRequest("GET", "/api/v2/sessions?state=Active", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var sessions []models.V2SessionState
	json.NewDecoder(w.Body).Decode(&sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestGetSessionDetail404(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)

	req := httptest.NewRequest("GET", "/api/v2/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// === Topology 端点测试 ===

func TestGetSessionTopology(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)
	seedLink(t, db, "link-1", "gw-1", "Active")
	seedSession(t, db, "sess-1", "user-1", "gw-1", "link-1", "Active")
	seedControl(t, db, "gw-1")

	req := httptest.NewRequest("GET", "/api/v2/sessions/sess-1/topology", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var topo map[string]json.RawMessage
	json.NewDecoder(w.Body).Decode(&topo)
	if _, ok := topo["session"]; !ok {
		t.Fatal("topology missing session")
	}
	if _, ok := topo["link"]; !ok {
		t.Fatal("topology missing link")
	}
	if _, ok := topo["control"]; !ok {
		t.Fatal("topology missing control")
	}
}

// === Control 端点测试 ===

func TestGetControl(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)
	seedControl(t, db, "gw-1")

	req := httptest.NewRequest("GET", "/api/v2/control/gw-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetControl404(t *testing.T) {
	db := setupTestDB(t)
	mux := setupTestServer(db)

	req := httptest.NewRequest("GET", "/api/v2/control/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
