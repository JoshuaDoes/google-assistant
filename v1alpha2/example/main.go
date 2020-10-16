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
)

func main() {
	var assistant gassist.Assistant
	var err error

	if len(os.Args) < 2 {
		panic("Usage: " + strings.Join(os.Args, " ") + " client_secret_XXXXXXXXXXXX-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX.apps.googleusercontent.com.json")
	}

	fmt.Println("Loading token...")
	tokenJSON, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	fmt.Println("Parsing token...")
	token := &gassist.Token{}
	err = json.Unmarshal(tokenJSON, &token)
	if err != nil {
		panic(err)
	}

	fmt.Println("Initializing Google Assistant...")
	assistant, err = gassist.NewAssistant(token, nil, "en-US", gassist.NewDevice("254636TEST0001", "assistant-for-clinet"), gassist.NewAudioSettings(1, 1, 16000, 16000, 100))
	if err != nil {
		panic(err)
	}

	if assistant.GetAuthURL() != "" {
		fmt.Println("Please open the following URL to authenticate:", assistant.GetAuthURL())
		fmt.Println("When you've authenticated successfully, press enter to continue.")
		pressEnter()
	}

	for {
		fmt.Println("Starting a new conversation...")
		conversation, err := assistant.NewConversation(time.Second * 240)
		if err != nil {
			panic(err)
		}
		defer conversation.Close()

		fmt.Print("Query: ")
		textIn := readLine()
		reqTxt := conversation.RequestTransportText()
		resTxt, err := reqTxt.Query(textIn)
		if err != nil {
			fmt.Println(err)
			pressEnter()
		}

		fmt.Println("Response:", resTxt)
		fmt.Println("Press enter to run another query!")
		pressEnter()
	}
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
	return line
}
