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

func Generate(ctx context.Context, model config.Config) error {
	messages := make([]internal.Message, 0)

	scanner, err := readline.New(readline.Prompt{
		Prompt:         ">>> ",
		AltPrompt:      "... ",
		Placeholder:    "Send a message (/? for help)",
		AltPlaceholder: `Use """ to end multi-line input`,
	})
	if err != nil {
		return err
	}

	fmt.Print(readline.StartBracketedPaste)
	defer fmt.Printf(readline.EndBracketedPaste)

	var sb strings.Builder

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
		default:
			sb.WriteString(line)
		}

		if sb.Len() > 0 {
			newMessage := internal.Message{Role: "user", Content: sb.String()}

			messages = append(messages, newMessage)

			assistant, err := chat(ctx, messages, model[0].Name)
			if err != nil {
				return err
			}
			if assistant != nil {
				messages = append(messages, *assistant)
			}

			sb.Reset()
		}
	}

}

func chat(ctx context.Context, messages []internal.Message, model string) (*internal.Message, error) {
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
