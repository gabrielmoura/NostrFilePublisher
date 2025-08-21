package model

import (
	"net/http"
	"sync"
)

type MapString map[string]string

// AppState armazena o estado global compartilhado da aplicação.
// Ele centraliza dados como configurações, chaves e estado da conexão.
type AppState struct {
	// HttpClient é um cliente HTTP reutilizável para requisições de rede,
	// como buscar thumbnails para gerar o blurhash.
	HttpClient *http.Client

	// Relays armazena o status de conexão dos relays configurados.
	// A chave do mapa é a URL do relay (ex: "wss://relay.damus.io").
	Relays map[string]*RelayStatus

	// BlossomServers armazena a lista de servidores Blossom configurados.
	// A chave e o valor são a URL do servidor.
	BlossomServers MapString

	// Nsec armazena a chave privada do usuário no formato nsec (Nostr Secret Key).
	// Esta chave é usada para assinar todos os eventos antes da publicação.
	Nsec string

	Npub     string
	UniqueID string

	// Mutex é usado para prevenir "race conditions" ao acessar os dados
	// do AppState de diferentes goroutines (por exemplo, UI e threads de rede).
	// Qualquer modificação ou leitura nos mapas (Relays, BlossomServers) ou na Nsec
	// deve ser protegida com Lock() e Unlock().
	Mutex *sync.Mutex
}

// RelayStatus representa o estado de um único relay.
type RelayStatus struct {
	// URL é o endereço websocket do relay.
	URL string

	// Status descreve o estado atual da conexão (ex: "Conectado", "Desconectado", "Erro").
	Status string
}

type PreEvent struct {
	Sha256, MimeType, BlurHash, Path, PubKey, PrivKey string
	Size                                              int64
	Tags, Indexers                                    []string
	Nsfw                                              bool
	Kind                                              int
}

//	{
//	 "url": "https://cdn.example.com/b1674191a88ec5cdd733e4240a81803105dc412d6c6708d53ab94fc248f4f553.pdf",
//	 "sha256": "b1674191a88ec5cdd733e4240a81803105dc412d6c6708d53ab94fc248f4f553",
//	 "size": 184292,
//	 "type": "application/pdf",
//	 "uploaded": 1725105921
//	}
type BlossomResponse struct {
	URL      string `json:"url"`
	Sha256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Type     string `json:"type"`
	Uploaded int64  `json:"uploaded"` // Timestamp in seconds
}
