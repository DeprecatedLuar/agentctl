package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/joho/godotenv"
)

const (
	openRouterAPIBase = "https://openrouter.ai/api/v1"

	// Filter flags for HandleModels
	// flagTools ("--tools") is shared with chat.go/flags.go.
	flagSTT  = "--stt"
	flagFree = "--free"
	flagAll  = "--all"
)

// staticModel represents a pre-baked model entry
type staticModel struct {
	id            string
	contextLen    int
	priceIn       string // price per 1M tokens
	priceOut      string // price per 1M tokens
	supportsTools bool
	isSTT         bool
}

// OpenAI static models
var openAIModels = []staticModel{
	// LLM models
	{id: "gpt-4o", contextLen: 128000, priceIn: "$2.50", priceOut: "$10.00", supportsTools: true, isSTT: false},
	{id: "gpt-4o-mini", contextLen: 128000, priceIn: "$0.15", priceOut: "$0.60", supportsTools: true, isSTT: false},
	{id: "o3", contextLen: 128000, priceIn: "$10.00", priceOut: "$40.00", supportsTools: true, isSTT: false},
	{id: "o3-mini", contextLen: 128000, priceIn: "$1.10", priceOut: "$4.40", supportsTools: true, isSTT: false},
	{id: "o1", contextLen: 128000, priceIn: "$15.00", priceOut: "$60.00", supportsTools: false, isSTT: false},
	{id: "o1-mini", contextLen: 128000, priceIn: "$3.00", priceOut: "$12.00", supportsTools: false, isSTT: false},
	{id: "gpt-4-turbo", contextLen: 128000, priceIn: "$10.00", priceOut: "$30.00", supportsTools: true, isSTT: false},
	{id: "gpt-3.5-turbo", contextLen: 16385, priceIn: "$0.50", priceOut: "$1.50", supportsTools: true, isSTT: false},

	// STT models
	{id: "whisper-1", priceIn: "$6.00", isSTT: true},
	{id: "gpt-4o-transcribe", priceIn: "$100.00", isSTT: true},
	{id: "gpt-4o-mini-transcribe", priceIn: "$15.00", isSTT: true},
}

// orModel represents an OpenRouter model from API
type orModel struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ContextLength   int       `json:"context_length"`
	Pricing         orPricing `json:"pricing"`
	SupportedParams []string  `json:"supported_parameters"`
}

type orPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

func (m orModel) isFree() bool {
	return strings.HasSuffix(m.ID, ":free") || (m.Pricing.Prompt == "0" && m.Pricing.Completion == "0")
}

func (m orModel) supportsTools() bool {
	for _, p := range m.SupportedParams {
		if p == "tools" {
			return true
		}
	}
	return false
}

// Popular providers to show by default
var popularProviders = []string{
	"anthropic/",
	"openai/",
	"google/",
	"meta/",
	"microsoft/",
	"mistralai/",
	"nvidia/",
	"qwen/",
	"deepseek/",
	"x-ai/",
	"openrouter/",
	"~", // Always show curated aliases
}

func (m orModel) isPopularProvider() bool {
	for _, provider := range popularProviders {
		if strings.HasPrefix(m.ID, provider) {
			return true
		}
	}
	return false
}

func (m orModel) provider() string {
	// Extract provider from ID (e.g., "anthropic/claude-opus" -> "anthropic")
	parts := strings.SplitN(m.ID, "/", 2)
	if len(parts) > 1 {
		return parts[0]
	}
	return "other"
}

func (m orModel) hasValidPricing() bool {
	// Filter out models with negative or placeholder pricing
	prompt, errP := strconv.ParseFloat(m.Pricing.Prompt, 64)
	completion, errC := strconv.ParseFloat(m.Pricing.Completion, 64)
	if errP != nil || errC != nil {
		return false
	}
	return prompt >= 0 && completion >= 0
}

func HandleModels(args []string) error {
	// Load .env from current directory (silent if missing)
	_ = godotenv.Load()

	// Parse flags and provider
	var stt, tools, free, all bool
	var provider string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case flagSTT:
			stt = true
		case flagTools:
			tools = true
		case flagFree:
			free = true
		case flagAll:
			all = true
		default:
			if provider == "" {
				provider = arg
			} else {
				return fmt.Errorf("unexpected argument: %s", arg)
			}
		}
	}

	// Validate provider
	if provider != "" && provider != "openai" && provider != "openrouter" {
		return fmt.Errorf("unrecognized provider: %s (use 'openai' or 'openrouter')", provider)
	}

	// Show both if no provider specified
	showBoth := provider == ""

	// Call appropriate handler based on STT vs LLM
	if stt {
		if showBoth || provider == "openai" {
			printOpenAISTTModels(showBoth)
		}
		if showBoth {
			fmt.Println() // Blank line separator
		}
		if showBoth || provider == "openrouter" {
			if err := printOpenRouterSTTModels(free, showBoth); err != nil {
				return err
			}
		}
	} else {
		if showBoth || provider == "openai" {
			printOpenAILLMModels(tools, free, showBoth)
		}
		if showBoth {
			fmt.Println() // Blank line separator
		}
		if showBoth || provider == "openrouter" {
			if err := printOpenRouterLLMModels(tools, free, all, showBoth); err != nil {
				return err
			}
		}
	}

	return nil
}

func printOpenAILLMModels(tools, free, showHeader bool) {
	if showHeader {
		fmt.Println("OpenAI Models")
		fmt.Println("─────────────")
	}

	if free {
		fmt.Println("No free OpenAI models available")
		return
	}

	// Filter LLM models
	var filtered []staticModel
	for _, m := range openAIModels {
		if m.isSTT {
			continue
		}
		if tools && !m.supportsTools {
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		fmt.Println("No models match the specified filters")
		return
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCTX\tIN\tOUT\tTOOLS")
	for _, m := range filtered {
		ctx := formatContextLen(m.contextLen)
		toolsStr := "✗"
		if m.supportsTools {
			toolsStr = "✓"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", m.id, ctx, m.priceIn, m.priceOut, toolsStr)
	}
	w.Flush()
	fmt.Printf("\n%d model(s)\n", len(filtered))
}

func printOpenAISTTModels(showHeader bool) {
	if showHeader {
		fmt.Println("OpenAI Models")
		fmt.Println("─────────────")
	}

	// Filter STT models
	var filtered []staticModel
	for _, m := range openAIModels {
		if !m.isSTT {
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		fmt.Println("No models match the specified filters")
		return
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPRICING")
	for _, m := range filtered {
		fmt.Fprintf(w, "%s\t%s/hour\n", m.id, m.priceIn)
	}
	w.Flush()
	fmt.Printf("\n%d model(s)\n", len(filtered))
}

func printOpenRouterLLMModels(tools, free, all, showHeader bool) error {
	if showHeader {
		fmt.Println("OpenRouter Models")
		fmt.Println("─────────────────")
	}

	// Fetch LLM models
	models, err := fetchOpenRouterModels("")
	if err != nil {
		return err
	}

	// Filter LLM models
	var filtered []orModel
	for _, m := range models {
		// Skip models with invalid pricing
		if !m.hasValidPricing() {
			continue
		}
		// Unless --all is set, show only: free models OR popular providers
		if !all && !m.isFree() && !m.isPopularProvider() {
			continue
		}
		if free && !m.isFree() {
			continue
		}
		if tools && !m.supportsTools() {
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		fmt.Println("No models match the specified filters")
		return nil
	}

	// Group models by: free/paid -> tools/no-tools -> provider
	type group struct {
		name   string
		models []orModel
	}

	groups := []group{
		{name: "Free Models with Tools", models: []orModel{}},
		{name: "Free Models without Tools", models: []orModel{}},
		{name: "Paid Models with Tools", models: []orModel{}},
		{name: "Paid Models without Tools", models: []orModel{}},
	}

	for _, m := range filtered {
		isFree := m.isFree()
		hasTools := m.supportsTools()

		var idx int
		if isFree && hasTools {
			idx = 0
		} else if isFree && !hasTools {
			idx = 1
		} else if !isFree && hasTools {
			idx = 2
		} else {
			idx = 3
		}

		groups[idx].models = append(groups[idx].models, m)
	}

	// Collect all models in order
	var orderedModels []orModel

	for i, grp := range groups {
		// Skip "Paid Models without Tools" unless --all is set
		if i == 3 && !all {
			continue
		}

		if len(grp.models) == 0 {
			continue
		}

		// Group by provider within this category
		providerMap := make(map[string][]orModel)
		for _, m := range grp.models {
			p := m.provider()
			providerMap[p] = append(providerMap[p], m)
		}

		// Sort providers alphabetically
		var providers []string
		for p := range providerMap {
			providers = append(providers, p)
		}
		sort.Strings(providers)

		// Add models in provider order
		for _, provider := range providers {
			orderedModels = append(orderedModels, providerMap[provider]...)
		}
	}

	// Print single table for LLM models
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCTX\tPRICE\tTOOLS")
	for _, m := range orderedModels {
		ctx := formatContextLen(m.ContextLength)
		price := formatPriceColumn(m.Pricing.Prompt, m.Pricing.Completion)
		toolsStr := "✗"
		if m.supportsTools() {
			toolsStr = "✓"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.ID, ctx, price, toolsStr)
	}
	w.Flush()

	fmt.Printf("\n%d model(s)\n", len(orderedModels))

	return nil
}

func printOpenRouterSTTModels(free, showHeader bool) error {
	if showHeader {
		fmt.Println("OpenRouter Models")
		fmt.Println("─────────────────")
	}

	// Fetch STT models
	models, err := fetchOpenRouterModels("transcription")
	if err != nil {
		return err
	}

	// Filter STT models (simple - just free filter)
	var filtered []orModel
	for _, m := range models {
		if free && !m.isFree() {
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		fmt.Println("No models match the specified filters")
		return nil
	}

	// Sort by provider
	providerMap := make(map[string][]orModel)
	for _, m := range filtered {
		p := m.provider()
		providerMap[p] = append(providerMap[p], m)
	}

	var providers []string
	for p := range providerMap {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	// Collect ordered models
	var orderedModels []orModel
	for _, provider := range providers {
		orderedModels = append(orderedModels, providerMap[provider]...)
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPRICING")
	for _, m := range orderedModels {
		// STT pricing: multiply by 1000 to get per-hour
		price, _ := strconv.ParseFloat(m.Pricing.Prompt, 64)
		var priceStr string
		if price == 0 {
			priceStr = "free"
		} else {
			priceStr = fmt.Sprintf("$%.2f/hr", price*1000)
		}
		fmt.Fprintf(w, "%s\t%s\n", m.ID, priceStr)
	}
	w.Flush()

	fmt.Printf("\n%d model(s)\n", len(orderedModels))

	return nil
}

func fetchOpenRouterModels(outputModality string) ([]orModel, error) {
	url := openRouterAPIBase + "/models"
	if outputModality != "" {
		url += "?output_modalities=" + outputModality
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// No auth required - /models endpoint is public

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []orModel `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Data, nil
}

// formatPricePerM converts per-token price string to $/M format
func formatPricePerM(perToken string) string {
	if perToken == "" || perToken == "0" {
		return "free"
	}

	price, err := strconv.ParseFloat(perToken, 64)
	if err != nil {
		return perToken
	}

	if price == 0 {
		return "free"
	}

	// Convert to per million
	perMillion := price * 1_000_000
	return fmt.Sprintf("$%.2f", perMillion)
}

// formatPriceColumn collapses "free/free" to "free"
func formatPriceColumn(in, out string) string {
	inFmt := formatPricePerM(in)
	outFmt := formatPricePerM(out)

	if inFmt == "free" && outFmt == "free" {
		return "free"
	}

	return inFmt + "/" + outFmt
}

// formatContextLen formats context length with K/M suffixes
func formatContextLen(n int) string {
	if n == 0 {
		return "-"
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%dM", n/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%dK", n/1_000)
	}
	return fmt.Sprintf("%d", n)
}
