package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
)

type model struct {
	filepicker   filepicker.Model
	selectedFile string
	quitting     bool
	err          error
}

type clearErrorMsg struct{}

func clearErrorAfter(t time.Duration) tea.Cmd {
	return tea.Tick(t, func(_ time.Time) tea.Msg {
		return clearErrorMsg{}
	})
}

func (m model) Init() tea.Cmd {
	return m.filepicker.Init()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}
	case clearErrorMsg:
		m.err = nil
	}

	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	// Did the user select a file?
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		// Get the path of the selected file.
		m.selectedFile = path
		if fileInfo, err := os.Stat(path); err == nil && fileInfo.IsDir() {
			fmt.Println("Directory selected!")
			m.quitting = true
			return m, tea.Quit
		} else {
			m.err = errors.New(path + " is not a directory.")
			m.selectedFile = ""
			return m, tea.Batch(cmd, clearErrorAfter(2*time.Second))
		}
	}

	// Did the user select a disabled file?
	// This is only necessary to display an error to the user.
	if didSelect, path := m.filepicker.DidSelectDisabledFile(msg); didSelect {
		// Let's clear the selectedFile and display an error.
		m.err = errors.New(path + " is not valid.")
		m.selectedFile = ""
		return m, tea.Batch(cmd, clearErrorAfter(2*time.Second))
	}

	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	var s strings.Builder
	s.WriteString("\n  ")
	if m.err != nil {
		s.WriteString(m.filepicker.Styles.DisabledFile.Render(m.err.Error()))
	} else if m.selectedFile == "" {
		s.WriteString("Pick a directory:")
	} else {
		s.WriteString("Selected file: " + m.filepicker.Styles.Selected.Render(m.selectedFile))
	}
	s.WriteString("\n\n" + m.filepicker.View() + "\n")
	return s.String()
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	apiKey := os.Getenv("OPENAI_KEY")

	fmt.Println("Enter brain dump:")
	reader := bufio.NewReader(os.Stdin)
	dumpInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Failed to read input: %v\n", err)
		return
	}
	dumpInput = dumpInput[:len(dumpInput)-1]

	client := openai.NewClient(apiKey)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "I want to store some text inside a .txt file so suggest me a suitable filename. This is the text I want to store: \n" + dumpInput,
				},
			},
		},
	)

	if err != nil {
		fmt.Printf("ChatCompletionError: %v\n", err)
		return
	}

  filename := resp.Choices[0].Message.Content

	fp := filepicker.New()
	fp.ShowPermissions = false
	fp.DirAllowed = true
	fp.CurrentDirectory, _ = os.UserHomeDir()

	m := model{
		filepicker: fp,
	}
	tm, _ := tea.NewProgram(&m).Run()
	mm := tm.(model)
	// X // We have the directory selected now
	fmt.Println("\nDirectory selected: " + m.filepicker.Styles.Selected.Render(mm.selectedFile) + "\n")
	
	// X // create an empty .txt file in the dir
	fileNameWithPath := mm.selectedFile + "\\" + filename

	if mkErr := os.MkdirAll(filepath.Dir(fileNameWithPath), 0770); mkErr != nil {
		log.Fatal("Error occurred while creating file!")
	}

	file, fileErr := os.Create(fileNameWithPath)
	if fileErr != nil {
		log.Fatalf("Failed to create file: %s", fileErr)
	}

	defer file.Close()

	// X // populate the .txt file with the input content
	writeErr := os.WriteFile(fileNameWithPath, []byte(dumpInput), 0644)
	if writeErr != nil {
		log.Fatalf("Failed writing to file: %s", writeErr)
	}

	fmt.Println("New file created: " + m.filepicker.Styles.Selected.Render(mm.selectedFile + "\\" + filename) + "\n")
}
