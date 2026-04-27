package provisioning

import (
	"testing"
	"time"
)

func TestBurnLink_RedeemOnce(t *testing.T) {
	p := &Provisioner{
		burnLinks:      make(map[string]*BurnLink),
		linkTTL:        5 * time.Minute,
		maxAccessCount: 1,
	}
	link := &BurnLink{
		Token:     "test-token",
		UID:       "test-uid",
		Payload:   nil,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		MaxAccess: 1,
	}
	p.burnLinks["test-token"] = link

	exists, consumed, expired := p.GetLinkStatus("test-token")
	if !exists || consumed || expired {
		t.Fatalf("link should exist, not consumed, not expired")
	}
}

func TestBurnLink_NonExistent(t *testing.T) {
	p := &Provisioner{
		burnLinks: make(map[string]*BurnLink),
	}
	exists, _, _ := p.GetLinkStatus("nonexistent")
	if exists {
		t.Fatal("nonexistent link should not exist")
	}
}

func TestBurnLink_Expired(t *testing.T) {
	p := &Provisioner{
		burnLinks: make(map[string]*BurnLink),
	}
	link := &BurnLink{
		Token:     "expired-token",
		UID:       "test-uid",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
		MaxAccess: 1,
	}
	p.burnLinks["expired-token"] = link

	_, _, expired := p.GetLinkStatus("expired-token")
	if !expired {
		t.Fatal("link should be expired")
	}
}

func TestCleanExpiredLinks(t *testing.T) {
	p := &Provisioner{
		burnLinks: make(map[string]*BurnLink),
	}
	p.burnLinks["expired"] = &BurnLink{ExpiresAt: time.Now().Add(-1 * time.Hour)}
	p.burnLinks["valid"] = &BurnLink{ExpiresAt: time.Now().Add(1 * time.Hour)}

	p.CleanExpiredLinks()

	if _, ok := p.burnLinks["expired"]; ok {
		t.Fatal("expired link should be cleaned")
	}
	if _, ok := p.burnLinks["valid"]; !ok {
		t.Fatal("valid link should remain")
	}
}
