package assistant

import (
	"fmt"
	"io"

	gassist "google.golang.org/genproto/googleapis/assistant/embedded/v1alpha2"
)

// Conversation holds a Google Assistant conversation
type Conversation struct {
	Assistant    *Assistant //A pointer to the assistant, because we don't like high memory usage with multiple conversations now do we?
	AssistClient gassist.EmbeddedAssistant_AssistClient
	Running      bool
}

// Refresh initializes a new client stream, must be called before every query
func (c *Conversation) Refresh() error {
	if c.Running {
		c.AssistClient.CloseSend()
		c.Running = false
	}
	assistClient, err := c.Assistant.GoogleAssistant.Assist(c.Assistant.Context)
	if err != nil {
		return err
	}
	c.AssistClient = assistClient
	c.Running = true
	return nil
}

// RequestTransportAudio returns an audio query transport, which must be used for the remainder of this conversation
func (c *Conversation) RequestTransportAudio() *TransportAudio {
	return &TransportAudio{
		Conversation: c,
	}
}

// RequestTransportText returns a text query transport, which must be used for the remainder of this conversation
func (c *Conversation) RequestTransportText() *TransportText {
	return &TransportText{
		Conversation: c,
	}
}

// Close closes the conversation
func (c *Conversation) Close() {
	if c.Running {
		c.AssistClient.CloseSend()
		c.Running = false
	}
}

// TransportAudio holds a request for an audio query
type TransportAudio struct {
	SpeechRecognitionResult    string
	SpeechRecognitionStability float32
	Finished                   bool

	Conversation *Conversation
}

// Start starts the process and blocks until a response is returned, and optionally can send true over a channel once the request is closed by the Assistant
func (r *TransportAudio) Start(finishedChan chan bool) error {
	err := r.Conversation.AssistClient.Send(&gassist.AssistRequest{
		Type: &gassist.AssistRequest_Config{
			Config: &gassist.AssistConfig{
				Type: &gassist.AssistConfig_AudioInConfig{
					AudioInConfig: &gassist.AudioInConfig{
						Encoding:        r.Conversation.Assistant.AudioSettings.AudioInEncoding,
						SampleRateHertz: r.Conversation.Assistant.AudioSettings.AudioInSampleRateHertz,
					},
				},
				AudioOutConfig: &gassist.AudioOutConfig{
					Encoding:         r.Conversation.Assistant.AudioSettings.AudioOutEncoding,
					SampleRateHertz:  r.Conversation.Assistant.AudioSettings.AudioOutSampleRateHertz,
					VolumePercentage: r.Conversation.Assistant.AudioSettings.AudioOutVolumePercentage,
				},
				DeviceConfig:  r.Conversation.Assistant.Device.DeviceConfig,
				DialogStateIn: r.Conversation.Assistant.DialogState,
			},
		},
	})
	if err != nil {
		return err
	}

	for {
		if r.Finished {
			if finishedChan != nil {
				finishedChan <- true
			}
			break
		}
	}

	return nil
}

// Read implements io.Reader and reads audio directly from Google, hanging until either audio is returned, an error is returned, or an EOF is returned upon the Assistant finishing the conversation
func (r *TransportAudio) Read(p []byte) (n int, err error) {
	for {
		response, err := r.Conversation.AssistClient.Recv()
		if err != nil {
			//TODO: Send io.EOF if conversation ends instead of the read stream ending
			r.Finished = true
			return 0, err
		}

		if response == nil {
			continue
		}

		if dialogStateOut := response.GetDialogStateOut(); dialogStateOut != nil {
			r.Conversation.Assistant.DialogState.ConversationState = dialogStateOut.ConversationState
			r.Conversation.Assistant.AudioSettings.AudioOutVolumePercentage = dialogStateOut.VolumePercentage
		}

		if r.SpeechRecognitionStability != 1.0 {
			if requestTexts := response.GetSpeechResults(); len(requestTexts) > 0 {
				if len(requestTexts) == 1 {
					r.SpeechRecognitionResult = requestTexts[0].Transcript
					r.SpeechRecognitionStability = requestTexts[0].Stability
				} else {
					transcript := ""
					for i := 0; i < len(requestTexts); i++ {
						if transcript == "" {
							transcript = requestTexts[i].Transcript
						} else {
							transcript += " " + requestTexts[i].Transcript

							if i == len(requestTexts)-1 {
								r.SpeechRecognitionStability = requestTexts[i].Stability
							}
						}
					}
					r.SpeechRecognitionResult = transcript
				}
			}
		}

		if audioOut := response.GetAudioOut(); audioOut != nil {
			return copy(p, audioOut.AudioData), nil
		}
	}

	return 0, nil
}

// Write implements io.Writer and sends audio directly to Google, must be buffered at an unknown-required rate (32KB/s seems too slow but works in testing)
func (r *TransportAudio) Write(p []byte) (n int, err error) {
	r.Conversation.Refresh() //Initialize a new stream
	err = r.Conversation.AssistClient.Send(&gassist.AssistRequest{
		Type: &gassist.AssistRequest_AudioIn{
			AudioIn: p,
		},
	})
	return len(p), err
}

// Transcript returns a transcript of words that the user has spoken so far, as well as an estimate of the likelihood that the Assistant will not change its guess about this result (0.0 = unset, 0.1 = unstable, 1.0 = stable and final)
func (r *TransportAudio) Transcript() (transcript string, stability float32) {
	return r.SpeechRecognitionResult, r.SpeechRecognitionStability
}

// TransportText holds a request for a text query
type TransportText struct {
	TextQuery    string
	TextResponse string

	Conversation *Conversation
}

// Query returns the Assistant's response to a query as text
func (r *TransportText) Query(textQuery string) (string, error) {
	r.Conversation.Refresh() //Initialize a new stream
	r.TextQuery = textQuery
	if err := r.send(textQuery); err != nil {
		return "", err
	}
	if err := r.recv(textQuery); err != nil {
		return "", err
	}
	return r.TextResponse, nil
}

func (r *TransportText) send(textQuery string) error {
	err := r.Conversation.AssistClient.Send(&gassist.AssistRequest{
		Type: &gassist.AssistRequest_Config{
			Config: &gassist.AssistConfig{
				Type: &gassist.AssistConfig_TextQuery{
					TextQuery: textQuery,
				},
				AudioOutConfig: &gassist.AudioOutConfig{
					Encoding:         r.Conversation.Assistant.AudioSettings.AudioOutEncoding,
					SampleRateHertz:  r.Conversation.Assistant.AudioSettings.AudioOutSampleRateHertz,
					VolumePercentage: r.Conversation.Assistant.AudioSettings.AudioOutVolumePercentage,
				},
				DeviceConfig:  r.Conversation.Assistant.Device.DeviceConfig,
				DialogStateIn: r.Conversation.Assistant.DialogState,
			},
		},
	})
	if err != nil {
		err = fmt.Errorf("error sending request: %v", err)
	}
	return err
}

func (r *TransportText) recv(textQuery string) error {
	for {
		response, err := r.Conversation.AssistClient.Recv()
		if err != nil {
			if err == io.EOF {
				if err := r.send(textQuery); err != nil {
					return fmt.Errorf("error re-sending request after EOF: %v", err)
				}
				response, err = r.Conversation.AssistClient.Recv()
				if err != nil {
					return fmt.Errorf("error getting response after re-sending request from EOF: %v", err)
				}
			} else {
				return fmt.Errorf("error getting response: %v", err)
			}
		}

		if response == nil {
			return fmt.Errorf("nil response")
		}

		if dialogStateOut := response.GetDialogStateOut(); dialogStateOut != nil {
			r.Conversation.Assistant.DialogState.ConversationState = dialogStateOut.ConversationState
			r.Conversation.Assistant.DialogState.IsNewConversation = false
			r.Conversation.Assistant.AudioSettings.AudioOutVolumePercentage = dialogStateOut.VolumePercentage
			r.TextResponse = dialogStateOut.GetSupplementalDisplayText()
			break
		}
	}
	return nil
}
