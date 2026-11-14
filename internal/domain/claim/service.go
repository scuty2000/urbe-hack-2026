package claim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// profileMetadataKeys are private-profile bookkeeping values that are not
// discriminative attributes and must never become verification questions.
var profileMetadataKeys = map[string]bool{
	"catalog_source": true,
	"embedding":      true,
	"embeddings":     true,
	"photos":         true,
}

type service struct {
	db   repository.Database
	deps Deps
}

func (s *service) now() time.Time {
	if s.deps.Now != nil {
		return s.deps.Now().UTC()
	}
	return time.Now().UTC()
}

// CreateClaim scores the answers, applies the policy and persists the claim,
// its verification session, the claim event and the audit entry atomically.
func (s *service) CreateClaim(ctx context.Context, input CreateClaimInput) (Claim, error) {
	itemID := strings.TrimSpace(input.ItemID)
	searcherID := strings.TrimSpace(input.SearcherID)
	if itemID == "" || searcherID == "" {
		return Claim{}, fmt.Errorf("create claim: item id and searcher id are required: %w", errs.ErrInvalidInput)
	}

	row, err := s.db.GetItemByID(ctx, itemID)
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}
	item, err := itemdomain.FromRecord(row)
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}

	channel, ok := returnChannelFor(item.Status)
	if !ok {
		return Claim{}, fmt.Errorf("create claim %q: item is %q: %w", itemID, item.Status, errs.ErrConflict)
	}
	requestedChannel := strings.TrimSpace(input.ReturnChannel)
	if requestedChannel != "" && requestedChannel != channel {
		return Claim{}, fmt.Errorf(
			"create claim %q: return channel %q conflicts with item status %q: %w",
			itemID, requestedChannel, item.Status, errs.ErrInvalidInput,
		)
	}

	evidence := mergeStrings(input.Evidence, input.Session.Evidence)
	flags := mergeStrings(input.Flags, input.Session.Flags)
	questions, expected, privateKeys := questionSet(item, input.Session.Questions)

	// Server-derived flags cannot be removed by the caller: identity and prompt
	// injection checks run on the raw answers before any filtering.
	identityMatch := identityMatches(item.PrivateProfile, input.Session.Answers)
	if hasIdentityMismatch(identityMatch) {
		flags = mergeStrings(flags, []string{verification.FlagIdentityMismatch})
	}
	if containsInjection(input.Session.Answers, evidence) {
		flags = mergeStrings(flags, []string{verification.FlagPromptInjection})
	}

	answers := filterAnswers(input.Session.Answers, questions)
	state := verification.State{
		Item:          expected,
		Searcher:      stringAnswers(answers),
		IdentityMatch: identityMatch,
		Evidence:      evidence,
		RiskTier:      riskTierOf(item),
	}

	result, err := s.deps.Engine.Score(ctx, state, questions)
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: scoring: %w", itemID, err)
	}
	confidence := clampConfidence(result.Confidence)
	policy := verification.Decision(state.RiskTier, confidence, evidence, flags)
	// Public listing attributes alone are not ownership evidence: an automatic
	// approval must include at least one matching private attribute.
	if policy.Decision == verification.DecisionAutoApprove && !hasPrivateMatch(result.Answers, privateKeys) {
		policy.Decision = verification.DecisionMoreInfo
		policy.Reasons = append(policy.Reasons, "no private attribute matched; more information required")
	}
	status := statusForDecision(policy.Decision)

	now := s.now()
	claimID, err := newID()
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}
	sessionID, err := newID()
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}
	eventID, err := newID()
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}
	auditID, err := newID()
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}

	claimRow := record.ClaimRecord{
		ID:            claimID,
		ItemID:        itemID,
		SearcherID:    searcherID,
		Status:        status,
		ReturnChannel: channel,
		Confidence:    confidence,
		CreatedAt:     now,
	}
	if status == StatusApproved {
		// The policy is the approver for low/medium risk claims; the custodian
		// still receives the approved claim and can review it if needed.
		claimRow.ApprovalBy = "policy"
		claimRow.ApprovedAt = &now
	}
	if claimRow.Evidence, err = encodeJSON(evidence, "[]"); err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}

	sessionRow := record.VerificationSessionRecord{
		ID:           sessionID,
		ClaimID:      claimID,
		Rationale:    result.Rationale,
		ModelVersion: result.ModelVersion,
		CreatedAt:    now,
	}
	if sessionRow.Questions, err = encodeJSON(questions, "{}"); err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}
	if sessionRow.Answers, err = encodeJSON(nonNilAnswers(answers), "{}"); err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}
	if sessionRow.Probabilities, err = encodeJSON(nonNilAnswers(result.Answers), "{}"); err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}
	if sessionRow.Flags, err = encodeJSON(flags, "[]"); err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}

	metadata, err := encodeJSON(map[string]any{
		"claimId":    claimID,
		"itemId":     itemID,
		"status":     status,
		"decision":   policy.Decision,
		"riskTier":   state.RiskTier,
		"confidence": confidence,
	}, "{}")
	if err != nil {
		return Claim{}, fmt.Errorf("create claim %q: %w", itemID, err)
	}

	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		// Re-read the item inside the transaction: the return channel is a
		// function of the current state, never of the client payload.
		current, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("create claim %q: %w", itemID, err)
		}
		currentChannel, ok := returnChannelFor(current.Status)
		if !ok {
			return fmt.Errorf("create claim %q: item is %q: %w", itemID, current.Status, errs.ErrConflict)
		}
		if requestedChannel != "" && requestedChannel != currentChannel {
			return fmt.Errorf(
				"create claim %q: return channel %q conflicts with item status %q: %w",
				itemID, requestedChannel, current.Status, errs.ErrInvalidInput,
			)
		}
		claimRow.ReturnChannel = currentChannel

		if err := tx.CreateClaim(ctx, claimRow); err != nil {
			return fmt.Errorf("create claim %q: %w", itemID, err)
		}
		if err := tx.CreateVerificationSession(ctx, sessionRow); err != nil {
			return fmt.Errorf("create claim %q: %w", itemID, err)
		}
		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventClaimCreated,
			ActorID:   searcherID,
			Timestamp: now,
			Notes:     "claim " + claimID + " created; policy " + policy.Decision,
		}); err != nil {
			return fmt.Errorf("create claim %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   searcherID,
			Action:    "claim.created",
			Entity:    "claim",
			EntityID:  claimID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("create claim %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return Claim{}, err
	}

	// Notifications are best-effort and run after the claim is committed: a
	// notification failure must never fail a successful claim.
	s.notifyClaimCreated(ctx, item, claimRow)

	return claimFromRecord(claimRow), nil
}

// notifyClaimCreated tells the item's custodian/legal finder that a claim
// matched, and the searcher that the claim was recorded. Recipients without an
// account are skipped by the platform notifier.
func (s *service) notifyClaimCreated(ctx context.Context, item itemdomain.Item, row record.ClaimRecord) {
	if s.deps.Notifier == nil {
		return
	}
	payload := map[string]any{
		"itemId":  item.ID,
		"claimId": row.ID,
		"status":  row.Status,
	}
	seen := map[string]bool{}
	for _, recipient := range []string{item.CustodianID, item.LegalFinderID} {
		if recipient == "" || seen[recipient] {
			continue
		}
		seen[recipient] = true
		platform.TryNotifyAccount(ctx, s.deps.Notifier, "claim.created", recipient, platform.NotificationMatch, payload)
	}

	if row.SearcherID == "" || seen[row.SearcherID] {
		return
	}
	searcherPayload := map[string]any{
		"itemId":  item.ID,
		"claimId": row.ID,
		"status":  row.Status,
		"role":    ActorRoleSearcher,
	}
	platform.TryNotifyAccount(ctx, s.deps.Notifier, "claim.created", row.SearcherID, platform.NotificationMatch, searcherPayload)
}

// GetClaim returns the role-safe claim. Only the custodian and the institution
// receive the verification report; the searcher receives status, confidence
// band and return channel.
func (s *service) GetClaim(ctx context.Context, claimID string, actor Actor) (Claim, error) {
	claimID = strings.TrimSpace(claimID)
	if claimID == "" {
		return Claim{}, fmt.Errorf("get claim: claim id is required: %w", errs.ErrInvalidInput)
	}

	row, err := s.db.GetClaimByID(ctx, claimID)
	if err != nil {
		return Claim{}, fmt.Errorf("get claim %q: %w", claimID, err)
	}
	itemRow, err := s.db.GetItemByID(ctx, row.ItemID)
	if err != nil {
		return Claim{}, fmt.Errorf("get claim %q: %w", claimID, err)
	}
	if err := authorizeRead(actor, row, itemRow); err != nil {
		return Claim{}, fmt.Errorf("get claim %q: %w", claimID, err)
	}

	result := claimFromRecord(row)
	if actor.Role == ActorRoleSearcher {
		// Defence in depth: the service never hands the raw confidence to an
		// owner, even if a future caller forgets the role-safe DTO.
		result.Confidence = 0
		return result, nil
	}

	session, err := s.db.GetVerificationSessionByClaimID(ctx, claimID)
	if errors.Is(err, errs.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return Claim{}, fmt.Errorf("get claim %q: %w", claimID, err)
	}
	report, err := buildReport(row, session, riskTierOf(itemdomain.Item{
		RiskTier:  itemRow.RiskTier,
		Sensitive: itemRow.Sensitive,
	}))
	if err != nil {
		return Claim{}, fmt.Errorf("get claim %q: %w", claimID, err)
	}
	result.Report = &report
	return result, nil
}

// ReviewClaim records the custodian or institution decision. An institution is
// recognised by its user role; the custodian and the legal finder by the item.
func (s *service) ReviewClaim(ctx context.Context, claimID, decision, actorID string) (Claim, error) {
	claimID = strings.TrimSpace(claimID)
	decision = strings.TrimSpace(decision)
	actorID = strings.TrimSpace(actorID)
	if claimID == "" || actorID == "" {
		return Claim{}, fmt.Errorf("review claim: claim id and actor id are required: %w", errs.ErrInvalidInput)
	}
	if decision != DecisionApproved && decision != DecisionRejected {
		return Claim{}, fmt.Errorf("review claim: decision must be approved or rejected: %w", errs.ErrInvalidInput)
	}

	now := s.now()
	eventID, err := newID()
	if err != nil {
		return Claim{}, fmt.Errorf("review claim %q: %w", claimID, err)
	}
	auditID, err := newID()
	if err != nil {
		return Claim{}, fmt.Errorf("review claim %q: %w", claimID, err)
	}
	metadata, err := encodeJSON(map[string]any{
		"claimId":  claimID,
		"decision": decision,
	}, "{}")
	if err != nil {
		return Claim{}, fmt.Errorf("review claim %q: %w", claimID, err)
	}

	var reviewed Claim
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetClaimByID(ctx, claimID)
		if err != nil {
			return fmt.Errorf("review claim %q: %w", claimID, err)
		}
		switch row.Status {
		case StatusSubmitted, StatusUnderReview, StatusMoreInfo:
		default:
			return fmt.Errorf("review claim %q: claim is %q: %w", claimID, row.Status, errs.ErrConflict)
		}

		itemRow, err := tx.GetItemByID(ctx, row.ItemID)
		if err != nil {
			return fmt.Errorf("review claim %q: %w", claimID, err)
		}
		if err := authorizeReview(ctx, tx, itemRow, actorID); err != nil {
			return fmt.Errorf("review claim %q: %w", claimID, err)
		}

		fields := map[string]any{
			"status":      decision,
			"approval_by": actorID,
			"approved_at": nil,
		}
		if decision == DecisionApproved {
			fields["approved_at"] = now
		}
		if err := tx.UpdateClaimFields(ctx, claimID, fields); err != nil {
			return fmt.Errorf("review claim %q: %w", claimID, err)
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    row.ItemID,
			Type:      decision,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     "claim " + claimID + " " + decision,
		}); err != nil {
			return fmt.Errorf("review claim %q: %w", claimID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    "claim." + decision,
			Entity:    "claim",
			EntityID:  claimID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("review claim %q: %w", claimID, err)
		}

		row.Status = decision
		row.ApprovalBy = actorID
		row.ApprovedAt = nil
		if decision == DecisionApproved {
			row.ApprovedAt = &now
		}
		reviewed = claimFromRecord(row)
		return nil
	})
	if err != nil {
		return Claim{}, err
	}
	return reviewed, nil
}

// authorizeRead enforces the claim read rules.
func authorizeRead(actor Actor, row record.ClaimRecord, item record.ItemRecord) error {
	switch actor.Role {
	case ActorRoleSearcher:
		if actor.ID != "" && actor.ID == row.SearcherID {
			return nil
		}
	case ActorRoleIntermediary:
		if actor.ID != "" && (actor.ID == item.CustodianID || actor.ID == item.LegalFinderID) {
			return nil
		}
	case ActorRoleInstitution, ActorRoleAdmin:
		return nil
	}
	return errs.ErrForbidden
}

// authorizeReview accepts the current custodian, the legal finder and any
// institution user (looked up in the users table).
func authorizeReview(ctx context.Context, tx repository.Repository, item record.ItemRecord, actorID string) error {
	if actorID != "" && (actorID == item.CustodianID || actorID == item.LegalFinderID) {
		return nil
	}
	user, err := tx.GetUserByID(ctx, actorID)
	if err == nil && (user.Role == ActorRoleInstitution || user.Role == ActorRoleAdmin) {
		return nil
	}
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return err
	}
	return errs.ErrForbidden
}

// buildReport decodes the stored session and recomputes the deterministic
// decision so the reviewers see why the claim reached its status.
func buildReport(row record.ClaimRecord, session record.VerificationSessionRecord, riskTier string) (Report, error) {
	questions := map[string]itemdomain.Question{}
	if session.Questions != "" {
		if err := json.Unmarshal([]byte(session.Questions), &questions); err != nil {
			return Report{}, fmt.Errorf("decoding verification questions: %w", err)
		}
	}
	answers := map[string]string{}
	if session.Answers != "" {
		if err := json.Unmarshal([]byte(session.Answers), &answers); err != nil {
			return Report{}, fmt.Errorf("decoding verification answers: %w", err)
		}
	}
	probabilities := map[string]itemdomain.Answer{}
	if session.Probabilities != "" {
		if err := json.Unmarshal([]byte(session.Probabilities), &probabilities); err != nil {
			return Report{}, fmt.Errorf("decoding verification probabilities: %w", err)
		}
	}
	flags := []string{}
	if session.Flags != "" {
		if err := json.Unmarshal([]byte(session.Flags), &flags); err != nil {
			return Report{}, fmt.Errorf("decoding verification flags: %w", err)
		}
	}
	evidence := []string{}
	if row.Evidence != "" {
		if err := json.Unmarshal([]byte(row.Evidence), &evidence); err != nil {
			return Report{}, fmt.Errorf("decoding claim evidence: %w", err)
		}
	}

	policy := verification.Decision(riskTier, row.Confidence, evidence, flags)
	return Report{
		Questions:     questions,
		Answers:       answers,
		Probabilities: probabilities,
		Confidence:    row.Confidence,
		Rationale:     session.Rationale,
		ModelVersion:  session.ModelVersion,
		Evidence:      evidence,
		Flags:         flags,
		Decision:      policy.Decision,
		Reasons:       policy.Reasons,
	}, nil
}

// expectedAttributes merges the public attributes with the discriminative
// private-profile values. It also returns the set of private keys so the
// caller can require private evidence for an automatic approval.
//
// Identity fields are excluded entirely: they are compared separately and
// never become questions or D3 attributes. Unsupported values (nested objects,
// arrays of objects) are excluded too, so they cannot depress honest scores.
func expectedAttributes(item itemdomain.Item) (map[string]any, map[string]bool) {
	attributes := map[string]any{}
	private := map[string]bool{}
	public := map[string]string{
		"category":  item.Category,
		"brand":     item.Brand,
		"model":     item.Model,
		"color":     item.Color,
		"condition": item.Condition,
	}
	publicKeys := map[string]bool{
		"category": true, "brand": true, "model": true, "color": true, "condition": true,
	}
	for key, value := range public {
		if strings.TrimSpace(value) != "" {
			attributes[key] = value
		}
	}
	for key, value := range item.PrivateProfile {
		key = strings.TrimSpace(key)
		if key == "" || profileMetadataKeys[strings.ToLower(key)] || verification.IsIdentityField(key) {
			continue
		}
		if !supportedAttributeValue(value) {
			continue
		}
		attributes[key] = value
		// A profile key that merely repeats a public listing attribute is not
		// private evidence.
		if !publicKeys[strings.ToLower(key)] {
			private[key] = true
		}
	}
	return attributes, private
}

// supportedAttributeValue reports whether a private value can be asked as a
// verification question. Scalars are compared directly; lists of scalars are
// membership questions; objects and nested structures are excluded.
func supportedAttributeValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case bool:
		return true
	case float64, float32, int, int64, int32:
		return true
	case []string:
		return len(v) > 0
	case []any:
		if len(v) == 0 {
			return false
		}
		for _, item := range v {
			if !isScalarValue(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isScalarValue reports whether a list element is directly comparable.
func isScalarValue(value any) bool {
	switch value.(type) {
	case string, bool, float64, float32, int, int64, int32:
		return true
	default:
		return false
	}
}

// questionSet uses the agent-supplied questions when present and otherwise
// derives one question per supported attribute. Booleans stay yes/no (noul),
// lists become choice questions, and scalar strings/numbers stay noul.
func questionSet(
	item itemdomain.Item,
	supplied verification.Questions,
) (verification.Questions, map[string]any, map[string]bool) {
	attributes, private := expectedAttributes(item)
	if len(supplied) > 0 {
		return supplied, attributes, private
	}

	questions := make(verification.Questions, len(attributes)+1)
	for key, value := range attributes {
		questions[key] = questionForValue(value)
	}
	if len(questions) == 0 {
		// Nothing discriminative was recorded: ask the searcher for a free-text
		// description so the policy still has a deterministic signal.
		questions["description"] = itemdomain.Question{Type: "score"}
	}
	return questions, attributes, private
}

// questionForValue maps a supported private value to its d3 question type.
func questionForValue(value any) itemdomain.Question {
	switch value.(type) {
	case []any, []string:
		return itemdomain.Question{Type: "choice", Criteria: choiceCriteria(value)}
	default:
		// noul covers both yes/no booleans and free-text attributes; the local
		// scorer normalizes boolean synonyms and the D3 client receives the
		// same question type used by the enrollment catalog.
		return itemdomain.Question{Type: "noul"}
	}
}

// choiceCriteria exposes list members as d3 choice options.
func choiceCriteria(value any) map[string]string {
	options := map[string]string{}
	appendOption := func(item any) {
		label := strings.TrimSpace(fmt.Sprint(item))
		if label != "" {
			options[label] = label
		}
	}
	switch values := value.(type) {
	case []any:
		for _, item := range values {
			appendOption(item)
		}
	case []string:
		for _, item := range values {
			appendOption(item)
		}
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

// filterAnswers drops every answer that is not part of the server-derived
// question set. Untrusted caller keys never reach D3 or the stored session.
func filterAnswers(answers map[string]string, questions verification.Questions) map[string]string {
	filtered := make(map[string]string, len(questions))
	for key, value := range answers {
		canonical := canonicalQuestionKey(questions, key)
		if canonical == "" {
			continue
		}
		filtered[canonical] = value
	}
	return filtered
}

// canonicalQuestionKey resolves an answer key to the exact question key.
func canonicalQuestionKey(questions verification.Questions, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if _, ok := questions[key]; ok {
		return key
	}
	for questionKey := range questions {
		if strings.EqualFold(questionKey, key) {
			return questionKey
		}
	}
	return ""
}

// identityMatches compares only the identity fields the searcher actually
// answered. A missing identity answer is not a mismatch; a present but
// different value is.
func identityMatches(profile map[string]any, answers map[string]string) map[string]bool {
	if len(profile) == 0 || len(answers) == 0 {
		return nil
	}
	matches := map[string]bool{}
	for key, expected := range profile {
		if !verification.IsIdentityField(key) || !hasContent(expected) {
			continue
		}
		for answerKey, answerValue := range answers {
			if strings.EqualFold(strings.TrimSpace(answerKey), strings.TrimSpace(key)) {
				matches[key] = strings.EqualFold(strings.TrimSpace(fmt.Sprint(expected)), strings.TrimSpace(answerValue))
				break
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

// hasIdentityMismatch reports whether any comparable identity field differed.
func hasIdentityMismatch(matches map[string]bool) bool {
	for _, match := range matches {
		if !match {
			return true
		}
	}
	return false
}

// hasPrivateMatch reports whether at least one private attribute was answered
// with ownership-grade confidence.
func hasPrivateMatch(answers map[string]itemdomain.Answer, privateKeys map[string]bool) bool {
	for key := range privateKeys {
		answer, ok := answers[key]
		if ok && verification.AnswerProbability(answer) >= 0.75 {
			return true
		}
	}
	return false
}

// injectionPatterns are server-side prompt-injection signatures. They are
// deliberately conservative: legitimate item descriptions do not contain
// instruction overrides, role changes or approval commands.
var injectionPatterns = []string{
	"ignore previous",
	"ignore all previous",
	"ignore the previous",
	"disregard previous",
	"forget your instructions",
	"system prompt",
	"you are now",
	"act as a",
	"act as an",
	"jailbreak",
	"developer mode",
	"prompt injection",
	"override the policy",
	"approve this claim",
	"approve the claim",
	"```",
	"<script",
	"assistant:",
	"system:",
}

// containsInjection scans answers and evidence for obvious injection attempts.
func containsInjection(answers map[string]string, evidence []string) bool {
	for _, value := range answers {
		if looksLikeInjection(value) {
			return true
		}
	}
	for _, value := range evidence {
		if looksLikeInjection(value) {
			return true
		}
	}
	return false
}

func looksLikeInjection(value string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(value), " "))
	for _, pattern := range injectionPatterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}

// riskTierOf normalises the item tier. A missing tier defaults to medium; a
// sensitive flag always wins.
func riskTierOf(item itemdomain.Item) string {
	if item.Sensitive {
		return itemdomain.RiskSensitive
	}
	switch strings.ToLower(strings.TrimSpace(item.RiskTier)) {
	case itemdomain.RiskLow, itemdomain.RiskMedium, itemdomain.RiskHigh, itemdomain.RiskSensitive:
		return strings.ToLower(strings.TrimSpace(item.RiskTier))
	default:
		return itemdomain.RiskMedium
	}
}

// stringAnswers converts free-text answers to the any-map used by the engine.
func stringAnswers(answers map[string]string) map[string]any {
	values := make(map[string]any, len(answers))
	for key, value := range answers {
		values[key] = value
	}
	return values
}

// mergeStrings concatenates two slices without duplicates.
func mergeStrings(left, right []string) []string {
	merged := make([]string, 0, len(left)+len(right))
	seen := map[string]bool{}
	for _, values := range [][]string{left, right} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			merged = append(merged, value)
		}
	}
	return merged
}

// hasContent reports whether a profile value is present.
func hasContent(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return true
	}
}

// nonNilAnswers guarantees an empty JSON object rather than null.
func nonNilAnswers[T any](values map[string]T) map[string]T {
	if values == nil {
		return map[string]T{}
	}
	return values
}

// encodeJSON serialises a value, defaulting empty slices/maps to valid JSON.
func encodeJSON(value any, fallback string) (string, error) {
	switch v := value.(type) {
	case []string:
		if v == nil {
			return fallback, nil
		}
	case map[string]any:
		if v == nil {
			return fallback, nil
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encoding json: %w", err)
	}
	if string(encoded) == "null" {
		return fallback, nil
	}
	return string(encoded), nil
}

// clampConfidence bounds a probability to [0, 1].
func clampConfidence(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func newID() (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
