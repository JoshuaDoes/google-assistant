package assistant

import (
	"context"
	"fmt"
	"net/http"

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

//PermissionCallback holds a callback function to return an authentication token to
type PermissionCallback func(string) error

//GCPAuthWrapper handles Google authentication
type GCPAuthWrapper struct {
	AuthURL        string
	CallbackFunc   PermissionCallback
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
func (w *GCPAuthWrapper) Initialize(credentials *Token, oauthRedirectURL string, callbackFunc PermissionCallback) error {
	if w.PermissionCode != "" {
		err := w.SetTokenSource(w.PermissionCode)
		return err
	}

	oauthRedirectURL = "http://localhost:8080"

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
	permissionCode := req.URL.Query().Get("code")

	if permissionCode != "" {
		w.PermissionCode = permissionCode

		writer.Write([]byte(fmt.Sprintf("<html><body><h3>Authentication Successful</h3><p>Your token is <strong>%s</strong>.</p><footer>You may safely close this page.</footer></body></html>", w.PermissionCode)))

		if w.CallbackFunc != nil {
			w.CallbackFunc(w.PermissionCode)
		}
	} else {
		writer.Write([]byte(fmt.Sprintf("<html><body><h3>Authentication Failure</h3><p>No token received!</p><footer>You should try logging in again.</footer></body></html>")))
	}
}

//SetTokenSource is the default permission callback function, which is used to finish the authentication process
func (w *GCPAuthWrapper) SetTokenSource(permissionCode string) error {
	var err error

	ctx := context.Background()

	w.OauthToken, err = w.Config.Exchange(ctx, permissionCode)
	if err != nil {
		return err
	}

	return nil
}
