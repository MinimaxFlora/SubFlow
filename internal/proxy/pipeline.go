package proxy

import (
	"fmt"
	"strings"
)

// ConvertOptions describes a full conversion request.
type ConvertOptions struct {
	// Content is raw subscription text (URI list or Base64).
	Content string
	// Nodes may be supplied directly instead of Content.
	Nodes []Node
	// Operations are applied in order after parsing.
	Operations []Operation
	// Platform is the target output format.
	Platform string
	// IncludeUnsupported keeps nodes a target cannot natively represent.
	IncludeUnsupported bool
}

// ConvertResult carries the produced output together with pipeline stats.
type ConvertResult struct {
	Output   string `json:"output"`
	Platform string `json:"platform"`
	Total    int    `json:"total"`
	Emitted  int    `json:"emitted"`
	// Source is the final URL when the content was fetched remotely.
	Source   string   `json:"source,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Convert runs the full parse -> operate -> produce pipeline.
func Convert(opts ConvertOptions) (ConvertResult, error) {
	if !IsPlatform(opts.Platform) {
		return ConvertResult{}, fmt.Errorf("unknown platform %q", opts.Platform)
	}

	nodes := opts.Nodes
	if len(nodes) == 0 && opts.Content != "" {
		parsed, err := ParseSubscription(opts.Content)
		if err != nil {
			return ConvertResult{}, err
		}
		nodes = parsed
	}

	processed, err := ApplyOperations(nodes, opts.Operations)
	if err != nil {
		return ConvertResult{}, err
	}

	emitted := 0
	for _, n := range processed {
		if opts.IncludeUnsupported || Supports(opts.Platform, n) {
			emitted++
		}
	}

	output, err := Produce(processed, opts.Platform, Options{IncludeUnsupported: opts.IncludeUnsupported})
	if err != nil {
		return ConvertResult{}, err
	}

	result := ConvertResult{
		Output:   output,
		Platform: opts.Platform,
		Total:    len(processed),
		Emitted:  emitted,
	}
	if emitted < len(processed) {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"%d node(s) were omitted because %s cannot represent them",
			len(processed)-emitted, opts.Platform))
	}
	return result, nil
}

// NormalizeContent trims BOM and surrounding whitespace from subscription text.
func NormalizeContent(content string) string {
	return strings.TrimSpace(strings.TrimPrefix(content, "\ufeff"))
}
