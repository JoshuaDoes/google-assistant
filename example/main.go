package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strconv"
	"time"

	gassist "github.com/JoshuaDoes/google-assistant"
)

func main() {
	var assistant gassist.Assistant
	var err error

	fmt.Println("Loading credentials...")
	credentials, err := gassist.GetCredentialsFromFile("client_secret_XXXXXXXXXXXX-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX.apps.googleusercontent.com.json")
	if err != nil {
		panic(err)
	}

	fmt.Println("Initializing Google Assistant...")
	err = assistant.Initialize(credentials, nil)
	if err != nil {
		panic(err)
	}

	fmt.Println("Please open the following URL to authenticate: ", assistant.GCPAuth.AuthURL)
	fmt.Println("When you've authenticated successfully, press enter to continue.")
	pressEnter()

	fmt.Println("Starting assistant...")
	err = assistant.Start()
	if err != nil {
		panic(err)
	}
	defer assistant.Close()

	fmt.Println("The Google Assistant is ready!")
	fmt.Println("Press enter to send 'input.wav' and listen for responses.")
	pressEnter()

	//	/*
	fmt.Println("Reading 'input.wav'...")
	audioIn, err := ioutil.ReadFile("input.wav")
	if err != nil {
		panic(err)
	}

	go func() {
		fmt.Println("Inputting audio...")

		loop := int(math.Ceil(float64(len(audioIn)) / float64(8192)))
		for i := 1; i < (loop + 1); i++ {
			fmt.Println("Sending pass " + strconv.Itoa(i) + "/" + strconv.Itoa(loop))

			low := (i - 1) * 8192
			high := i * 8192
			if high > len(audioIn) {
				high = len(audioIn)
			}

			err = assistant.AudioIn(audioIn[low:high])
			if err != nil {
				panic(err)
			}

			time.Sleep(250 * time.Millisecond)
		}

		fmt.Println("Audio sent successfully!")
	}()
	go func() {
		audioOutFile, err := os.OpenFile("output.pcm16.16000Hz.wav", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}

		wroteAudio := false
		gotReqTxt := false
		for {
			response, err := assistant.RequestResponse()
			if err == io.EOF {
				wroteAudio = true
			} else if err != nil {
				panic(err)
			}
			result := response.GetResult()

			if response == nil {
				continue
			}
			if audioOut := response.GetAudioOut(); audioOut != nil {
				if wroteAudio == false {
					fmt.Println("====== Writing audio... ======")
					audioOutFile.Write(audioOut.GetAudioData())
				}
			}
			if requestText := result.GetSpokenRequestText(); requestText != "" {
				fmt.Println("====== Request Text ======")
				fmt.Println(requestText)
				gotReqTxt = true
			}
			/*			if responseText := result.GetSpokenResponseText(); responseText != "" {
						fmt.Println("====== Response Text ======")
						fmt.Println(responseText)
					} */

			if wroteAudio && gotReqTxt {
				break
			}
		}

		fmt.Println("Closing input stream...")
		err = assistant.Conversation.CloseSend()
		if err != nil {
			panic(err)
		}

		fmt.Printf("\nPlease press enter to exit.\n")
	}()

	pressEnter()
}

func pressEnter() {
	bufio.NewReader(os.Stdin).ReadLine()
}
