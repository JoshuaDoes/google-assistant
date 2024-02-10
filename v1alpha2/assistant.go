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
func NewAssistant(token *Token, oauthToken *oauth2.Token, callbackFunc TokenCallback, internalHost, languageCode string, device *Device, audioSettings *AudioSettings) (Assistant, error) {
	assistant := Assistant{
		AudioSettings: audioSettings,
		Device:        device,
		GCPAuth:       &GCPAuthWrapper{OauthToken: oauthToken},
		LanguageCode:  languageCode,
		DialogState: &gassist.DialogStateIn{
			ConversationState: make([]byte, 0),
			LanguageCode:      languageCode,
			IsNewConversation: true,
		},
	}
	if oauthToken.Valid() {
		return assistant, nil
	}
	return assistant, assistant.GCPAuth.Initialize(token, internalHost, callbackFunc)
}

// GetAuthURL returns the Google authentication URL to sign into your Google account, only if you actually need to
// Also acts as a token refresh mechanism when running into auth issues
func (a *Assistant) GetAuthURL() string {
	if a.GCPAuth == nil {
		return "ERROR" //Must initialize with NewAssistant!
	}
	if a.GCPAuth.OauthToken == nil || !a.GCPAuth.OauthToken.Valid() {
		return a.GCPAuth.AuthURL
	}
	return ""
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

	if int64(timeout) > 0 {
		a.Context, a.Canceler = context.WithDeadline(context.Background(), time.Now().Add(timeout))
	} else {
		a.Context = context.Background()
	}

	a.Connection, err = a.newConnection(a.Context)
	if err != nil {
		return nil, err
	}

	a.GoogleAssistant = gassist.NewEmbeddedAssistantClient(a.Connection)

	conversation := &Conversation{
		Assistant: a,
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
