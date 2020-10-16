package assistant

import (
	"context"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
	gassist "google.golang.org/genproto/googleapis/assistant/embedded/v1alpha2"
	"google.golang.org/grpc"
)

// Assistant holds the Google Assistant and methods to interact with it
type Assistant struct {
	//The real deal
	GoogleAssistant gassist.EmbeddedAssistantClient
	Conversation    gassist.EmbeddedAssistant_AssistClient
	DialogState     *gassist.DialogStateIn

	//Assistant configuration
	AudioSettings *AudioSettings
	Device        *Device
	GCPAuth       *GCPAuthWrapper //Google Cloud Platform authentication wrapper
	LanguageCode  string
	//AssistConfig  *gassist.AssistConfig

	//Connection stuff
	Canceler   context.CancelFunc
	Connection *grpc.ClientConn
	Context    context.Context
}

// NewAssistant returns a new Google Assistant to operate on
func NewAssistant(token *Token, oauthToken *oauth2.Token, languageCode string, device *Device, audioSettings *AudioSettings) (Assistant, error) {
	gcpAuthWrapper := &GCPAuthWrapper{OauthToken: oauthToken}

	assistant := Assistant{
		AudioSettings: audioSettings,
		Device:        device,
		GCPAuth:       gcpAuthWrapper,
		LanguageCode:  languageCode,
		DialogState: &gassist.DialogStateIn{
			ConversationState: make([]byte, 0),
			LanguageCode:      languageCode,
			IsNewConversation: true,
		},
	}

	if oauthToken.Valid() == false {
		return assistant, gcpAuthWrapper.Initialize(token, "", nil)
	}

	return assistant, nil
}

// GetAuthURL returns the Google authentication URL to sign into your Google account
func (a *Assistant) GetAuthURL() string {
	if a.GCPAuth == nil {
		return ""
	}
	return a.GCPAuth.AuthURL
}

func (a *Assistant) newConnection(ctx context.Context) (conn *grpc.ClientConn, err error) {
	tokenSource := a.GCPAuth.Config.TokenSource(ctx, a.GCPAuth.OauthToken)
	return transport.DialGRPC(ctx,
		option.WithTokenSource(tokenSource),
		option.WithEndpoint("embeddedassistant.googleapis.com:443"),
		option.WithScopes("https://www.googleapis.com/auth/assistant-sdk-prototype"),
	)
}

// NewConversation starts a new conversation and returns it, it's the caller's job to close it
func (a *Assistant) NewConversation(timeout time.Duration) (*Conversation, error) {
	var err error
	err = a.GCPAuth.Error()
	if err != nil {
		return nil, err
	}

	a.Context, a.Canceler = context.WithDeadline(context.Background(), time.Now().Add(timeout))

	a.Connection, err = a.newConnection(a.Context)
	if err != nil {
		return nil, err
	}

	a.GoogleAssistant = gassist.NewEmbeddedAssistantClient(a.Connection)

	assistClient, err := a.GoogleAssistant.Assist(a.Context)
	if err != nil {
		return nil, err
	}

	conversation := &Conversation{
		Assistant:    a,
		AssistClient: assistClient,
	}

	return conversation, nil
}

// Close closes the connection to the Assistant and cleans up all resources, except for conversations which must be handled by the caller
func (a *Assistant) Close() {
	if a.Context != nil {
		a.Context.Done()
	}
	if a.Canceler != nil {
		a.Canceler()
	}
	if a.Connection != nil {
		a.Connection.Close()
	}
	if a.GCPAuth != nil {
		if a.GCPAuth.OauthSrv != nil {
			a.GCPAuth.OauthSrv.Shutdown(context.Background())
		}
	}
}

// Conversation holds a Google Assistant conversation
type Conversation struct {
	Assistant    *Assistant //A pointer to the assistant, because we don't like high memory usage with multiple conversations now do we?
	AssistClient gassist.EmbeddedAssistant_AssistClient
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
	c.AssistClient.CloseSend()
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
	r.TextQuery = textQuery

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
		return "", err
	}

	for {
		response, err := r.Conversation.AssistClient.Recv()
		if err != nil {
			return "", err
		}

		if response == nil {
			continue
		}

		if dialogStateOut := response.GetDialogStateOut(); dialogStateOut != nil {
			r.Conversation.Assistant.DialogState.ConversationState = dialogStateOut.ConversationState
			r.Conversation.Assistant.AudioSettings.AudioOutVolumePercentage = dialogStateOut.VolumePercentage
			return dialogStateOut.GetSupplementalDisplayText(), nil
		}
	}

	return "", nil
}
