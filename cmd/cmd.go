package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/bz888/helloer/internal"
	"github.com/bz888/helloer/internal/config"
	"github.com/bz888/helloer/internal/progress"
	"github.com/bz888/helloer/internal/readline"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type displayResponseState struct {
	lineLength int
	wordBuffer string
}

func displayResponse(content string, state *displayResponseState) {
	termWidth, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if termWidth >= 10 {
		for _, ch := range content {
			if state.lineLength+1 > termWidth-5 {

				if runewidth.StringWidth(state.wordBuffer) > termWidth-10 {
					fmt.Printf("%s%c", state.wordBuffer, ch)
					state.wordBuffer = ""
					state.lineLength = 0
					continue
				}

				// backtrack the length of the last word and clear to the end of the line
				a := runewidth.StringWidth(state.wordBuffer)
				if a > 0 {
					fmt.Printf("\x1b[%dD", a)
				}
				fmt.Printf("\x1b[K\n")
				fmt.Printf("%s%c", state.wordBuffer, ch)
				chWidth := runewidth.RuneWidth(ch)

				state.lineLength = runewidth.StringWidth(state.wordBuffer) + chWidth
			} else {
				fmt.Print(string(ch))
				state.lineLength += runewidth.RuneWidth(ch)
				if runewidth.RuneWidth(ch) >= 2 {
					state.wordBuffer = ""
					continue
				}

				switch ch {
				case ' ':
					state.wordBuffer = ""
				case '\n':
					state.lineLength = 0
				default:
					state.wordBuffer += string(ch)
				}
			}
		}
	} else {
		fmt.Printf("%s%s", state.wordBuffer, content)
		if len(state.wordBuffer) > 0 {
			state.wordBuffer = ""
		}
	}
}

type ModelMessages struct {
	model1Messages []internal.Message
	model2Messages []internal.Message
}

func Generate(ctx context.Context, model config.Config) error {
	messages := ModelMessages{
		model1Messages: make([]internal.Message, 0),
		model2Messages: make([]internal.Message, 0),
	}

	scanner, err := readline.New(readline.Prompt{
		Prompt:         ">>> ",
		AltPrompt:      "... ",
		Placeholder:    "Message",
		AltPlaceholder: `Use """ to end multi-line input`,
	})
	if err != nil {
		return err
	}

	fmt.Print(readline.StartBracketedPaste)
	defer fmt.Printf(readline.EndBracketedPaste)

	var sb strings.Builder
	lastModelIndex := 0 // to track the last model used

	for {
		line, err := scanner.Readline()

		switch {
		case errors.Is(err, io.EOF):
			fmt.Println()
			return nil
		case errors.Is(err, readline.ErrInterrupt):
			if line == "" {
				fmt.Println("\nUse Ctrl + d or /bye to exit.")
			}

			scanner.Prompt.UseAlt = false
			sb.Reset()

			continue
		case err != nil:
			return err
		case strings.HasPrefix(line, "/exit"), strings.HasPrefix(line, "/bye"):
			return nil
		case strings.HasPrefix(line, "/continue"):
			currentMessages := messages.model1Messages
			if lastModelIndex == 1 {
				currentMessages = messages.model2Messages
			}

			assistant, err := chat(ctx, currentMessages, model[lastModelIndex].Name)
			if err != nil {
				return err
			}

			if assistant != nil {
				if lastModelIndex == 0 {
					messages.model1Messages = append(messages.model1Messages, *assistant)
					invertedAssistant := *assistant
					if assistant.Role == "assistant" {
						invertedAssistant.Role = "user"
					} else if assistant.Role == "user" {
						invertedAssistant.Role = "assistant"
					}
					messages.model2Messages = append(messages.model2Messages, invertedAssistant)
				} else {
					messages.model2Messages = append(messages.model2Messages, *assistant)
					invertedAssistant := *assistant
					if assistant.Role == "assistant" {
						invertedAssistant.Role = "user"
					} else if assistant.Role == "user" {
						invertedAssistant.Role = "assistant"
					}
					messages.model1Messages = append(messages.model1Messages, invertedAssistant)
				}
				lastModelIndex = (lastModelIndex + 1) % len(model) // toggle between models
			}
			continue
		default:
			sb.WriteString(line)
		}

		if sb.Len() > 0 {
			role, err := getRole(scanner)
			if err != nil {
				return err
			}

			newMessage := internal.Message{Role: role, Content: sb.String()}

			messages.model1Messages = append(messages.model1Messages, newMessage)
			messages.model2Messages = append(messages.model2Messages, newMessage)

			switch role {
			case "user", "system":
				currentMessages := messages.model1Messages
				if lastModelIndex == 1 {
					currentMessages = messages.model2Messages
				}

				assistant, err := chat(ctx, currentMessages, model[lastModelIndex].Name)
				if err != nil {
					return err
				}

				if assistant != nil {
					if lastModelIndex == 0 {
						messages.model1Messages = append(messages.model1Messages, *assistant)
						invertedAssistant := *assistant
						if assistant.Role == "assistant" {
							invertedAssistant.Role = "user"
						} else if assistant.Role == "user" {
							invertedAssistant.Role = "assistant"
						}
						messages.model2Messages = append(messages.model2Messages, invertedAssistant)
					} else {
						messages.model2Messages = append(messages.model2Messages, *assistant)
						invertedAssistant := *assistant
						if assistant.Role == "assistant" {
							invertedAssistant.Role = "user"
						} else if assistant.Role == "user" {
							invertedAssistant.Role = "assistant"
						}
						messages.model1Messages = append(messages.model1Messages, invertedAssistant)
					}
					lastModelIndex = (lastModelIndex + 1) % len(model) // toggle between models
				}
			case "assistant":
				fmt.Println("Continuing as assistant...")

				messages.model1Messages = append(messages.model1Messages, newMessage)
				messages.model2Messages = append(messages.model2Messages, newMessage)
				continue
			}
			sb.Reset()
		}
	}

}

func chat(ctx context.Context, messages []internal.Message, model string) (*internal.Message, error) {
	log.Printf("Current model %v\n", model)
	client := internal.NewClient(
		&url.URL{
			Scheme: "http",
			Host:   "localhost:11434",
		},
		http.DefaultClient)

	p := progress.NewProgress(os.Stderr)
	defer p.StopAndClear()

	spinner := progress.NewSpinner("")
	p.Add("", spinner)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	go func() {
		<-sigChan
		cancel()
	}()

	var state *displayResponseState = &displayResponseState{}
	//var latest internal.ChatResponse
	var fullResponse strings.Builder
	var role string

	fn := func(response internal.ChatResponse) error {
		p.StopAndClear()

		//latest = response

		role = response.Message.Role
		content := response.Message.Content
		fullResponse.WriteString(content)

		displayResponse(content, state)

		return nil
	}

	req := &internal.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}

	if err := client.Chat(cancelCtx, req, fn); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, err
	}

	if len(messages) > 0 {
		fmt.Println()
		fmt.Println()
	}

	return &internal.Message{Role: role, Content: fullResponse.String()}, nil
}

func getRole(scanner *readline.Instance) (string, error) {
	for {
		fmt.Print("Select role (user/assistant/system): ")
		line, err := scanner.Readline()
		if err != nil {
			return "", err
		}
		role := strings.TrimSpace(line)
		if role == "user" || role == "assistant" || role == "system" {
			return role, nil
		}
		fmt.Println("Invalid role. Please select either 'user', 'assistant', or 'system'.")
	}
}

func configModels() {

}

func listModels() {

}
