package commit

import (
	"fmt"
	"sync"
)

// memTxStore 内存事务存储
type memTxStore struct {
	mu  sync.RWMutex
	txs map[string]*CommitTransaction
}

func newMemTxStore() *memTxStore {
	return &memTxStore{txs: make(map[string]*CommitTransaction)}
}

func (s *memTxStore) Save(tx *CommitTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *tx
	s.txs[tx.TxID] = &cp
	return nil
}

func (s *memTxStore) Update(tx *CommitTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *tx
	s.txs[tx.TxID] = &cp
	return nil
}

func (s *memTxStore) GetByID(txID string) (*CommitTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.txs[txID]
	if !ok {
		return nil, fmt.Errorf("tx not found: %s", txID)
	}
	cp := *tx
	return &cp, nil
}

func (s *memTxStore) List(filter *TxFilter) ([]*CommitTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*CommitTransaction
	for _, tx := range s.txs {
		if filter != nil {
			if filter.TxType != nil && tx.TxType != *filter.TxType {
				continue
			}
			if filter.TxPhase != nil && tx.TxPhase != *filter.TxPhase {
				continue
			}
			if filter.TargetSessionID != nil && tx.TargetSessionID != *filter.TargetSessionID {
				continue
			}
		}
		cp := *tx
		result = append(result, &cp)
	}
	return result, nil
}

func (s *memTxStore) GetActive() ([]*CommitTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*CommitTransaction
	for _, tx := range s.txs {
		if !IsTerminal(tx.TxPhase) {
			cp := *tx
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (s *memTxStore) GetIncomplete() ([]*CommitTransaction, error) {
	return s.GetActive()
}

// memControlState 内存 ControlState
type memControlState struct {
	mu                  sync.RWMutex
	epoch               uint64
	lastSuccessfulEpoch uint64
	activeTxID          string
	rollbackMarker      uint64
	controlHealth       string
}

func newMemControlState(epoch uint64) *memControlState {
	return &memControlState{epoch: epoch, lastSuccessfulEpoch: epoch, controlHealth: "Healthy"}
}

func (s *memControlState) GetEpoch() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epoch
}
func (s *memControlState) GetLastSuccessfulEpoch() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSuccessfulEpoch
}
func (s *memControlState) GetActiveTxID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeTxID
}
func (s *memControlState) SetActiveTxID(txID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeTxID = txID
	return nil
}
func (s *memControlState) IncrementEpoch() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch++
	return s.epoch, nil
}
func (s *memControlState) SetLastSuccessfulEpoch(epoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSuccessfulEpoch = epoch
	return nil
}
func (s *memControlState) SetRollbackMarker(epoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rollbackMarker = epoch
	return nil
}
func (s *memControlState) GetRollbackMarker() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rollbackMarker
}
func (s *memControlState) RestoreEpoch(epoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch = epoch
	return nil
}
func (s *memControlState) SetControlHealth(health string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controlHealth = health
	return nil
}

// memSessionState 内存 SessionState
type memSessionState struct {
	mu       sync.RWMutex
	sessions map[string]map[string]interface{}
}

func newMemSessionState() *memSessionState {
	return &memSessionState{sessions: make(map[string]map[string]interface{})}
}

func (s *memSessionState) AddSession(id string, data map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = data
}

func (s *memSessionState) GetSession(sessionID string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return sess, nil
}

func (s *memSessionState) UpdateLink(sessionID, linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess["current_link_id"] = linkID
	}
	return nil
}

func (s *memSessionState) UpdateGateway(sessionID, gatewayID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess["gateway_id"] = gatewayID
	}
	return nil
}

func (s *memSessionState) UpdateSurvivalMode(sessionID, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess["current_survival_mode"] = mode
	}
	return nil
}

// memLinkState 内存 LinkState
type memLinkState struct {
	mu    sync.RWMutex
	links map[string]map[string]interface{}
}

func newMemLinkState() *memLinkState {
	return &memLinkState{links: make(map[string]map[string]interface{})}
}

func (s *memLinkState) AddLink(id string, data map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[id] = data
}

func (s *memLinkState) GetLink(linkID string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	link, ok := s.links[linkID]
	if !ok {
		return nil, fmt.Errorf("link not found: %s", linkID)
	}
	return link, nil
}
