package item

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
)

// Catalog input/output types. They are the domain port implemented by
// platform/d3, so the service never depends on the HTTP client directly.

// Question is a d3 question. Type is one of "choice", "noul" or "score"; the
// criteria payload depends on it (map for choice, list for score).
type Question struct {
	Type     string `json:"type"`
	Criteria any    `json:"criteria,omitempty"`
}

// Answer is a d3 answer for one question.
type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Noul          float64            `json:"noul,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
}

// CatalogForm is what the finder or intermediary typed in the form. D3 never
// silently overrides it; on failure these values are returned unchanged.
type CatalogForm struct {
	Category     string
	Color        string
	Condition    string
	DocumentType string
	HasCrack     bool
	HasSticker   bool
	HasCase      bool
	IsDocument   bool
}

// CatalogResult is the confirmed classification. Source is "d3" when the
// decision engine answered and "form" when it was unavailable.
type CatalogResult struct {
	Category     string
	Color        string
	Condition    string
	DocumentType string
	HasCrack     bool
	HasSticker   bool
	HasCase      bool
	IsDocument   bool
	Source       string
}

// Catalog sources.
const (
	CatalogSourceD3   = "d3"
	CatalogSourceForm = "form"
)

// ImageInput is one photo to classify. Data wins over Path; Path is resolved
// through the injected Storage port.
type ImageInput struct {
	Path      string
	Data      []byte
	MediaType string
}

// Cataloger is the D3 port: it receives inline images (data URLs) and a small
// question set, and returns probability answers.
type Cataloger interface {
	Ask(ctx context.Context, state string, images []string, questions map[string]Question) (map[string]Answer, error)
}

// Storage is the upload port. Save writes one photo and returns its relative
// path; Read returns the bytes of a previously stored relative path.
type Storage interface {
	Save(r io.Reader) (string, error)
	Read(path string) ([]byte, error)
}

const catalogState = "Classify the lost item shown in the photos. Only answer the requested questions."

// catalogQuestions is the small, deterministic question set used at enrollment.
func catalogQuestions() map[string]Question {
	return map[string]Question{
		"category": {
			Type: "choice",
			Criteria: map[string]string{
				"smartphone": "a smartphone or mobile phone",
				"laptop":     "a laptop or portable computer",
				"wallet":     "a wallet or purse",
				"keys":       "a bunch of keys",
				"document":   "an identity or payment document",
				"jewelry":    "jewelry",
				"watch":      "a watch",
				"glasses":    "eyeglasses or sunglasses",
				"backpack":   "a backpack or bag",
				"headphones": "headphones or earbuds",
				"book":       "a book or notebook",
				"bottle":     "a bottle or thermos",
				"clothing":   "an item of clothing",
				"umbrella":   "an umbrella",
				"other":      "none of the above",
			},
		},
		"color": {
			Type: "choice",
			Criteria: map[string]string{
				"black": "black", "white": "white", "gray": "gray or silver",
				"red": "red", "blue": "blue", "green": "green", "yellow": "yellow",
				"brown": "brown", "pink": "pink", "other": "another color",
			},
		},
		"condition": {
			Type: "choice",
			Criteria: map[string]string{
				"new": "like new", "good": "good condition",
				"worn": "worn or used", "damaged": "damaged",
			},
		},
		"has_crack":   {Type: "noul"},
		"has_sticker": {Type: "noul"},
		"has_case":    {Type: "noul"},
		"is_document": {Type: "noul"},
	}
}

func documentTypeQuestion() map[string]Question {
	return map[string]Question{
		"document_type": {
			Type: "choice",
			Criteria: map[string]string{
				"identity_card":   "identity card",
				"passport":        "passport",
				"driving_license": "driving license",
				"credit_card":     "credit or debit card",
				"health_card":     "health card",
				"other":           "another document",
			},
		},
	}
}

// Catalog classifies the photos with D3 and merges the answers over the form
// values. A missing, failing or empty D3 endpoint never fails enrollment: the
// result falls back to the form with Source "form".
func (s *service) Catalog(ctx context.Context, photos []ImageInput, form CatalogForm) (CatalogResult, error) {
	if s.deps.D3 == nil {
		return catalogFromForm(form), nil
	}

	images := s.dataURLs(photos)
	answers, err := s.deps.D3.Ask(ctx, catalogState, images, catalogQuestions())
	if err != nil || len(answers) == 0 {
		// The caller should not have to distinguish "D3 is down" from "D3
		// answered nothing": both mean the form is the source of truth.
		return catalogFromForm(form), nil
	}

	result := catalogFromForm(form)
	result.Source = CatalogSourceD3
	result.Category = choiceOrFallback(answers["category"], result.Category)
	result.Color = choiceOrFallback(answers["color"], result.Color)
	result.Condition = choiceOrFallback(answers["condition"], result.Condition)
	result.HasCrack = noulOrFallback(answers["has_crack"], result.HasCrack)
	result.HasSticker = noulOrFallback(answers["has_sticker"], result.HasSticker)
	result.HasCase = noulOrFallback(answers["has_case"], result.HasCase)
	result.IsDocument = noulOrFallback(answers["is_document"], result.IsDocument)

	if result.IsDocument {
		// document_type is only meaningful once the item is known to be a
		// document; a failure here keeps the form value and does not discard
		// the answers already obtained.
		if docAnswers, docErr := s.deps.D3.Ask(ctx, catalogState, images, documentTypeQuestion()); docErr == nil {
			result.DocumentType = choiceOrFallback(docAnswers["document_type"], result.DocumentType)
		}
	}

	return result, nil
}

// catalogFromForm returns the form values tagged as their own source.
func catalogFromForm(form CatalogForm) CatalogResult {
	return CatalogResult{
		Category:     strings.TrimSpace(form.Category),
		Color:        strings.TrimSpace(form.Color),
		Condition:    strings.TrimSpace(form.Condition),
		DocumentType: strings.TrimSpace(form.DocumentType),
		HasCrack:     form.HasCrack,
		HasSticker:   form.HasSticker,
		HasCase:      form.HasCase,
		IsDocument:   form.IsDocument,
		Source:       CatalogSourceForm,
	}
}

// dataURLs converts the photos into the inline data URLs accepted by d3.
// Photos that cannot be read are skipped: enrollment must not fail because one
// upload is missing from disk, and the form fallback still has a value for
// every field.
func (s *service) dataURLs(photos []ImageInput) []string {
	urls := make([]string, 0, len(photos))
	for _, photo := range photos {
		data := photo.Data
		if len(data) == 0 && photo.Path != "" && s.deps.Storage != nil {
			stored, err := s.deps.Storage.Read(photo.Path)
			if err != nil {
				continue
			}
			data = stored
		}
		if len(data) == 0 {
			continue
		}

		mediaType := photo.MediaType
		if mediaType == "" {
			mediaType = http.DetectContentType(data)
		}
		urls = append(urls, "data:"+mediaType+";base64,"+base64.StdEncoding.EncodeToString(data))
	}
	return urls
}

// choiceOrFallback returns the explicit choice, then the most probable option,
// then the form value.
func choiceOrFallback(answer Answer, fallback string) string {
	if strings.TrimSpace(answer.Choice) != "" {
		return strings.TrimSpace(answer.Choice)
	}
	best, bestProbability := "", -1.0
	for option, probability := range answer.Probabilities {
		if probability > bestProbability {
			best, bestProbability = option, probability
		}
	}
	if best != "" {
		return best
	}
	return fallback
}

// noulOrFallback maps a yes/no probability at the 0.5 threshold.
func noulOrFallback(answer Answer, fallback bool) bool {
	if answer.Type != "noul" {
		return fallback
	}
	return answer.Noul >= 0.5
}
