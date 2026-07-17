package finance

import (
	"context"
	"fmt"
	"time"
)

// ExpenseCategory represents the category of an expense.
type ExpenseCategory string

const (
	CategoryFood      ExpenseCategory = "餐饮"
	CategoryTransport ExpenseCategory = "交通"
	CategoryShopping  ExpenseCategory = "购物"
	CategoryHousing   ExpenseCategory = "住房"
	CategoryEntertainment ExpenseCategory = "娱乐"
	CategoryMedical   ExpenseCategory = "医疗"
	CategoryEducation ExpenseCategory = "教育"
	CategoryOther     ExpenseCategory = "其他"
)

// Expense represents a single expense record.
type Expense struct {
	ID        string          `json:"id"`
	MemberID  string          `json:"member_id"`
	Amount    float64         `json:"amount"`
	Category  ExpenseCategory `json:"category"`
	Note      string          `json:"note"`
	Date      time.Time       `json:"date"`
	CreatedAt time.Time       `json:"created_at"`
}

// MonthlyReport holds expense data for a specific month.
type MonthlyReport struct {
	Month        string                 `json:"month"`
	TotalAmount  float64                `json:"total_amount"`
	ByCategory   map[string]float64     `json:"by_category"`
	Transactions []Expense              `json:"transactions"`
	DailyAvg     float64                `json:"daily_avg"`
}

// BudgetStatus holds current budget tracking status.
type BudgetStatus struct {
	Month         string             `json:"month"`
	TotalBudget   float64            `json:"total_budget"`
	TotalSpent    float64            `json:"total_spent"`
	Remaining     float64            `json:"remaining"`
	ByCategory    map[string]float64 `json:"by_category"`
	Warnings      []string           `json:"warnings,omitempty"`
}

// FinanceTracker provides local finance tracking functionality.
type FinanceTracker struct {
	store FinanceStore
}

// FinanceStore defines the interface for finance data persistence.
type FinanceStore interface {
	SaveExpense(ctx context.Context, expense *Expense) error
	GetExpensesByMemberAndMonth(ctx context.Context, memberID string, month string) ([]Expense, error)
	GetExpensesByMember(ctx context.Context, memberID string, start, end time.Time) ([]Expense, error)
	DeleteExpense(ctx context.Context, expenseID string) error
}

// NewFinanceTracker creates a new finance tracker.
func NewFinanceTracker(store FinanceStore) *FinanceTracker {
	return &FinanceTracker{store: store}
}

// RecordExpense records a new expense.
func (t *FinanceTracker) RecordExpense(ctx context.Context, memberID string, amount float64, category ExpenseCategory, note string) (*Expense, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than zero")
	}

	expense := &Expense{
		ID:        generateID(),
		MemberID:  memberID,
		Amount:    amount,
		Category:  category,
		Note:      note,
		Date:      time.Now(),
		CreatedAt: time.Now(),
	}

	if err := t.store.SaveExpense(ctx, expense); err != nil {
		return nil, fmt.Errorf("save expense failed: %w", err)
	}

	return expense, nil
}

// GetMonthlyReport generates a monthly expense report.
func (t *FinanceTracker) GetMonthlyReport(ctx context.Context, memberID string, month string) (*MonthlyReport, error) {
	expenses, err := t.store.GetExpensesByMemberAndMonth(ctx, memberID, month)
	if err != nil {
		return nil, fmt.Errorf("fetch expenses failed: %w", err)
	}

	report := &MonthlyReport{
		Month:        month,
		ByCategory:   make(map[string]float64),
		Transactions: expenses,
	}

	for _, e := range expenses {
		report.TotalAmount += e.Amount
		report.ByCategory[string(e.Category)] += e.Amount
	}

	// Calculate daily average for the month
	year := time.Now().Year()
	monthNum := int(time.Now().Month())
	fmt.Sscanf(month, "%d-%d", &year, &monthNum)
	daysInMonth := time.Date(year, time.Month(monthNum+1), 0, 0, 0, 0, 0, time.UTC).Day()
	if daysInMonth > 0 {
		report.DailyAvg = report.TotalAmount / float64(daysInMonth)
	}

	return report, nil
}

// GetBudgetStatus returns the current budget status.
func (t *FinanceTracker) GetBudgetStatus(ctx context.Context, memberID string) (*BudgetStatus, error) {
	month := time.Now().Format("2006-01")
	expenses, err := t.store.GetExpensesByMemberAndMonth(ctx, memberID, month)
	if err != nil {
		return nil, fmt.Errorf("fetch expenses failed: %w", err)
	}

	status := &BudgetStatus{
		Month:      month,
		TotalBudget: 10000, // default monthly budget
		ByCategory: make(map[string]float64),
	}

	for _, e := range expenses {
		status.TotalSpent += e.Amount
		status.ByCategory[string(e.Category)] += e.Amount
	}

	status.Remaining = status.TotalBudget - status.TotalSpent

	if status.TotalSpent > status.TotalBudget {
		status.Warnings = append(status.Warnings, "本月支出已超出预算")
	}
	if status.ByCategory[string(CategoryFood)] > status.TotalBudget*0.4 {
		status.Warnings = append(status.Warnings, "餐饮支出占比过高，建议适当控制")
	}

	return status, nil
}

// generateID generates a simple unique ID for expenses.
func generateID() string {
	return fmt.Sprintf("exp_%d", time.Now().UnixNano())
}
