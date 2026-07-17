package diet

import (
	"context"
	"fmt"
	"strings"

	"github.com/homemate/server/internal/mcpmanager"
)

// FoodItem represents a single food item in a meal.
type FoodItem struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
}

// MealAnalysis holds the nutritional analysis result for a meal.
type MealAnalysis struct {
	Foods        []FoodItem    `json:"foods"`
	TotalCalories float64      `json:"total_calories"`
	ProteinG      float64      `json:"protein_g"`
	CarbsG        float64      `json:"carbs_g"`
	FatG          float64      `json:"fat_g"`
	FiberG        float64      `json:"fiber_g"`
	SodiumMg      float64      `json:"sodium_mg"`
	Warnings      []string      `json:"warnings,omitempty"`
}

// AllergyCheck holds the result of an allergy check.
type AllergyCheck struct {
	MemberID     string   `json:"member_id"`
	Ingredients  []string `json:"ingredients"`
	Allergens    []string `json:"allergens,omitempty"`
	IsSafe       bool     `json:"is_safe"`
	Warnings     []string `json:"warnings,omitempty"`
}

// DietSuggestion provides personalized diet suggestions.
type DietSuggestion struct {
	MemberID     string   `json:"member_id"`
	Suggestions  []string `json:"suggestions"`
	DailyCalorie int      `json:"daily_calorie_target"`
	DailyProtein int      `json:"daily_protein_target"`
}

// DietService provides diet and nutrition functionality.
type DietService struct {
	mcpManager *mcpmanager.MCPClientManager
	registry   *mcpmanager.Registry
	store      DietStore
}

// DietStore defines the interface for diet-related data persistence.
type DietStore interface {
	GetMemberAllergies(ctx context.Context, memberID string) ([]string, error)
	GetMemberDietProfile(ctx context.Context, memberID string) (*DietProfile, error)
}

// DietProfile holds a member's diet preferences and restrictions.
type DietProfile struct {
	MemberID      string   `json:"member_id"`
	Allergies     []string `json:"allergies"`
	DietType      string   `json:"diet_type"` // omnivore, vegetarian, vegan, keto
	CalorieTarget int      `json:"calorie_target"`
	Goals         []string `json:"goals"`
}

// NewDietService creates a new diet service.
func NewDietService(mcpManager *mcpmanager.MCPClientManager, registry *mcpmanager.Registry, store DietStore) *DietService {
	return &DietService{
		mcpManager: mcpManager,
		registry:   registry,
		store:      store,
	}
}

// AnalyzeMeal analyzes the nutritional content of a meal.
func (s *DietService) AnalyzeMeal(ctx context.Context, foodList []FoodItem) (*MealAnalysis, error) {
	servers := s.registry.GetToolsForCategory(mcpmanager.CategoryFood)
	if len(servers) == 0 {
		// Fallback: basic calculation without MCP
		return s.basicMealAnalysis(foodList), nil
	}

	foodNames := make([]string, len(foodList))
	for i, f := range foodList {
		foodNames[i] = f.Name
	}

	result, err := s.mcpManager.CallTool(ctx, servers[0].Name, "analyzeNutrition", map[string]interface{}{
		"foods": foodNames,
	})
	if err != nil {
		return nil, fmt.Errorf("analyze meal failed: %w", err)
	}

	_ = result
	// TODO: parse MCP result into MealAnalysis
	return s.basicMealAnalysis(foodList), nil
}

// CheckAllergy checks if ingredients contain allergens for a member.
func (s *DietService) CheckAllergy(ctx context.Context, memberID string, ingredients []string) (*AllergyCheck, error) {
	knownAllergies, err := s.store.GetMemberAllergies(ctx, memberID)
	if err != nil {
		return nil, fmt.Errorf("fetch allergies failed: %w", err)
	}

	check := &AllergyCheck{
		MemberID:    memberID,
		Ingredients: ingredients,
		IsSafe:      true,
	}

	for _, ingredient := range ingredients {
		for _, allergy := range knownAllergies {
			if strings.Contains(strings.ToLower(ingredient), strings.ToLower(allergy)) {
				check.Allergens = append(check.Allergens, allergy)
				check.IsSafe = false
				check.Warnings = append(check.Warnings,
					fmt.Sprintf("成分 '%s' 可能含有过敏原 '%s'", ingredient, allergy))
			}
		}
	}

	return check, nil
}

// GetDietSuggestions returns personalized diet suggestions for a member.
func (s *DietService) GetDietSuggestions(ctx context.Context, memberID string) (*DietSuggestion, error) {
	profile, err := s.store.GetMemberDietProfile(ctx, memberID)
	if err != nil {
		return nil, fmt.Errorf("fetch diet profile failed: %w", err)
	}

	suggestion := &DietSuggestion{
		MemberID:     memberID,
		DailyCalorie: profile.CalorieTarget,
		DailyProtein: 60,
		Suggestions: []string{
			"每日摄入足量蔬菜（300-500克）",
			"优先选择全谷物和优质蛋白质",
			"控制添加糖和饱和脂肪的摄入",
			"保持充足的水分摄入（每日1.5-2升）",
		},
	}

	if profile.DietType == "vegetarian" {
		suggestion.Suggestions = append(suggestion.Suggestions,
			"注意补充维生素B12和铁质",
			"多食用豆类和坚果补充蛋白质")
	}

	if profile.DietType == "keto" {
		suggestion.Suggestions = append(suggestion.Suggestions,
			"严格控制碳水化合物摄入在50克/天以下",
			"增加健康脂肪的摄入比例")
	}

	return suggestion, nil
}

// basicMealAnalysis provides a fallback meal analysis without external MCP.
func (s *DietService) basicMealAnalysis(foodList []FoodItem) *MealAnalysis {
	analysis := &MealAnalysis{
		Foods:    foodList,
		Warnings: []string{},
	}

	// Very rough estimates for demo
	for _, f := range foodList {
		switch strings.ToLower(f.Name) {
		case "米饭", "rice":
			analysis.TotalCalories += 200 * f.Quantity
			analysis.CarbsG += 45 * f.Quantity
		case "鸡胸肉", "chicken breast":
			analysis.TotalCalories += 165 * f.Quantity
			analysis.ProteinG += 31 * f.Quantity
		case "牛肉", "beef":
			analysis.TotalCalories += 250 * f.Quantity
			analysis.ProteinG += 26 * f.Quantity
			analysis.FatG += 17 * f.Quantity
		case "鸡蛋", "egg":
			analysis.TotalCalories += 70 * f.Quantity
			analysis.ProteinG += 6 * f.Quantity
			analysis.FatG += 5 * f.Quantity
		case "苹果", "apple":
			analysis.TotalCalories += 52 * f.Quantity
			analysis.CarbsG += 14 * f.Quantity
			analysis.FiberG += 2.4 * f.Quantity
		default:
			analysis.TotalCalories += 100 * f.Quantity
			analysis.ProteinG += 5 * f.Quantity
			analysis.CarbsG += 15 * f.Quantity
			analysis.FatG += 3 * f.Quantity
		}
	}

	return analysis
}
