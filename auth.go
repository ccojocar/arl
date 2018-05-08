package main

import (
	"fmt"
	"net/http"

	"errors"
	"github.com/ccojocar/adal"
	"sync"
)

const authority = "https://login.microsoftonline.com/"

//TokenSource interface which should be implemented by an access token provider
type TokenSource interface {
	Token() (string, error)
	Refresh() (string, error)
}

// AzureTokenSource is the Azure access token provider
type AzureTokenSource struct {
	lock        sync.Mutex
	oauthConfig adal.OAuthConfig
	clientID    string
	resource    string
	spt         *adal.ServicePrincipalToken
}

// NewAzureTokenSource create a new Azure token source
func NewAzureTokenSource(tenantID string, clientID string, resource string) (*AzureTokenSource, error) {
	oauthConfig, err := adal.NewOAuthConfig(authority, tenantID)
	if err != nil {
		return nil, err
	}
	return &AzureTokenSource{
		oauthConfig: *oauthConfig,
		clientID:    clientID,
		resource:    resource,
		spt:         nil,
	}, nil
}

// Token returns a new access token
func (ts *AzureTokenSource) Token() (string, error) {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	var err error
	ts.spt, err = ts.acquireTokenDeviceCodeFlow()
	return ts.spt.AccessToken, err
}

// Refresh refreshes an existing and returns its new value
func (ts *AzureTokenSource) Refresh() (string, error) {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	if ts.spt == nil {
		return "", errors.New("service principal token is nil. call Token() before Refresh()")
	}
	err := ts.spt.Refresh()
	return ts.spt.AccessToken, err
}

func (ts *AzureTokenSource) acquireTokenDeviceCodeFlow() (*adal.ServicePrincipalToken, error) {
	callback := func(token adal.Token) error {
		return nil
	}
	oauthClient := &http.Client{}
	deviceCode, err := adal.InitiateDeviceAuth(
		oauthClient,
		ts.oauthConfig,
		ts.clientID,
		ts.resource)
	if err != nil {
		return nil, fmt.Errorf("Failed to start device auth flow: %s", err)
	}

	fmt.Println(*deviceCode.Message)

	token, err := adal.WaitForUserCompletion(oauthClient, deviceCode)
	if err != nil {
		return nil, fmt.Errorf("Failed to finish device auth flow: %s", err)
	}

	spt, err := adal.NewServicePrincipalTokenFromManualToken(
		ts.oauthConfig,
		ts.clientID,
		resource,
		*token,
		callback)
	return spt, err
}
