package util

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

// GetMimeFromUrl faz uma requisição HTTP para a URL fornecida e tenta determinar o tipo MIME do conteúdo.
// Ele lê os primeiros 500 bytes do corpo da resposta para detectar o tipo MIME.
func GetMimeFromUrl(httpClient *http.Client, url string) (string, error) {
	const byteLimit = 500

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Range", "bytes=0-500")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return "", nil
	}

	limitedReader := io.LimitReader(resp.Body, byteLimit)
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, limitedReader)
	if err != nil {
		return "", err
	}

	contentType := resp.Header.Get("Content-Type")

	if contentType != "" && !strings.HasPrefix(contentType, "application/octet-stream") {
		return contentType, nil
	}

	return http.DetectContentType(buf.Bytes()), nil
}
