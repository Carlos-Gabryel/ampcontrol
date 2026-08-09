package amp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type APIClient struct {
	username   string
	password   string
	httpClient *http.Client
}

type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Token      string `json:"token"`
	RememberMe bool   `json:"rememberMe"`
}

type loginResponse struct {
	Result       int      `json:"result"`
	ResultReason string   `json:"resultReason"`
	Success      bool     `json:"success"`
	Permissions  []string `json:"permissions"`
	SessionID    string   `json:"sessionID"`
}

type authenticatedRequest struct {
	SessionID string `json:"SESSIONID"`
}

type actionResponse struct {
	Status       bool    `json:"Status"`
	Reason       *string `json:"Reason"`
	SupportURL   *string `json:"SupportURL"`
	SupportTitle *string `json:"SupportTitle"`
}

func NewAPIClient(
	username string,
	password string,
) *APIClient {
	return &APIClient{
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// StartApplication autentica na instância AMP e inicia
// ou acorda a aplicação.
func (c *APIClient) StartApplication(
	ctx context.Context,
	baseURL string,
) error {
	sessionID, err := c.login(
		ctx,
		baseURL,
	)
	if err != nil {
		return err
	}

	requestBody := authenticatedRequest{
		SessionID: sessionID,
	}

	var response actionResponse

	err = c.postJSON(
		ctx,
		buildAPIURL(
			baseURL,
			"Core",
			"Start",
		),
		sessionID,
		requestBody,
		&response,
	)
	if err != nil {
		return fmt.Errorf(
			"falha chamando Core.Start: %w",
			err,
		)
	}

	if !response.Status {
		reason := "o AMP não informou o motivo"

		if response.Reason != nil &&
			strings.TrimSpace(*response.Reason) != "" {
			reason = strings.TrimSpace(
				*response.Reason,
			)
		}

		return fmt.Errorf(
			"o AMP recusou Core.Start: %s",
			reason,
		)
	}

	return nil
}

// StopApplication autentica na instância AMP e encerra
// somente o processo do jogo.
//
// A instância AMP continua ligada, deixando o servidor
// no estado Idle usado pelo AmpControl.
func (c *APIClient) StopApplication(
	ctx context.Context,
	baseURL string,
) error {
	sessionID, err := c.login(
		ctx,
		baseURL,
	)
	if err != nil {
		return err
	}

	requestBody := authenticatedRequest{
		SessionID: sessionID,
	}

	err = c.postJSON(
		ctx,
		buildAPIURL(
			baseURL,
			"Core",
			"Stop",
		),
		sessionID,
		requestBody,
		nil,
	)
	if err != nil {
		return fmt.Errorf(
			"falha chamando Core.Stop: %w",
			err,
		)
	}

	return nil
}

func (c *APIClient) login(
	ctx context.Context,
	baseURL string,
) (string, error) {
	requestBody := loginRequest{
		Username:   c.username,
		Password:   c.password,
		Token:      "",
		RememberMe: false,
	}

	var response loginResponse

	err := c.postJSON(
		ctx,
		buildAPIURL(
			baseURL,
			"Core",
			"Login",
		),
		"",
		requestBody,
		&response,
	)
	if err != nil {
		return "", fmt.Errorf(
			"falha autenticando na API AMP: %w",
			err,
		)
	}

	if !response.Success {
		reason := strings.TrimSpace(
			response.ResultReason,
		)

		if reason == "" {
			reason = "o AMP não informou o motivo"
		}

		return "", fmt.Errorf(
			"login AMP recusado: %s",
			reason,
		)
	}

	if strings.TrimSpace(response.SessionID) == "" {
		return "", fmt.Errorf(
			"o login AMP não retornou uma sessão",
		)
	}

	return response.SessionID, nil
}

func (c *APIClient) postJSON(
	ctx context.Context,
	url string,
	sessionID string,
	requestBody any,
	responseBody any,
) error {
	payload, err := json.Marshal(
		requestBody,
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível gerar o JSON da requisição: %w",
			err,
		)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível criar a requisição HTTP: %w",
			err,
		)
	}

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	request.Header.Set(
		"Accept",
		"application/json",
	)

	if sessionID != "" {
		request.Header.Set(
			"Authorization",
			"Bearer "+sessionID,
		)
	}

	response, err := c.httpClient.Do(
		request,
	)
	if err != nil {
		return fmt.Errorf(
			"erro de comunicação com o AMP: %w",
			err,
		)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(
		io.LimitReader(
			response.Body,
			1024*1024,
		),
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível ler a resposta do AMP: %w",
			err,
		)
	}

	if response.StatusCode < 200 ||
		response.StatusCode >= 300 {
		return fmt.Errorf(
			"o AMP respondeu HTTP %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	if responseBody == nil {
		return nil
	}

	if err := json.Unmarshal(
		body,
		responseBody,
	); err != nil {
		return fmt.Errorf(
			"resposta JSON inválida do AMP: %w; conteúdo: %s",
			err,
			strings.TrimSpace(string(body)),
		)
	}

	return nil
}

func buildAPIURL(
	baseURL string,
	module string,
	method string,
) string {
	return strings.TrimRight(
		baseURL,
		"/",
	) + "/API/" + module + "/" + method
}
