package billing

import "testing"

func TestProcessExpiredPure_AutoRenew_SufficientBalance(t *testing.T) {
	// Diamond plan costs $2999/month, user has $5000
	newLevel := ProcessExpiredPure(5000.0, 3, true, "plan_diamond_monthly")
	if newLevel != 3 {
		t.Fatalf("expected level 3 (auto-renewed), got %d", newLevel)
	}
}

func TestProcessExpiredPure_AutoRenew_InsufficientBalance(t *testing.T) {
	// Diamond plan costs $2999/month, user has $10
	newLevel := ProcessExpiredPure(10.0, 3, true, "plan_diamond_monthly")
	if newLevel != 1 {
		t.Fatalf("expected level 1 (downgraded), got %d", newLevel)
	}
}

func TestProcessExpiredPure_NoAutoRenew(t *testing.T) {
	// Even with huge balance, no auto-renew → downgrade
	newLevel := ProcessExpiredPure(100000.0, 2, false, "plan_platinum_monthly")
	if newLevel != 1 {
		t.Fatalf("expected level 1 (downgraded, no auto-renew), got %d", newLevel)
	}
}

func TestProcessExpiredPure_AlreadyStandard(t *testing.T) {
	newLevel := ProcessExpiredPure(0, 1, false, "")
	if newLevel != 1 {
		t.Fatalf("expected level 1 (already standard), got %d", newLevel)
	}
}
