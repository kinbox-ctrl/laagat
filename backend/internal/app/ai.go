package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"buildcost/internal/calc"
)

// AI is the assistance layer. It only ever produces suggestions and drafts
// that an analyst must accept: it never writes quantities, rates, mappings or
// report text directly.
type AI struct {
	client anthropic.Client
	model  anthropic.Model
}

func NewAI(model string) *AI {
	if model == "" {
		model = "claude-opus-5-5"
	}
	return &AI{client: anthropic.NewClient(), model: model}
}

func (a *AI) ask(ctx context.Context, system, user string) (string, error) {
	resp, err := a.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     a.model,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user)),
		},
		// If a safety classifier declines the request, let the API re-serve it
		// on its default fallback model instead of failing.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case 401, 403:
				return "", errors.New("the AI service rejected the configured credentials")
			case 429:
				return "", errors.New("the AI service is rate limiting requests; try again shortly")
			}
			return "", fmt.Errorf("AI service error (HTTP %d)", apiErr.StatusCode)
		}
		return "", errors.New("could not reach the AI service")
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return "", errors.New("the AI service declined this request")
	}
	if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
		return "", errors.New("the AI response was cut off before it finished")
	}
	var sb strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// SuggestWorkItems proposes a catalog work item code for each BOQ description
// that the keyword rules could not classify. The result is index-aligned with
// descriptions; "" means no suggestion. Codes outside the catalog are dropped.
func (a *AI) SuggestWorkItems(ctx context.Context, descriptions []string) ([]string, error) {
	var cat strings.Builder
	for _, w := range calc.Catalog {
		fmt.Fprintf(&cat, "- %s: %s (measured in %s)\n", w.Code, w.Name, w.Unit)
	}
	var items strings.Builder
	for i, d := range descriptions {
		fmt.Fprintf(&items, "%d. %s\n", i, d)
	}
	system := "You help a quantity surveyor classify line items from Indian residential construction BOQs and contractor quotations. " +
		"The surveyor reviews every suggestion, so prefer no suggestion over a doubtful one.\n\n" +
		"Supported work items:\n" + cat.String() + "\n" +
		"For each numbered line, choose the single work item code that the line is charging for, or null when none of the supported items fits " +
		"(for example furniture, site overheads, or items that combine several kinds of work). " +
		"Reply with only a JSON object mapping each line number to a code or null, for example {\"0\": \"WP\", \"1\": null}."
	out, err := a.ask(ctx, system, items.String())
	if err != nil {
		return nil, err
	}
	start, end := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if start < 0 || end <= start {
		return nil, errors.New("the AI reply was not in the expected format")
	}
	var parsed map[string]*string
	if err := json.Unmarshal([]byte(out[start:end+1]), &parsed); err != nil {
		return nil, errors.New("the AI reply was not in the expected format")
	}
	codes := make([]string, len(descriptions))
	for i := range descriptions {
		if c := parsed[fmt.Sprint(i)]; c != nil && calc.Item(*c) != nil {
			codes[i] = *c
		}
	}
	return codes, nil
}

// DraftFindingNote writes a plain-language explanation of a verified finding
// for a homeowner. It is given only the deterministic finding and its
// evidence and is told not to introduce figures of its own.
func (a *AI) DraftFindingNote(ctx context.Context, f *FindingRow) (string, error) {
	system := "You draft short reviewer's notes for an independent construction cost assessment report in India. " +
		"The reader is a homeowner with no construction background who is about to sign a contract with a builder.\n\n" +
		"You are given one finding produced by a deterministic calculation engine, with its evidence. " +
		"Explain in two to four plain sentences what it means for the homeowner and why it matters, and what a reasonable answer from the contractor would look like. " +
		"Every number you mention must come from the finding as given; do not estimate, round differently or add figures, rates or savings of your own, " +
		"because the note sits beside audited calculations and must not contradict them. " +
		"Do not accuse the contractor of wrongdoing: a discrepancy is something to clarify, not proof of overcharging. " +
		"Write the note only, as plain prose without headings or lists. A quantity surveyor will edit it before it is used."
	user := fmt.Sprintf("Check: %s\nSeverity: %s\nFinding: %s\nExplanation: %s\nRequired action: %s\nEvidence (JSON): %s",
		f.Code, f.Severity, f.Title, f.Explanation, f.Action, string(f.Evidence))
	return a.ask(ctx, system, user)
}
