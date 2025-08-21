package blossom

import (
	"NostrFilePublisher/model"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/nbd-wtf/go-nostr"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// SendFile envia um arquivo para múltiplos endpoints Blossom e retorna as respostas e erros.
func SendFile(httpClient *http.Client, preEvt model.PreEvent, appState model.AppState) ([]model.BlossomResponse, []error) {
	var (
		errs      []error
		responses []model.BlossomResponse
	)
	if len(appState.BlossomServers) == 0 {
		return nil, []error{fmt.Errorf("no Blossom servers configured")}
	}

	file, err := os.Open(preEvt.Path)
	if err != nil {
		return nil, []error{fmt.Errorf("error opening file %s: %w", preEvt.Path, err)}
	}
	defer file.Close()

	authHex, err := buildAuthHeader(preEvt, appState, file.Name())
	if err != nil {
		return nil, []error{fmt.Errorf("error signing event: %w", err)}
	}

	for _, bURL := range appState.BlossomServers {
		resp, err := uploadFile(httpClient, bURL, file, preEvt, authHex)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		responses = append(responses, *resp)
	}

	return responses, errs
}

// buildAuthHeader cria e assina o evento Nostr para autenticação.
func buildAuthHeader(preEvt model.PreEvent, appState model.AppState, fileName string) (string, error) {
	tags := nostr.Tags{
		{"t", "upload"},
		{"x", preEvt.Sha256},
		{"expiration", fmt.Sprintf("%d", time.Now().Add(10*time.Minute).Unix())},
	}

	evt := &nostr.Event{
		CreatedAt: nostr.Now(),
		Tags:      tags,
		Content:   fmt.Sprintf("Upload %s", fileName),
		Kind:      nostr.KindBlobs,
		PubKey:    appState.Npub,
	}

	if err := evt.Sign(appState.Nsec); err != nil {
		return "", err
	}

	rawEvent, _ := json.Marshal(evt)
	return hex.EncodeToString(rawEvent), nil
}

// uploadFile realiza o upload para um único servidor Blossom.
func uploadFile(httpClient *http.Client, blossomURL string, file *os.File, preEvt model.PreEvent, authHex string) (*model.BlossomResponse, error) {
	// resetar o ponteiro do arquivo para cada upload
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("error resetting file pointer: %w", err)
	}

	parsedURL, err := url.Parse(blossomURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %s: %w", blossomURL, err)
	}
	parsedURL.Path = "/upload"

	req, err := http.NewRequest(http.MethodPut, parsedURL.String(), file)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", preEvt.MimeType)
	req.Header.Set("X-File-Metadata", preEvt.Sha256)
	req.Header.Set("Authorization", fmt.Sprintf("Nostr %s", authHex))
	req.ContentLength = preEvt.Size

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error uploading to %s: %w", parsedURL.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(body))
	}

	var blossomResp model.BlossomResponse
	if err := json.Unmarshal(body, &blossomResp); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return &blossomResp, nil
}
