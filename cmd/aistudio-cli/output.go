package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"aistudio-cli/internal/aistudio"
)

type nonStreamResult struct {
	Model        string          `json:"model"`
	Text         string          `json:"text"`
	Reasoning    string          `json:"reasoning,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
	Usage        *aistudio.Usage `json:"usage,omitempty"`
}

type eventCollector struct {
	asJSON bool
	stream bool

	textBuilder      strings.Builder
	reasoningBuilder strings.Builder
	finishReason     string
	providerModel    string
	usage            *aistudio.Usage
}

func newEventCollector(asJSON bool, stream bool) *eventCollector {
	return &eventCollector{
		asJSON: asJSON,
		stream: stream,
	}
}

func (c *eventCollector) Handle(event aistudio.Event) error {
	if event.Err != nil {
		return event.Err
	}

	if event.ProviderModel != "" {
		c.providerModel = event.ProviderModel
	}
	if event.Usage != nil {
		c.usage = event.Usage
	}
	if event.FinishReason != "" {
		c.finishReason = event.FinishReason
	}

	switch event.Kind {
	case aistudio.EventText:
		c.textBuilder.WriteString(event.Text)
		if c.stream && !c.asJSON {
			fmt.Print(event.Text)
			_ = os.Stdout.Sync()
		}
	case aistudio.EventReasoning:
		c.reasoningBuilder.WriteString(event.Text)
	case aistudio.EventFinish:
		if c.finishReason == "" {
			c.finishReason = "STOP"
		}
	}

	if c.stream && c.asJSON {
		line, err := json.Marshal(event)
		if err != nil {
			return err
		}
		fmt.Println(string(line))
	}

	return nil
}

func (c *eventCollector) Finish() error {
	if c.stream {
		if !c.asJSON {
			fmt.Println()
		}
		return nil
	}

	res := nonStreamResult{
		Model:        c.providerModel,
		Text:         c.textBuilder.String(),
		Reasoning:    c.reasoningBuilder.String(),
		FinishReason: c.finishReason,
		Usage:        c.usage,
	}

	if c.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	if res.Reasoning != "" {
		fmt.Println("[思考过程]")
		fmt.Println(res.Reasoning)
		fmt.Println("--------------------")
	}
	fmt.Print(res.Text)
	if !strings.HasSuffix(res.Text, "\n") {
		fmt.Println()
	}
	return nil
}
