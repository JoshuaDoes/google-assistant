package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	externalip "github.com/GlenDC/go-external-ip"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
	embedded "google.golang.org/genproto/googleapis/assistant/embedded/v1alpha1"
	"google.golang.org/grpc"
)

type Token struct {
	Installed struct {
		ClientID                string   `json:"client_id"`
		ProjectID               string   `json:"project_id"`
		AuthURI                 string   `json:"auth_uri"`
		TokenURI                string   `json:"token_uri"`
		AuthProviderX509CertURL string   `json:"auth_provider_x509_cert_url"`
		ClientSecret            string   `json:"client_secret"`
		RedirectUris            []string `json:"redirect_uris"`
	} `json:"installed"`
}

func GetCredentialsFromFile(fileCredentials string) (*Token, error) {
	file, err := os.Open(fileCredentials)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var token *Token
	if err = json.NewDecoder(file).Decode(&token); err != nil {
		return nil, err
	}

	return token, nil
}

type Assistant struct {
	GoogleAssistant embedded.EmbeddedAssistantClient
	Config          *embedded.ConverseRequest_Config
	AudioBuffer     *AudioBuffer

	Canceler                  context.CancelFunc
	Connection                *grpc.ClientConn
	Context                   context.Context
	Conversation              embedded.EmbeddedAssistant_ConverseClient
	ConversationConfiguration *embedded.ConverseConfig
	ConversationState         []byte
	GCPAuth                   *GCPAuthWrapper
}

type AudioBuffer struct {
	AudioInEncoding        embedded.AudioInConfig_Encoding
	AudioInSampleRateHertz int32

	AudioOutEncoding         embedded.AudioOutConfig_Encoding
	AudioOutSampleRateHertz  int32
	AudioOutVolumePercentage int32
}

func NewAssistant() *Assistant {
	return &Assistant{}
}

func (a *Assistant) Initialize(credentials *Token, oauthToken *oauth2.Token) error {
	return a.InitializeRaw(nil, nil, credentials, oauthToken, "", nil)
}

func (a *Assistant) InitializeRaw(assistConfig *embedded.ConverseRequest_Config, audioBuffer *AudioBuffer, credentials *Token, oauthToken *oauth2.Token, oauthRedirectURL string, callbackFunc func(permissionCode string) error) error {
	if audioBuffer == nil {
		a.AudioBuffer = &AudioBuffer{
			AudioInEncoding:          embedded.AudioInConfig_LINEAR16,
			AudioInSampleRateHertz:   16000,
			AudioOutEncoding:         embedded.AudioOutConfig_LINEAR16,
			AudioOutSampleRateHertz:  16000,
			AudioOutVolumePercentage: 100,
		}
	} else {
		a.AudioBuffer = audioBuffer
	}

	if assistConfig == nil {
		a.Config = &embedded.ConverseRequest_Config{
			Config: &embedded.ConverseConfig{
				AudioInConfig: &embedded.AudioInConfig{
					Encoding:        a.AudioBuffer.AudioInEncoding,
					SampleRateHertz: a.AudioBuffer.AudioInSampleRateHertz,
				},
				AudioOutConfig: &embedded.AudioOutConfig{
					Encoding:         a.AudioBuffer.AudioOutEncoding,
					SampleRateHertz:  a.AudioBuffer.AudioOutSampleRateHertz,
					VolumePercentage: a.AudioBuffer.AudioOutVolumePercentage,
				},
			},
		}
	}

	a.GCPAuth = &GCPAuthWrapper{OauthToken: oauthToken}
	if a.GCPAuth.OauthToken.Valid() == false {
		err := a.GCPAuth.Initialize(credentials, oauthRedirectURL, callbackFunc)
		return err
	}
	return nil
}

func (a *Assistant) Start() error {
	var err error

	if a.GCPAuth.Error() != nil {
		return a.GCPAuth.Error()
	}

	runDuration := 240 * time.Second
	a.Context, a.Canceler = context.WithDeadline(context.Background(), time.Now().Add(runDuration))

	a.Connection, err = a.newConnection(a.Context)
	if err != nil {
		return err
	}

	a.GoogleAssistant = embedded.NewEmbeddedAssistantClient(a.Connection)

	if len(a.ConversationState) > 0 {
		a.Config.Config.ConverseState = &embedded.ConverseState{ConversationState: a.ConversationState}
	}

	a.Conversation, err = a.GoogleAssistant.Converse(a.Context)
	if err != nil {
		return err
	}

	err = a.Conversation.Send(&embedded.ConverseRequest{
		ConverseRequest: a.Config,
	})
	if err != nil {
		return err
	}

	return nil
}

func (a *Assistant) newConnection(ctx context.Context) (conn *grpc.ClientConn, err error) {
	tokenSource := a.GCPAuth.Config.TokenSource(ctx, a.GCPAuth.OauthToken)
	return transport.DialGRPC(ctx,
		option.WithTokenSource(tokenSource),
		option.WithEndpoint("embeddedassistant.googleapis.com:443"),
		option.WithScopes("https://www.googleapis.com/auth/assistant-sdk-prototype"),
	)
}

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

//AudioIn sends input audio data to be handled when ready
func (a *Assistant) AudioIn(audioIn []byte) error {
	err := a.Conversation.Send(&embedded.ConverseRequest{
		ConverseRequest: &embedded.ConverseRequest_AudioIn{
			AudioIn: audioIn,
		},
	})
	return err
}

//RequestResponse requests a response based on the available input audio data
func (a *Assistant) RequestResponse() (*embedded.ConverseResponse, error) {
	resp, err := a.Conversation.Recv()
	if err == io.EOF {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if err := resp.GetError(); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	result := resp.GetResult()
	if result != nil && result.ConversationState != nil {
		a.ConversationState = result.ConversationState
	}

	return resp, nil
}

type PermissionCallback func(string) error

type GCPAuthWrapper struct {
	AuthURL        string
	CallbackFunc   PermissionCallback
	Config         *oauth2.Config
	OauthSrv       *http.Server
	OauthToken     *oauth2.Token
	PermissionCode string

	AuthError error
}

func (w *GCPAuthWrapper) Error() error {
	return w.AuthError
}

func (w *GCPAuthWrapper) Initialize(credentials *Token, oauthRedirectURL string, callbackFunc PermissionCallback) error {
	if w.PermissionCode != "" {
		err := w.SetTokenSource(w.PermissionCode)
		return err
	}

	if oauthRedirectURL == "" {
		consensus := externalip.DefaultConsensus(nil, nil)
		ip, err := consensus.ExternalIP()
		if err != nil {
			return err
		}

		oauthRedirectURL = "http://" + ip.String() + ":8080"
		oauthRedirectURL = "http://localhost:8080"
	}

	w.Config = &oauth2.Config{
		ClientID:     credentials.Installed.ClientID,
		ClientSecret: credentials.Installed.ClientSecret,
		Scopes: []string{
			"https://www.googleapis.com/auth/assistant-sdk-prototype",
		},
		RedirectURL: oauthRedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://accounts.google.com/o/oauth2/token",
		},
	}

	w.AuthURL = w.Config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	if callbackFunc != nil {
		w.CallbackFunc = callbackFunc
	} else {
		w.CallbackFunc = w.SetTokenSource
	}

	go w.startOauthHandler()

	return nil
}

func (w *GCPAuthWrapper) startOauthHandler() {
	w.OauthSrv = &http.Server{
		Addr:    ":8080",
		Handler: http.DefaultServeMux,
	}

	http.HandleFunc("/", w.oauthHandler)

	err := w.OauthSrv.ListenAndServe()
	if err != http.ErrServerClosed {
		w.AuthError = err
	}
}

func (w *GCPAuthWrapper) oauthHandler(writer http.ResponseWriter, req *http.Request) {
	w.PermissionCode = req.URL.Query().Get("code")
	writer.Write([]byte(fmt.Sprintf("<html><body><h3>Authentication Successful</h3><p>Your token is <strong>%s</strong>.</p><footer>You may safely close this page.</footer></body></html>", w.PermissionCode)))
	if w.CallbackFunc != nil {
		w.CallbackFunc(w.PermissionCode)
	}
}

func (w *GCPAuthWrapper) SetTokenSource(permissionCode string) error {
	var err error

	ctx := context.Background()

	w.OauthToken, err = w.Config.Exchange(ctx, permissionCode)
	if err != nil {
		return err
	}

	return nil
}
