// Package proxy provides the main proxy server.
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
)

type ConfighubOrderflowProxyCredentials struct {
	TLSCert            string         `json:"tls_cert"`
	EcdsaPubkeyAddress common.Address `json:"ecdsa_pubkey_address"`
}

type ConfighubBuilder struct {
	Name           string                             `json:"name"`
	IP             string                             `json:"ip"`
	OrderflowProxy ConfighubOrderflowProxyCredentials `json:"orderflow_proxy"`
}

type BuilderConfigHub struct {
	log      *slog.Logger
	endpoint string
}

func NewBuilderConfigHub(log *slog.Logger, endpoint string) *BuilderConfigHub {
	return &BuilderConfigHub{
		log:      log,
		endpoint: endpoint,
	}
}

func (b *BuilderConfigHub) RegisterCredentials(info ConfighubOrderflowProxyCredentials) error {
	body, err := json.Marshal(info)
	if err != nil {
		return err
	}
	resp, err := http.Post(b.endpoint+"/api/l1-builder/v1/register_credentials/orderflow-proxy", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("builder config hub returned error, code: %d, body: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (b *BuilderConfigHub) Builders() (result []ConfighubBuilder, err error) {
	defer func() {
		if err != nil {
			confighubErrorsCounter.Inc()
			b.log.Error("Failed to fetch peer list from config hub", slog.Any("error", err))
		}
	}()

	var resp *http.Response
	resp, err = http.Get(b.endpoint + "/api/l1-builder/v1/builders")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	return
}
