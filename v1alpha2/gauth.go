package assistant

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

//Token holds a Google OAuth2 token
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

//TokenCallback holds a callback function to return an OAuth2 token to, usually to cache for relogging in
type TokenCallback func(*oauth2.Token)

//GCPAuthWrapper handles Google authentication
type GCPAuthWrapper struct {
	AuthURL        string
	CallbackFunc   TokenCallback
	Config         *oauth2.Config
	OauthSrv       *http.Server
	OauthToken     *oauth2.Token
	PermissionCode string

	AuthError error
}

//Error returns an authentication error
func (w *GCPAuthWrapper) Error() error {
	return w.AuthError
}

//Initialize initializes authentication to allow a user to sign in to the application
func (w *GCPAuthWrapper) Initialize(credentials *Token, internalHost string, callbackFunc TokenCallback) error {
	w.AuthURL = credentials.Installed.RedirectUris[0]

	if w.PermissionCode != "" {
		err := w.SetTokenSource(w.PermissionCode)
		return err
	}

	w.Config = &oauth2.Config{
		ClientID:     credentials.Installed.ClientID,
		ClientSecret: credentials.Installed.ClientSecret,
		Scopes: []string{
			"https://www.googleapis.com/auth/assistant-sdk-prototype",
		},
		RedirectURL: w.AuthURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://accounts.google.com/o/oauth2/token",
		},
	}

	w.AuthURL = w.Config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	if callbackFunc != nil {
		w.CallbackFunc = callbackFunc
	}

	w.OauthSrv = &http.Server{
		Addr: internalHost,
		Handler: http.DefaultServeMux,
	}

	http.HandleFunc("/", w.oauthHandler)
	go func() {
		if err := w.OauthSrv.ListenAndServe(); err != nil {
			w.AuthError = err
		}
	}()
	time.Sleep(time.Second * 1) //Give it about 1 second to time out

	return w.AuthError
}

func (w *GCPAuthWrapper) oauthHandler(writer http.ResponseWriter, req *http.Request) {
	w.PermissionCode = req.URL.Query().Get("code")
	if w.PermissionCode != "" {
		if err := w.SetTokenSource(w.PermissionCode); err != nil {
			writer.Write([]byte(fmt.Sprintf("<html><body><style>{background-color:black;color:white;}</style><h3>Authentication Failure</h3><p>The following error was provided: <strong>%v</strong>.</p><footer>You should try logging in again.</footer></body></html>")))
		} else {
			writer.Write([]byte(fmt.Sprintf("<html><body><style>body{background-color:black;color:white;}</style><h3>Authentication Successful</h3><p>Your token is <strong>%s</strong>.</p><footer>You may safely close this page.</footer></body></html>", w.PermissionCode)))
		}
	} else {
		writer.Write([]byte(fmt.Sprintf("<html><body><style>body{background-color:black;color:white;}</style><h3>Authentication Failure</h3><p>No token received!</p><footer>You should try logging in again.</footer></body></html>")))
	}
}

//SetTokenSource is used to finish the authentication process, as well as calling any provided callback function
func (w *GCPAuthWrapper) SetTokenSource(permissionCode string) error {
	var err error

	ctx := context.Background()

	w.OauthToken, err = w.Config.Exchange(ctx, permissionCode)
	if err != nil {
		return err
	}

	if w.CallbackFunc != nil {
		go w.CallbackFunc(w.OauthToken)
	}

	return nil
}
