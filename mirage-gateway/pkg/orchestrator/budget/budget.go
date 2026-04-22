package budget

import "gorm.io/gorm"

// BudgetEngine holds the assembled Budget Engine components
type BudgetEngine struct {
	BudgetChecker       *BudgetCheckerImpl
	ServiceClassChecker *ServiceClassCheckerImpl
	Ledger              BudgetLedger
	Store               BudgetProfileStore
}

// NewBudgetEngine creates a fully assembled Budget Engine
func NewBudgetEngine(db *gorm.DB, sessionGetter SessionGetter) *BudgetEngine {
	costModel := NewDefaultCostModel()
	slaPolicy := NewDefaultSLAPolicy()
	ledger := NewInMemoryLedger()
	store := NewGormBudgetProfileStore(db)

	budgetChecker := NewBudgetCheckerImpl(costModel, slaPolicy, ledger, store)
	serviceClassChecker := NewServiceClassCheckerImpl(slaPolicy, sessionGetter)

	return &BudgetEngine{
		BudgetChecker:       budgetChecker,
		ServiceClassChecker: serviceClassChecker,
		Ledger:              ledger,
		Store:               store,
	}
}
