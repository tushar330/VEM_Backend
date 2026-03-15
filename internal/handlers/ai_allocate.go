package handlers

/*
 * AI ALLOCATION ENGINE — Constraint-Aware Heuristic Optimiser
 *
 * This engine models room allocation as a constrained bin-packing problem.
 * It uses a Best-Fit-Decreasing (BFD) heuristic to minimise capacity waste
 * while respecting negotiated pricing, family cohesion, and arrival ordering.
 *
 * DESIGN INTENT:
 *   - Current implementation: deterministic BFD heuristic (O(n·m) per run)
 *   - Designed to evolve into advanced optimisation solvers such as
 *     Integer Linear Programming (ILP) or Reinforcement Learning (RL) agents.
 *   - All state is read-only during the suggestion phase. Zero DB mutations.
 *
 * OBJECTIVES (in priority order):
 *   1. Minimise total capacity waste        (weight: 0.6)
 *   2. Minimise total accommodation cost    (weight: 0.3)
 *   3. Penalise large capacity mismatches   (weight: 0.1, quadratic)
 *
 * DETERMINISM GUARANTEE:
 *   All maps are converted to sorted slices before iteration.
 *   sort.SliceStable is used everywhere. No random range-over-map.
 */

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
)

// planValidityDuration defines how long a suggestion plan is considered fresh.
// Concurrent allocations, guest edits, or negotiation updates after this window
// may cause inventory drift. Frontend must check planValidity before applying.
const planValidityDuration = 5 * time.Minute

// optimisationEffectiveThreshold is the minimum improvement percent for the
// optimisation to be considered meaningfully better than the baseline.
// Below this threshold the plan is still valid but flagged as near-optimal.
// FIX 1: Configurable constant — adjust without touching algorithm.
const optimisationEffectiveThreshold = 5.0

// ─────────────────────────────────────────────────────────────────────────────
// DATA TYPES
// ─────────────────────────────────────────────────────────────────────────────

// FamilyGroup holds a family and all their guest data.
type FamilyGroup struct {
	FamilyID      string
	Size          int
	ArrivalDate   time.Time
	DepartureDate time.Time
	StayDuration  float64 // derived: departure - arrival in days
	Guests        []models.Guest
}

// VirtualRoom is a working copy of inventory used during simulation.
type VirtualRoom struct {
	RoomOfferID  string
	RoomName     string
	MaxCapacity  int
	Available    int
	Total        int
	PricePerRoom float64 // effective price (negotiation-aware)
}

// OptimisationContext is the full context loaded from DB for Phase 1.
type OptimisationContext struct {
	EventID         uuid.UUID
	EventStatus     string
	UnallocatedFams []FamilyGroup
	BaseInventory   []VirtualRoom              // deep copy for baseline simulation
	OptInventory    []VirtualRoom              // deep copy for heuristic optimisation
	RoomOfferMap    map[string]models.RoomOffer // roomOfferID → RoomOffer
}

// SuggestionItem is one suggested assignment returned to the client.
// FIX 3: reason field removed — per-family heuristic guess was misleading.
// confidenceScore remains: reflects objective packing tightness [0,1].
type SuggestionItem struct {
	FamilyID        string  `json:"familyId"`
	RoomOfferID     string  `json:"roomOfferId"`
	RoomName        string  `json:"roomName"`
	FamilySize      int     `json:"familySize"`
	Capacity        int     `json:"capacity"`
	AllocationScore float64 `json:"allocationScore"`
	CapacityWaste   int     `json:"capacityWaste"`
	RoomPrice       float64 `json:"roomPrice"`
	ConfidenceScore float64 `json:"confidenceScore"` // packing tightness [0,1]
}

// SimResult is produced by baseline or optimised simulation.
type SimResult struct {
	Suggestions         []SuggestionItem
	TotalWaste          int
	TotalCost           float64
	RoomsUsed           int
	FamiliesSkipped     int
	UnplaceableFamilies []string
}

// PlanValidity communicates the freshness window to the frontend (Phase 3).
type PlanValidity struct {
	GeneratedAt      time.Time `json:"generatedAt"`
	ValidityExpiresAt time.Time `json:"validityExpiresAt"`
	ValidForSeconds  int       `json:"validForSeconds"`
}

// AIAllocateResponse is the full JSON response.
// FIX 1: optimisationEffective + optimisationMessage added for UX clarity.
type AIAllocateResponse struct {
	Suggestions            []SuggestionItem `json:"suggestions"`
	UnplaceableFamilies    []string         `json:"unplaceableFamilies"`
	Metrics                AIMetrics        `json:"metrics"`
	Reasoning              string           `json:"reasoning"`
	PlanValidity           PlanValidity     `json:"planValidity"`
	OptimisationEffective  bool             `json:"optimisationEffective"`  // FIX 1
	OptimisationMessage    string           `json:"optimisationMessage"`    // FIX 1
}

// AIMetrics bundles all computed quality numbers.
type AIMetrics struct {
	CapacityWasteBefore           int     `json:"capacityWasteBefore"`
	CapacityWasteAfter            int     `json:"capacityWasteAfter"`
	ImprovementPercent            float64 `json:"improvementPercent"`
	TotalCostOptimised            float64 `json:"totalCostOptimised"`
	RoomsUsedBefore               int     `json:"roomsUsedBefore"`
	RoomsUsedAfter                int     `json:"roomsUsedAfter"`
	UtilisationImprovementPercent float64 `json:"utilisationImprovementPercent"`
}

// ─────────────────────────────────────────────────────────────────────────────
// PHASE 1 — DATA EXTRACTION
// ─────────────────────────────────────────────────────────────────────────────

// GetOptimisationContext fetches all data needed for the AI engine from DB.
// It is READ-ONLY and never modifies any row.
func GetOptimisationContext(db *gorm.DB, eventUUID uuid.UUID) (*OptimisationContext, error) {

	// 1a. Fetch event
	var event models.Event
	if err := db.First(&event, "id = ?", eventUUID).Error; err != nil {
		return nil, fmt.Errorf("event not found: %w", err)
	}

	// 1b. Parse JSONB inventory
	var inventoryItems []RoomsInventoryItem
	if len(event.RoomsInventory) > 0 {
		if err := json.Unmarshal(event.RoomsInventory, &inventoryItems); err != nil {
			return nil, fmt.Errorf("failed to parse rooms_inventory JSONB: %w", err)
		}
	}

	// 1c. Collect all RoomOffer IDs from inventory (slice, not map — deterministic)
	roomOfferIDs := make([]string, 0, len(inventoryItems))
	for _, item := range inventoryItems {
		roomOfferIDs = append(roomOfferIDs, item.RoomOfferID)
	}

	// 1d. Fetch matching RoomOffer rows
	var roomOffers []models.RoomOffer
	if len(roomOfferIDs) > 0 {
		if err := db.Where("id IN ?", roomOfferIDs).Find(&roomOffers).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch room offers: %w", err)
		}
	}
	roomOfferMap := make(map[string]models.RoomOffer, len(roomOffers))
	for _, ro := range roomOffers {
		roomOfferMap[ro.ID] = ro
	}

	// 1e. Resolve effective price per room (price hierarchy: cart → inventory → offer)
	effectivePrice := func(roomOfferID string, fallback float64) float64 {
		return resolveRoomEffectivePrice(db, eventUUID, roomOfferID, fallback)
	}

	// 1f. Build virtual room slices (base + optimised are independent deep copies)
	baseInventory := make([]VirtualRoom, len(inventoryItems))
	optInventory := make([]VirtualRoom, len(inventoryItems))
	for i, item := range inventoryItems {
		price := effectivePrice(item.RoomOfferID, float64(item.PricePerRoom))
		baseInventory[i] = VirtualRoom{
			RoomOfferID:  item.RoomOfferID,
			RoomName:     item.RoomName,
			MaxCapacity:  item.MaxCapacity,
			Available:    item.Available,
			Total:        item.Total,
			PricePerRoom: price,
		}
		optInventory[i] = baseInventory[i]
	}

	// 1g. Fetch existing allocations to identify unallocated guests
	var allAllocations []models.GuestAllocation
	if err := db.Where("event_id = ?", eventUUID).Find(&allAllocations).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch allocations: %w", err)
	}
	allocatedGuestIDs := make(map[uuid.UUID]bool, len(allAllocations))
	for _, a := range allAllocations {
		allocatedGuestIDs[a.GuestID] = true
	}

	// 1h. Fetch all guests, filter unallocated, group by family
	var allGuests []models.Guest
	if err := db.Where("event_id = ?", eventUUID).Find(&allGuests).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch guests: %w", err)
	}

	familyMap := make(map[string][]models.Guest)
	for _, g := range allGuests {
		if !allocatedGuestIDs[g.ID] {
			fid := g.FamilyID.String()
			familyMap[fid] = append(familyMap[fid], g)
		}
	}

	// Determinism: convert map keys to sorted slice before iteration (Phase 9, Adj 9)
	familyIDs := make([]string, 0, len(familyMap))
	for fid := range familyMap {
		familyIDs = append(familyIDs, fid)
	}
	sort.Strings(familyIDs)

	unallocatedFams := make([]FamilyGroup, 0, len(familyIDs))
	for _, fid := range familyIDs {
		members := familyMap[fid]
		if len(members) == 0 {
			continue
		}
		arrivalDate := members[0].ArrivalDate
		departureDate := members[0].DepartureDate
		stayDur := departureDate.Sub(arrivalDate).Hours() / 24.0
		if stayDur < 0 {
			stayDur = 0
		}
		if departureDate.IsZero() {
			stayDur = 1.0
		}

		unallocatedFams = append(unallocatedFams, FamilyGroup{
			FamilyID:      fid,
			Size:          len(members),
			ArrivalDate:   arrivalDate,
			DepartureDate: departureDate,
			StayDuration:  stayDur,
			Guests:        members,
		})
	}

	ctx := &OptimisationContext{
		EventID:         eventUUID,
		EventStatus:     event.Status,
		UnallocatedFams: unallocatedFams,
		BaseInventory:   baseInventory,
		OptInventory:    optInventory,
		RoomOfferMap:    roomOfferMap,
	}

	log.Printf("🔍 Optimisation context loaded: families: %d  rooms: %d  inventory entries: %d",
		len(unallocatedFams), len(roomOffers), len(inventoryItems))

	return ctx, nil
}

// resolveRoomEffectivePrice implements the price hierarchy (Phase 2 pricing):
//  1. Approved CartItem.LockedPrice  (negotiation-adjusted)
//  2. RoomsInventoryItem.PricePerRoom  (JSON snapshot)
//  3. RoomOffer.TotalFare  (fallback static price)
func resolveRoomEffectivePrice(db *gorm.DB, eventUUID uuid.UUID, roomOfferID string, inventoryFallback float64) float64 {
	var cartItem models.CartItem
	err := db.Where("event_id = ? AND ref_id = ? AND type = ? AND status = ?",
		eventUUID, roomOfferID, "room", "approved").
		First(&cartItem).Error
	if err == nil && cartItem.LockedPrice > 0 {
		return cartItem.LockedPrice
	}
	if inventoryFallback > 0 {
		return inventoryFallback
	}
	var ro models.RoomOffer
	if err := db.Where("id = ?", roomOfferID).First(&ro).Error; err == nil {
		return ro.TotalFare
	}
	return 0
}

// ─────────────────────────────────────────────────────────────────────────────
// CONTEXT VALIDATION
// ─────────────────────────────────────────────────────────────────────────────

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidateOptimisationContext checks data integrity before running the engine.
func ValidateOptimisationContext(ctx *OptimisationContext) []ValidationError {
	var errs []ValidationError

	validStatuses := map[string]bool{"active": true, "draft": true, "room_mapping": true}
	if !validStatuses[ctx.EventStatus] {
		errs = append(errs, ValidationError{
			Field:   "event_status",
			Message: fmt.Sprintf("event status '%s' does not allow optimisation", ctx.EventStatus),
		})
	}

	if len(ctx.BaseInventory) == 0 {
		errs = append(errs, ValidationError{Field: "rooms_inventory", Message: "no rooms defined in inventory"})
	}

	for i, room := range ctx.BaseInventory {
		label := fmt.Sprintf("inventory[%d] room_offer_id=%s", i, room.RoomOfferID)
		if room.Available < 0 {
			errs = append(errs, ValidationError{Field: label, Message: "available count is negative"})
		}
		if room.Total < room.Available {
			errs = append(errs, ValidationError{Field: label, Message: "total < available: data inconsistency"})
		}
		if room.MaxCapacity <= 0 {
			errs = append(errs, ValidationError{Field: label, Message: "max_capacity must be > 0"})
		}
		if _, exists := ctx.RoomOfferMap[room.RoomOfferID]; !exists {
			errs = append(errs, ValidationError{Field: label, Message: "room_offer_id not found in DB"})
		}
	}

	for _, fam := range ctx.UnallocatedFams {
		if fam.Size <= 0 {
			errs = append(errs, ValidationError{Field: "family_" + fam.FamilyID, Message: "family has zero guests"})
		}
	}

	return errs
}

// ─────────────────────────────────────────────────────────────────────────────
// BASELINE SIMULATION
// ─────────────────────────────────────────────────────────────────────────────

// SimulateBaselineAllocation mirrors the REAL AutoAllocate behaviour deterministically.
// Preserves all flaws of the original (first-fit, JSON order, silent skipping)
// to produce an HONEST before-metric for the improvement calculation.
func SimulateBaselineAllocation(ctx *OptimisationContext) SimResult {
	inv := deepCopyInventory(ctx.BaseInventory)

	fams := make([]FamilyGroup, len(ctx.UnallocatedFams))
	copy(fams, ctx.UnallocatedFams)
	// Mirror AutoAllocate: families in stable family_id order
	sort.Slice(fams, func(i, j int) bool {
		return fams[i].FamilyID < fams[j].FamilyID
	})

	result := SimResult{}

	for _, fam := range fams {
		placed := false
		for ri := range inv {
			if inv[ri].Available > 0 && inv[ri].MaxCapacity >= fam.Size {
				waste := inv[ri].MaxCapacity - fam.Size
				result.TotalWaste += waste
				result.TotalCost += inv[ri].PricePerRoom
				result.RoomsUsed++
				inv[ri].Available--

				result.Suggestions = append(result.Suggestions, SuggestionItem{
					FamilyID:        fam.FamilyID,
					RoomOfferID:     inv[ri].RoomOfferID,
					RoomName:        inv[ri].RoomName,
					FamilySize:      fam.Size,
					Capacity:        inv[ri].MaxCapacity,
					CapacityWaste:   waste,
					AllocationScore: computeScore(waste, inv[ri].PricePerRoom),
					RoomPrice:       inv[ri].PricePerRoom,
					ConfidenceScore: computeConfidence(waste, inv[ri].MaxCapacity),
				})
				placed = true
				break // first-fit: stop at first match
			}
		}
		if !placed {
			result.FamiliesSkipped++
			result.UnplaceableFamilies = append(result.UnplaceableFamilies, fam.FamilyID)
		}
	}

	log.Printf("📊 Baseline simulation: waste=%d  cost=%.2f  roomsUsed=%d  familiesSkipped=%d",
		result.TotalWaste, result.TotalCost, result.RoomsUsed, result.FamiliesSkipped)

	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// HEURISTIC OPTIMISATION
// ─────────────────────────────────────────────────────────────────────────────

// RunHeuristicOptimisation implements Best Fit Decreasing:
//   - Family sort: size DESC → stayDuration DESC → arrivalDate ASC
//   - Room sort: capacity ASC → price ASC
//   - Scoring: quadratic waste penalty (see computeScore)
//   - All ordering is deterministic via sort.SliceStable
func RunHeuristicOptimisation(ctx *OptimisationContext) SimResult {
	inv := deepCopyInventory(ctx.OptInventory)

	fams := make([]FamilyGroup, len(ctx.UnallocatedFams))
	copy(fams, ctx.UnallocatedFams)

	sort.SliceStable(fams, func(i, j int) bool {
		fi, fj := fams[i], fams[j]
		if fi.Size != fj.Size {
			return fi.Size > fj.Size // largest families first
		}
		if fi.StayDuration != fj.StayDuration {
			return fi.StayDuration > fj.StayDuration // longer stays first
		}
		aiZero := fi.ArrivalDate.IsZero()
		ajZero := fj.ArrivalDate.IsZero()
		if aiZero && ajZero {
			return fi.FamilyID < fj.FamilyID // stable tie-break
		}
		if aiZero {
			return false // push missing-arrival to end
		}
		if ajZero {
			return true
		}
		return fi.ArrivalDate.Before(fj.ArrivalDate) // earliest arrival first
	})

	// Rooms: smallest capacity first, cheapest price first within same capacity
	sort.SliceStable(inv, func(i, j int) bool {
		if inv[i].MaxCapacity != inv[j].MaxCapacity {
			return inv[i].MaxCapacity < inv[j].MaxCapacity
		}
		return inv[i].PricePerRoom < inv[j].PricePerRoom
	})

	log.Printf("🔄 Deterministic ordering enforced: %d families  %d room types", len(fams), len(inv))

	result := SimResult{}

	for _, fam := range fams {
		bestScore := math.MaxFloat64
		bestIdx := -1

		for ri := range inv {
			if inv[ri].Available <= 0 || inv[ri].MaxCapacity < fam.Size {
				continue
			}
			waste := inv[ri].MaxCapacity - fam.Size
			sc := computeScore(waste, inv[ri].PricePerRoom)
			if sc < bestScore {
				bestScore = sc
				bestIdx = ri
			}
		}

		if bestIdx >= 0 {
			waste := inv[bestIdx].MaxCapacity - fam.Size
			result.TotalWaste += waste
			result.TotalCost += inv[bestIdx].PricePerRoom
			result.RoomsUsed++
			inv[bestIdx].Available--

			result.Suggestions = append(result.Suggestions, SuggestionItem{
				FamilyID:        fam.FamilyID,
				RoomOfferID:     inv[bestIdx].RoomOfferID,
				RoomName:        inv[bestIdx].RoomName,
				FamilySize:      fam.Size,
				Capacity:        inv[bestIdx].MaxCapacity,
				CapacityWaste:   waste,
				AllocationScore: math.Round(bestScore*100) / 100,
				RoomPrice:       inv[bestIdx].PricePerRoom,
				ConfidenceScore: computeConfidence(waste, inv[bestIdx].MaxCapacity),
			})
		} else {
			result.FamiliesSkipped++
			result.UnplaceableFamilies = append(result.UnplaceableFamilies, fam.FamilyID)
		}
	}

	log.Printf("✅ Optimised simulation: waste=%d  cost=%.2f  roomsUsed=%d  unplaceable=%d",
		result.TotalWaste, result.TotalCost, result.RoomsUsed, result.FamiliesSkipped)

	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// PHASE 1 — CONFIDENCE SCORE
// ─────────────────────────────────────────────────────────────────────────────

// computeConfidence returns a packing tightness score in [0, 1].
//
//	confidence = 1 - (capacityWaste / roomCapacity)
//	Clamped to [0, 1] to handle rounding edge cases.
func computeConfidence(capacityWaste, roomCapacity int) float64 {
	if roomCapacity <= 0 {
		return 0
	}
	c := 1.0 - float64(capacityWaste)/float64(roomCapacity)
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return math.Round(c*1000) / 1000 // 3 decimal places
}



// ─────────────────────────────────────────────────────────────────────────────
// SCORING HELPER
// ─────────────────────────────────────────────────────────────────────────────

// computeScore returns a lower-is-better assignment quality score.
//
//	waste           = roomCapacity - familySize
//	penalty         = waste²  (quadratic; discourages large mismatches)
//	score = (waste × 0.6) + (price × 0.3) + (penalty × 0.1)
func computeScore(capacityWaste int, roomPrice float64) float64 {
	waste := float64(capacityWaste)
	penalty := 0.0
	if capacityWaste > 0 {
		penalty = waste * waste
	}
	return (waste * 0.6) + (roomPrice * 0.3) + (penalty * 0.1)
}

// ─────────────────────────────────────────────────────────────────────────────
// METRICS COMPUTATION
// ─────────────────────────────────────────────────────────────────────────────

// computeMetrics derives all quality metrics from baseline and optimised results.
func computeMetrics(baseline, optimised SimResult, totalGuests int) AIMetrics {
	var improvementPct float64
	if baseline.TotalWaste > 0 {
		improvementPct = float64(baseline.TotalWaste-optimised.TotalWaste) / float64(baseline.TotalWaste) * 100.0
	}
	improvementPct = math.Round(improvementPct*100) / 100
	if improvementPct < 0 {
		improvementPct = 0
	}
	if improvementPct > 100 {
		improvementPct = 100
	}

	baselineCapUsed := baseline.TotalWaste + totalGuests
	optimisedCapUsed := optimised.TotalWaste + totalGuests

	var utilisationImprovementPct float64
	if baselineCapUsed > 0 && optimisedCapUsed > 0 {
		baselineUtil := float64(totalGuests) / float64(baselineCapUsed)
		optimisedUtil := float64(totalGuests) / float64(optimisedCapUsed)
		utilisationImprovementPct = (optimisedUtil - baselineUtil) / baselineUtil * 100.0
		utilisationImprovementPct = math.Round(utilisationImprovementPct*100) / 100
	}

	return AIMetrics{
		CapacityWasteBefore:           baseline.TotalWaste,
		CapacityWasteAfter:            optimised.TotalWaste,
		ImprovementPercent:            improvementPct,
		TotalCostOptimised:            math.Round(optimised.TotalCost*100) / 100,
		RoomsUsedBefore:               baseline.RoomsUsed,
		RoomsUsedAfter:                optimised.RoomsUsed,
		UtilisationImprovementPercent: utilisationImprovementPct,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PHASE 3 — PLAN VALIDITY
// ─────────────────────────────────────────────────────────────────────────────

// buildPlanValidity stamps the plan with a 5-minute freshness window.
// After this window, inventory drift (concurrent allocations, guest changes,
// negotiation updates) may invalidate the suggestions.
func buildPlanValidity(now time.Time) PlanValidity {
	expiresAt := now.Add(planValidityDuration)
	return PlanValidity{
		GeneratedAt:      now,
		ValidityExpiresAt: expiresAt,
		ValidForSeconds:  int(planValidityDuration.Seconds()),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PHASE 4 — IMPROVED REASONING SUMMARY
// ─────────────────────────────────────────────────────────────────────────────

// generateReasoning produces a comprehensive explanation of the optimisation run.
// FIX 2: Near-zero improvement returns a corrected message instead of misleading text.
func generateReasoning(baseline, optimised SimResult, metrics AIMetrics) string {
	const header = "This allocation uses a Best-Fit-Decreasing heuristic optimised for capacity " +
		"utilisation (weight: 60%) and pricing efficiency (weight: 30%), with a quadratic penalty " +
		"for capacity mismatches (weight: 10%). " +
		"Results are deterministic and valid for a short execution window."

	switch {
	case metrics.ImprovementPercent >= optimisationEffectiveThreshold:
		// FIX 2: Only show improvement text when improvement is meaningful.
		return fmt.Sprintf(
			"AI heuristic (BFD) reduced capacity waste by %.1f%% (%d → %d spare seats). "+
				"Rooms used: %d → %d. Families sorted by size (DESC) and stay duration (DESC) "+
				"to ensure largest groups are placed first. %s",
			metrics.ImprovementPercent,
			baseline.TotalWaste, optimised.TotalWaste,
			baseline.RoomsUsed, optimised.RoomsUsed,
			header,
		)
	case baseline.TotalWaste == 0:
		return "Allocation is already optimal — zero unused capacity detected. " + header
	default:
		// FIX 2: Near-zero improvement — honest, non-misleading message.
		return "No significant optimisation opportunity detected. " +
			"Current room inventory constraints prevent tighter capacity packing."
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PHASE 5 — API HANDLER
// ─────────────────────────────────────────────────────────────────────────────

// AIAllocateHandler is the read-only POST /api/v1/events/:id/ai-allocate handler.
// Validates ownership, runs the full AI pipeline, and returns suggestions.
// ZERO database writes are performed.
func AIAllocateHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		runStart := time.Now()

		// ── Auth ──────────────────────────────────────────────────────────────
		userIDRaw := c.Locals("userID")
		if userIDRaw == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		userUUID, ok := userIDRaw.(uuid.UUID)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid user ID type"})
		}

		userRoleRaw := c.Locals("role")
		if userRoleRaw == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		role, ok := userRoleRaw.(string)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid user role type"})
		}
		if role != "agent" && role != "head_guest" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized"})
		}

		// ── Parse event ID ────────────────────────────────────────────────────
		eventIDStr := c.Params("id")
		eventUUID, err := uuid.Parse(eventIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event ID"})
		}

		// ── Fetch event for ownership + status check ──────────────────────────
		var event models.Event
		if err := db.First(&event, "id = ?", eventUUID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
		}

		if role == "agent" && userUUID != event.AgentID {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized for this event"})
		}
		if role == "head_guest" && userUUID != event.HeadGuestID {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized for this event"})
		}
		if event.Status == "finalized" || event.Status == "locked" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Event is finalized or locked; AI allocation suggestions unavailable",
			})
		}

		// ── Load context ──────────────────────────────────────────────────────
		ctx, err := GetOptimisationContext(db, eventUUID)
		if err != nil {
			log.Printf("❌ AI allocate context error (event %s): %v", eventIDStr, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		if len(ctx.UnallocatedFams) == 0 {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"message": "No unallocated families found; nothing to optimise",
			})
		}

		// ── Validate context ──────────────────────────────────────────────────
		if validationErrors := ValidateOptimisationContext(ctx); len(validationErrors) > 0 {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error":   "Optimisation context validation failed",
				"details": validationErrors,
			})
		}

		totalGuests := 0
		for _, f := range ctx.UnallocatedFams {
			totalGuests += f.Size
		}

		// ── Baseline + Optimisation ───────────────────────────────────────────
		baseline := SimulateBaselineAllocation(ctx)
		optimised := RunHeuristicOptimisation(ctx)

		// ── Metrics + Reasoning + Validity ────────────────────────────────────
		metrics := computeMetrics(baseline, optimised, totalGuests)
		reasoning := generateReasoning(baseline, optimised, metrics)
		planValidity := buildPlanValidity(runStart)

		// FIX 1: Determine if optimisation is meaningfully better than baseline.
		optimisationEffective := metrics.ImprovementPercent >= optimisationEffectiveThreshold
		optimisationMessage := ""
		if !optimisationEffective {
			optimisationMessage = "Current allocation is already near-optimal given available room capacities."
		}

		// FIX 7: Structured deterministic debug log with optimisationEffective flag.
		roomsEvaluated := len(ctx.OptInventory)
		log.Printf("📈 AI optimisation run | eventId: %s | familiesProcessed: %d | roomsEvaluated: %d | "+
			"baselineWaste: %d | optimisedWaste: %d | improvementPercent: %.2f%% | "+
			"optimisationEffective: %v | deterministicOrdering: true",
			eventIDStr, len(ctx.UnallocatedFams), roomsEvaluated,
			baseline.TotalWaste, optimised.TotalWaste, metrics.ImprovementPercent,
			optimisationEffective)
		log.Printf("   Families skipped baseline: %d | Unplaceable optimised: %d | Plan valid until: %s",
			baseline.FamiliesSkipped, optimised.FamiliesSkipped,
			planValidity.ValidityExpiresAt.Format(time.RFC3339))

		// ── Nil-safe serialisation ────────────────────────────────────────────
		if optimised.Suggestions == nil {
			optimised.Suggestions = []SuggestionItem{}
		}
		if optimised.UnplaceableFamilies == nil {
			optimised.UnplaceableFamilies = []string{}
		}

		return c.Status(fiber.StatusOK).JSON(AIAllocateResponse{
			Suggestions:           optimised.Suggestions,
			UnplaceableFamilies:   optimised.UnplaceableFamilies,
			Metrics:               metrics,
			Reasoning:             reasoning,
			PlanValidity:          planValidity,
			OptimisationEffective: optimisationEffective,   // FIX 1
			OptimisationMessage:   optimisationMessage,     // FIX 1
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UTILITIES
// ─────────────────────────────────────────────────────────────────────────────

// deepCopyInventory returns an independent clone so baseline and optimised
// simulations operate on separate virtual inventories without interference.
func deepCopyInventory(src []VirtualRoom) []VirtualRoom {
	dst := make([]VirtualRoom, len(src))
	copy(dst, src)
	return dst
}
