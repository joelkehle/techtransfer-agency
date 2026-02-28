package priorartsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const systemPrompt = "You are a patent search strategist and analyst for a university technology transfer office. You produce conservative, structured outputs and do not invent facts. Return strict JSON only."

var statusCodeRe = regexp.MustCompile(`(?:status(?:\\s+code)?[:=\\s]+)(\\d{3})`)

type llmFailureClass int

const (
	failureNone llmFailureClass = iota
	failureTimeout
	failureRateLimit
	failureServer
	failureClient
)

type LLMCaller interface {
	GenerateJSON(ctx context.Context, prompt string) (string, error)
	ModelName() string
}

type AnthropicMessager interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

type AnthropicCaller struct {
	messages AnthropicMessager
	model    string
}

type AnthropicClientCreator func(apiKey string) AnthropicMessager

func defaultAnthropicCreator(apiKey string) AnthropicMessager {
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &c.Messages
}

var newAnthropicClient AnthropicClientCreator = defaultAnthropicCreator

func NewAnthropicCallerFromEnv() (*AnthropicCaller, error) {
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY not configured")
	}
	model := strings.TrimSpace(os.Getenv("PRIOR_ART_LLM_MODEL"))
	if model == "" {
		model = DefaultLLMModel
	}
	return &AnthropicCaller{messages: newAnthropicClient(apiKey), model: model}, nil
}

func (a *AnthropicCaller) ModelName() string { return a.model }

func (a *AnthropicCaller) GenerateJSON(ctx context.Context, prompt string) (string, error) {
	resp, err := a.messages.New(ctx, anthropic.MessageNewParams{
		Model:       anthropic.Model(a.model),
		MaxTokens:   4096,
		System:      []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:    []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		Temperature: anthropic.Float(0),
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, b := range resp.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String(), nil
}

type StageExecutor struct {
	caller LLMCaller
}

func NewStageExecutor(caller LLMCaller) *StageExecutor {
	return &StageExecutor{caller: caller}
}

func (e *StageExecutor) ModelName() string {
	if e == nil || e.caller == nil {
		return DefaultLLMModel
	}
	return e.caller.ModelName()
}

func (e *StageExecutor) Run(ctx context.Context, stageName, prompt string, out any, validate func() error) (StageAttemptMetrics, error) {
	metrics := StageAttemptMetrics{}
	feedback := ""
	for attempt := 1; attempt <= 3; attempt++ {
		metrics.Attempts = attempt
		fullPrompt := prompt
		if feedback != "" {
			fullPrompt += "\n\n" + feedback
		}

		raw, err := e.caller.GenerateJSON(ctx, fullPrompt)
		if err != nil {
			class := classifyTransportError(err)
			if class == failureTimeout || class == failureRateLimit || class == failureServer {
				if attempt < 3 {
					time.Sleep(backoffDelay(attempt))
					continue
				}
			}
			return metrics, fmt.Errorf("%s transport failure: %w", stageName, err)
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			if attempt < 3 {
				metrics.ContentRetries++
				feedback = "Your previous response was empty. Return valid JSON only."
				continue
			}
			return metrics, fmt.Errorf("%s failed: empty response", stageName)
		}

		clean := stripCodeFences(raw)
		if err := json.Unmarshal([]byte(clean), out); err != nil {
			if attempt < 3 {
				metrics.ContentRetries++
				feedback = "Your previous response was not valid JSON. Return valid JSON only."
				continue
			}
			return metrics, fmt.Errorf("%s failed json parse: %w", stageName, err)
		}
		if err := validate(); err != nil {
			if attempt < 3 {
				metrics.ContentRetries++
				feedback = fmt.Sprintf("Your response failed validation: %s. Fix and return valid JSON only.", err)
				continue
			}
			return metrics, fmt.Errorf("%s failed validation: %w", stageName, err)
		}
		return metrics, nil
	}
	return metrics, fmt.Errorf("%s failed after retries", stageName)
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		parts := strings.SplitN(s, "\n", 2)
		if len(parts) == 2 {
			s = parts[1]
		}
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
	}
	return s
}

func classifyTransportError(err error) llmFailureClass {
	msg := strings.ToLower(err.Error())
	if errors.Is(err, context.DeadlineExceeded) {
		return failureTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return failureTimeout
	}
	m := statusCodeRe.FindStringSubmatch(msg)
	if len(m) == 2 {
		switch {
		case strings.HasPrefix(m[1], "429"):
			return failureRateLimit
		case strings.HasPrefix(m[1], "5"):
			return failureServer
		case strings.HasPrefix(m[1], "4"):
			return failureClient
		}
	}
	switch {
	case strings.Contains(msg, "status 429"), strings.Contains(msg, "status=429"), strings.Contains(msg, "rate limit"):
		return failureRateLimit
	case strings.Contains(msg, "status 5"), strings.Contains(msg, "status=5"), strings.Contains(msg, "status code: 5"), strings.Contains(msg, "server error"):
		return failureServer
	case strings.Contains(msg, "status 4"), strings.Contains(msg, "status=4"), strings.Contains(msg, "status code: 4"):
		return failureClient
	default:
		return failureServer
	}
}

func backoffDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 1 * time.Second
	case 2:
		return 2 * time.Second
	default:
		return 4 * time.Second
	}
}
