package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

// mockChat is the deterministic fallback. It never calls an external model and
// never echoes a private profile: it searches with the user's own words, asks
// one open question and, once the searcher supplied a recognisable detail,
// creates a claim through the claim service.
func (s *service) mockChat(ctx context.Context, searcherID string, messages []Message) (Reply, error) {
	text := userText(messages)
	users := messagesFromUser(messages)
	answers := mockAnswers(text)
	zone := ""
	if len(users) > 0 {
		zone = mockZone(users[0].Content)
	}
	results, err := s.deps.Search.Search(ctx, search.Query{
		Text:     text,
		Category: answers["category"],
		Zone:     zone,
		Limit:    searchLimit,
	})
	if err != nil {
		return Reply{}, fmt.Errorf("agent mock chat: searching: %w", err)
	}
	if results == nil {
		results = []search.SearchResult{}
	}

	if len(results) == 0 {
		return Reply{
			Message: "Non ho trovato oggetti compatibili. Prova a indicare il tipo di oggetto e la zona in cui lo hai perso.",
		}, nil
	}

	if len(messagesFromUser(messages)) < 2 || len(answers) == 0 {
		return Reply{
			Message:    mockQuestion(results),
			Candidates: results,
		}, nil
	}

	created, err := s.deps.Claim.CreateClaim(ctx, claim.CreateClaimInput{
		ItemID:     results[0].ItemID,
		SearcherID: searcherID,
		Session:    claim.Session{Answers: answers},
	})
	if err != nil {
		if errors.Is(err, errs.ErrConflict) {
			return Reply{
				Message:    "L'oggetto non è ancora disponibile per una richiesta. Prova più tardi o contatta un operatore.",
				Candidates: results,
			}, nil
		}
		return Reply{
			Message:    "Non riesco ancora a verificare i dettagli. Puoi aggiungere un altro dettaglio che solo il proprietario conosce?",
			Candidates: results,
		}, nil
	}

	return Reply{
		Message:    mockClaimMessage(created),
		Candidates: results,
		ClaimID:    created.ID,
		Done:       true,
	}, nil
}

// mockQuestion asks one open question and only mentions public fields.
func mockQuestion(results []search.SearchResult) string {
	kind := "oggetti"
	if len(results) == 1 {
		kind = "un oggetto"
	}
	return fmt.Sprintf(
		"Ho trovato %s compatibile. Descrivi un dettaglio che solo il proprietario conosce: un segno particolare, il colore di un accessorio o cosa conteneva.",
		kind,
	)
}

// mockClaimMessage maps the claim status to a safe searcher-facing sentence.
func mockClaimMessage(created claim.Claim) string {
	switch created.Status {
	case claim.StatusApproved:
		return "La richiesta è stata approvata. Puoi ritirare l'oggetto presso il custode."
	case claim.StatusUnderReview:
		return "Ho registrato la richiesta: è in verifica da parte del custode."
	case claim.StatusRejected:
		return "La richiesta non è stata approvata."
	default:
		return "Ho registrato la richiesta, ma servono altre informazioni per completare la verifica."
	}
}

// mockAnswers extracts recognisable details from the searcher's own words. It
// is deliberately conservative: it only answers what the searcher actually
// wrote and never guesses an exact model or serial number.
func mockAnswers(text string) map[string]string {
	lower := strings.ToLower(text)
	answers := map[string]string{}

	for _, category := range []struct {
		tokens []string
		value  string
	}{
		{[]string{"iphone", "telefono", "cellulare", "smartphone"}, "smartphone"},
		{[]string{"portafoglio", "wallet"}, "wallet"},
		{[]string{"ombrello", "umbrella"}, "umbrella"},
		{[]string{"documento", "document", "carta di identità", "patente"}, "document"},
		{[]string{"zaino", "backpack"}, "backpack"},
		{[]string{"cuffie", "auricolari", "headphones"}, "headphones"},
	} {
		if containsAny(lower, category.tokens...) {
			answers["category"] = category.value
			break
		}
	}

	for _, color := range []struct {
		tokens []string
		value  string
	}{
		{[]string{"nero", "black", "nera"}, "nero"},
		{[]string{"blu", "blue", "azzurro"}, "blu"},
		{[]string{"bianco", "white", "bianca"}, "bianco"},
		{[]string{"marrone", "brown"}, "marrone"},
		{[]string{"rosso", "red", "rossa"}, "rosso"},
		{[]string{"verde", "green"}, "verde"},
		{[]string{"grigio", "gray", "grey"}, "grigio"},
		{[]string{"rosa", "pink"}, "rosa"},
	} {
		if containsAny(lower, color.tokens...) {
			if strings.Contains(lower, "custodia") || strings.Contains(lower, "cover") {
				answers["case_color"] = color.value
			} else {
				answers["color"] = color.value
			}
			break
		}
	}

	if containsAny(lower, "senza crepe", "non ha crepe", "non è rotto", "no crack") {
		answers["has_crack"] = "no"
	} else if containsAny(lower, "crepa", "crepe", "crepato", "rotto", "crack") {
		answers["has_crack"] = "sì"
	}

	if containsAny(lower, "adesivo", "sticker") {
		if strings.Contains(lower, "gatto") {
			answers["sticker"] = "gatto"
		} else {
			answers["sticker"] = "sì"
		}
	}

	if containsAny(lower, "bancomat") {
		answers["contents"] = "bancomat"
	} else if strings.Contains(lower, "carta") {
		answers["contents"] = "carta di identità"
	}

	return answers
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// mockZone extracts the zone after "zona" so the demo mock can narrow the
// search without a gazetteer. It never returns a coordinate.
func mockZone(text string) string {
	lower := strings.ToLower(text)
	index := strings.Index(lower, "zona")
	if index < 0 {
		return ""
	}
	zone := text[index+len("zona"):]
	zone = strings.TrimLeft(zone, " :,;.-")
	if cut := strings.IndexAny(zone, ".,;!?"); cut >= 0 {
		zone = zone[:cut]
	}
	// A zone is at most a few words: stop before the rest of the sentence.
	words := strings.Fields(zone)
	if len(words) > 3 {
		words = words[:3]
	}
	zone = strings.TrimSpace(strings.Join(words, " "))
	if len(zone) > 40 {
		zone = strings.TrimSpace(zone[:40])
	}
	return zone
}
