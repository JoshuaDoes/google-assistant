package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	gassist "github.com/JoshuaDoes/google-assistant/v1alpha2"
	"golang.org/x/oauth2"
)

var (
	blob  *oauth2.Token
	creds *gassist.Token
)

func cacheToken(token *oauth2.Token) {
	blob = token
	blobJSON, err := json.Marshal(blob)
	if err != nil {
		fmt.Println("! Failed to marshal token!")
		return
	}
	if err := ioutil.WriteFile("token.blob", blobJSON, 0600); err != nil {
		fmt.Println("! Failed to cache token!")
	}
}

func main() {
	var assistant gassist.Assistant
	var err error

	if len(os.Args) < 2 {
		fmt.Println("! Usage: " + strings.Join(os.Args, " ") + " client_secret_XXXXXXXXXXXX-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX.apps.googleusercontent.com.json")
		os.Exit(1)
	}

	fmt.Println("> Loading credentials...")
	credsJSON, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(">", err)
	} else {
		fmt.Println("> Parsing credentials...")
		creds = &gassist.Token{}
		if err := json.Unmarshal(credsJSON, &creds); err != nil {
			fmt.Println(">", err)
			os.Exit(1)
		}
	}


	fmt.Println("> Loading cached token...")
	tokenJSON, err := ioutil.ReadFile("token.blob")
	if err != nil {
		fmt.Println(">", err)
	} else {
		fmt.Println("> Parsing cached token...")
		blob = &oauth2.Token{}
		if err := json.Unmarshal(tokenJSON, &blob); err != nil {
			fmt.Println(">", err)
			os.Exit(1)
		}
	}

	fmt.Println("> Initializing assistant...")
	assistant, err = gassist.NewAssistant(creds, blob, cacheToken, ":25480", "en-US", gassist.NewDevice("254636TEST0001", "assistant-for-clinet"), gassist.NewAudioSettings(1, 1, 16000, 16000, 100))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if assistant.GetAuthURL() != "" {
		fmt.Println(">")
		fmt.Println("! Please log into Google:", assistant.GetAuthURL())
		for blob == nil {
			time.Sleep(time.Millisecond * 100)
		}
		fmt.Println(">")
	}

	fmt.Println("> Starting a new conversation...")
	conversation, err := assistant.NewConversation(0)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conversation.Close()

	fmt.Println("! At any moment, type 'exit' and press enter to exit!")
	fmt.Println("")

	reqTxt := conversation.RequestTransportText()
	for {
		fmt.Print("@google ")
		textIn := readLine()
		if textIn == "exit" {
			break
		}

		resTxt, err := reqTxt.Query(textIn + "\n")
		if err != nil {
			fmt.Println("!", err)
		}
		if resTxt != "" {
			fmt.Println(">", resTxt)
		}
		fmt.Println("")
	}

	fmt.Println("Good-bye!")
}

func pressEnter() {
	bufio.NewReader(os.Stdin).ReadLine()
}

func readLine() string {
	rdr := bufio.NewReader(os.Stdin)
	line, err := rdr.ReadString('\n')
	switch err {
	case io.EOF:
		os.Exit(0)
	default:
		if err != nil {
			panic(err)
		}
	}
	return line[:len(line)-1] //Strip the newline
}
