package transactionquery

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
	if err := db.AutoMigrate(&models.CommitTransaction{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedTx(t *testing.T, db *gorm.DB, txID, txType, txPhase, txScope string) {
	t.Helper()
	tx := models.CommitTransaction{
		TxID:          txID,
		TxType:        txType,
		TxPhase:       txPhase,
		TxScope:       txScope,
		CreatedAt:     time.Now().UTC(),
		PrepareState:  json.RawMessage(`{}`),
		ValidateState: json.RawMessage(`{}`),
		ShadowState:   json.RawMessage(`{}`),
		FlipState:     json.RawMessage(`{}`),
		AckState:      json.RawMessage(`{}`),
		CommitState:   json.RawMessage(`{}`),
	}
	if err := db.Create(&tx).Error; err != nil {
		t.Fatalf("seed tx: %v", err)
	}
}

func TestGetTransaction(t *testing.T) {
	db := setupTestDB(t)
	seedTx(t, db, "tx-1", "PersonaSwitch", "Committed", "Session")

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/transactions/tx-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200, body=%s", w.Code, w.Body.String())
	}

	var result models.CommitTransaction
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.TxID != "tx-1" {
		t.Fatalf("tx_id=%s, want tx-1", result.TxID)
	}
}

func TestGetTransaction_NotFound(t *testing.T) {
	db := setupTestDB(t)
	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/transactions/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestListTransactions(t *testing.T) {
	db := setupTestDB(t)
	seedTx(t, db, "tx-1", "PersonaSwitch", "Committed", "Session")
	seedTx(t, db, "tx-2", "LinkMigration", "Preparing", "Link")

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/transactions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}

	var results []models.CommitTransaction
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("count=%d, want 2", len(results))
	}
}

func TestListTransactions_FilterByType(t *testing.T) {
	db := setupTestDB(t)
	seedTx(t, db, "tx-1", "PersonaSwitch", "Committed", "Session")
	seedTx(t, db, "tx-2", "LinkMigration", "Preparing", "Link")

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/transactions?tx_type=PersonaSwitch", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var results []models.CommitTransaction
	_ = json.Unmarshal(w.Body.Bytes(), &results)
	if len(results) != 1 {
		t.Fatalf("count=%d, want 1", len(results))
	}
}

func TestGetActiveTransactions(t *testing.T) {
	db := setupTestDB(t)
	seedTx(t, db, "tx-1", "PersonaSwitch", "Committed", "Session")
	seedTx(t, db, "tx-2", "LinkMigration", "Preparing", "Link")
	seedTx(t, db, "tx-3", "SurvivalModeSwitch", "Flipping", "Global")

	h := NewHandler(db)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/transactions/active", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}

	var results []models.CommitTransaction
	_ = json.Unmarshal(w.Body.Bytes(), &results)
	if len(results) != 2 {
		t.Fatalf("count=%d, want 2 (Preparing + Flipping)", len(results))
	}
}
