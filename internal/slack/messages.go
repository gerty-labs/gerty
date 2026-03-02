package slack

import (
	"fmt"
	"strings"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

// Severity levels for recommendation notifications.
type Severity string

const (
	SeverityCritical      Severity = "critical"
	SeverityOptimisation  Severity = "optimisation"
	SeverityInformational Severity = "informational"
)

// BlockItem is a Slack Block Kit block element.
type BlockItem struct {
	Type     string       `json:"type"`
	Text     *TextObject  `json:"text,omitempty"`
	Fields   []TextObject `json:"fields,omitempty"`
	Elements []BlockItem  `json:"elements,omitempty"`
	ActionID string       `json:"action_id,omitempty"`
	Value    string       `json:"value,omitempty"`
	Style    string       `json:"style,omitempty"`
}

// TextObject represents a Slack text object (mrkdwn or plain_text).
type TextObject struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ClassifySeverity maps a recommendation to a notification severity.
func ClassifySeverity(rec models.Recommendation) Severity {
	if rec.Risk == models.RiskHigh {
		return SeverityCritical
	}
	if rec.Confidence >= 0.7 {
		return SeverityOptimisation
	}
	return SeverityInformational
}

// RecKey returns a deduplication key for a recommendation.
func RecKey(rec models.Recommendation) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		rec.Target.Namespace, rec.Target.Kind, rec.Target.Name,
		rec.Container, rec.Resource)
}

// BuildDigestMessage builds a Block Kit message for a batch of recommendations
// grouped by namespace.
func BuildDigestMessage(recs []models.Recommendation) WebhookPayload {
	if len(recs) == 0 {
		return WebhookPayload{
			Text: "k8s-sage: No new recommendations.",
		}
	}

	// Group by namespace.
	byNS := make(map[string][]models.Recommendation)
	for _, r := range recs {
		ns := r.Target.Namespace
		byNS[ns] = append(byNS[ns], r)
	}

	blocks := []BlockItem{
		{
			Type: "header",
			Text: &TextObject{
				Type: "plain_text",
				Text: fmt.Sprintf("k8s-sage: %d new recommendation(s)", len(recs)),
			},
		},
	}

	for ns, nsRecs := range byNS {
		blocks = append(blocks, BlockItem{
			Type: "section",
			Text: &TextObject{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Namespace: %s* (%d)", ns, len(nsRecs)),
			},
		})

		for _, rec := range nsRecs {
			blocks = append(blocks, buildRecBlock(rec))
		}

		blocks = append(blocks, BlockItem{Type: "divider"})
	}

	return WebhookPayload{
		Text:   fmt.Sprintf("k8s-sage: %d new recommendation(s)", len(recs)),
		Blocks: blocks,
	}
}

// BuildSingleMessage builds a Block Kit message for a single recommendation.
func BuildSingleMessage(rec models.Recommendation) WebhookPayload {
	blocks := []BlockItem{
		{
			Type: "header",
			Text: &TextObject{
				Type: "plain_text",
				Text: fmt.Sprintf("k8s-sage: %s recommendation", string(ClassifySeverity(rec))),
			},
		},
		buildRecBlock(rec),
		buildActionBlock(rec),
	}

	return WebhookPayload{
		Text:   fmt.Sprintf("k8s-sage: %s %s/%s recommendation", rec.Resource, rec.Target.Kind, rec.Target.Name),
		Blocks: blocks,
	}
}

// buildRecBlock creates a section block with recommendation fields.
func buildRecBlock(rec models.Recommendation) BlockItem {
	workload := fmt.Sprintf("%s/%s", rec.Target.Kind, rec.Target.Name)
	currentReq := fmt.Sprintf("%d", rec.CurrentRequest)
	recommendedReq := fmt.Sprintf("%d", rec.RecommendedReq)

	var resourceUnit string
	if rec.Resource == "cpu" {
		resourceUnit = "m"
	} else {
		resourceUnit = " bytes"
	}

	fields := []TextObject{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Workload:*\n%s", workload)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Container:*\n%s", rec.Container)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*%s:*\n%s%s → %s%s", strings.ToUpper(rec.Resource), currentReq, resourceUnit, recommendedReq, resourceUnit)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Confidence:*\n%.0f%%", rec.Confidence*100)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Risk:*\n%s", rec.Risk)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Reasoning:*\n%s", truncate(rec.Reasoning, 200))},
	}

	return BlockItem{
		Type:   "section",
		Fields: fields,
	}
}

// buildActionBlock creates an actions block with Create PR and Acknowledge buttons.
func buildActionBlock(rec models.Recommendation) BlockItem {
	key := RecKey(rec)
	return BlockItem{
		Type: "actions",
		Elements: []BlockItem{
			{
				Type:     "button",
				Text:     &TextObject{Type: "plain_text", Text: "Create PR"},
				ActionID: "create_pr",
				Value:    key,
				Style:    "primary",
			},
			{
				Type:     "button",
				Text:     &TextObject{Type: "plain_text", Text: "Acknowledge"},
				ActionID: "acknowledge",
				Value:    key,
			},
		},
	}
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
